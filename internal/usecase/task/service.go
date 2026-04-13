package task

import (
	"context"
	"fmt"
	"strings"
	"time"

	taskdomain "example.com/taskservice/internal/domain/task"
)

type Service struct {
	repo Repository
	now  func() time.Time
}

const materializationHorizonDays = 30

func NewService(repo Repository) *Service {
	return &Service{
		repo: repo,
		now:  func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) Create(ctx context.Context, input CreateInput) (*taskdomain.Task, error) {
	normalized, err := validateCreateInput(input)
	if err != nil {
		return nil, err
	}

	model := &taskdomain.Task{
		Title:       normalized.Title,
		Description: normalized.Description,
		Status:      normalized.Status,
		Kind:        taskdomain.KindOneTime,
		ScheduledAt: cloneTimePtr(normalized.ScheduledAt),
		Recurrence:  normalized.Recurrence.Clone(),
	}
	if normalized.Recurrence != nil {
		model.Kind = taskdomain.KindRecurringTemplate
	}
	now := s.now()
	model.CreatedAt = now
	model.UpdatedAt = now

	created, err := s.repo.Create(ctx, model)
	if err != nil {
		return nil, err
	}

	if err := s.materializeRecurringTasks(ctx, now); err != nil {
		return nil, err
	}

	return created, nil
}

func (s *Service) GetByID(ctx context.Context, id int64) (*taskdomain.Task, error) {
	if id <= 0 {
		return nil, fmt.Errorf("%w: id must be positive", ErrInvalidInput)
	}

	if err := s.materializeRecurringTasks(ctx, s.now()); err != nil {
		return nil, err
	}

	return s.repo.GetByID(ctx, id)
}

func (s *Service) Update(ctx context.Context, id int64, input UpdateInput) (*taskdomain.Task, error) {
	if id <= 0 {
		return nil, fmt.Errorf("%w: id must be positive", ErrInvalidInput)
	}

	now := s.now()
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	normalized, err := validateUpdateInput(existing, input)
	if err != nil {
		return nil, err
	}

	model := &taskdomain.Task{
		ID:          id,
		Title:       normalized.Title,
		Description: normalized.Description,
		Status:      normalized.Status,
		Kind:        resolvedTaskKind(existing.Kind, normalized.Recurrence),
		TemplateID:  cloneInt64Ptr(existing.TemplateID),
		ScheduledAt: cloneTimePtr(normalized.ScheduledAt),
		Recurrence:  normalized.Recurrence.Clone(),
		UpdatedAt:   now,
	}
	if model.Kind != taskdomain.KindRecurringOccurrence {
		model.TemplateID = nil
	}

	updated, err := s.repo.Update(ctx, model)
	if err != nil {
		return nil, err
	}

	if updated.Kind == taskdomain.KindRecurringTemplate {
		if err := s.repo.DeleteOccurrencesFrom(ctx, updated.ID, beginningOfDay(now)); err != nil {
			return nil, err
		}
	}

	if err := s.materializeRecurringTasks(ctx, now); err != nil {
		return nil, err
	}

	return updated, nil
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	if id <= 0 {
		return fmt.Errorf("%w: id must be positive", ErrInvalidInput)
	}

	return s.repo.Delete(ctx, id)
}

func (s *Service) List(ctx context.Context) ([]taskdomain.Task, error) {
	if err := s.materializeRecurringTasks(ctx, s.now()); err != nil {
		return nil, err
	}

	return s.repo.List(ctx)
}

func validateCreateInput(input CreateInput) (CreateInput, error) {
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)

	if input.Title == "" {
		return CreateInput{}, fmt.Errorf("%w: title is required", ErrInvalidInput)
	}

	if input.Status == "" {
		input.Status = taskdomain.StatusNew
	}

	if !input.Status.Valid() {
		return CreateInput{}, fmt.Errorf("%w: invalid status", ErrInvalidInput)
	}

	normalizedSchedule, err := normalizeSchedule(input.Status, input.ScheduledAt, input.Recurrence)
	if err != nil {
		return CreateInput{}, err
	}

	input.ScheduledAt = normalizedSchedule.scheduledAt
	input.Recurrence = normalizedSchedule.recurrence

	return input, nil
}

func validateUpdateInput(existing *taskdomain.Task, input UpdateInput) (UpdateInput, error) {
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)

	if input.Title == "" {
		return UpdateInput{}, fmt.Errorf("%w: title is required", ErrInvalidInput)
	}

	if !input.Status.Valid() {
		return UpdateInput{}, fmt.Errorf("%w: invalid status", ErrInvalidInput)
	}

	if input.ScheduledAt == nil {
		input.ScheduledAt = cloneTimePtr(existing.ScheduledAt)
	}

	if input.Recurrence == nil && existing.Kind == taskdomain.KindRecurringTemplate {
		input.Recurrence = existing.Recurrence.Clone()
	}

	if existing.Kind == taskdomain.KindRecurringOccurrence && input.Recurrence != nil {
		return UpdateInput{}, fmt.Errorf("%w: generated occurrences cannot define recurrence; update the template instead", ErrInvalidInput)
	}

	normalizedSchedule, err := normalizeSchedule(input.Status, input.ScheduledAt, input.Recurrence)
	if err != nil {
		return UpdateInput{}, err
	}

	input.ScheduledAt = normalizedSchedule.scheduledAt
	input.Recurrence = normalizedSchedule.recurrence

	return input, nil
}

type normalizedSchedule struct {
	scheduledAt *time.Time
	recurrence  *taskdomain.Recurrence
}

func normalizeSchedule(status taskdomain.Status, scheduledAt *time.Time, recurrence *taskdomain.Recurrence) (normalizedSchedule, error) {
	if scheduledAt != nil {
		normalized := scheduledAt.UTC()
		scheduledAt = &normalized
	}

	if recurrence == nil {
		return normalizedSchedule{scheduledAt: scheduledAt}, nil
	}

	if scheduledAt == nil {
		return normalizedSchedule{}, fmt.Errorf("%w: scheduled_at is required when recurrence is provided", ErrInvalidInput)
	}

	if status == taskdomain.StatusDone {
		return normalizedSchedule{}, fmt.Errorf("%w: recurring templates cannot be created with done status", ErrInvalidInput)
	}

	normalizedRecurrence, err := recurrence.Normalize(*scheduledAt)
	if err != nil {
		return normalizedSchedule{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	return normalizedSchedule{
		scheduledAt: scheduledAt,
		recurrence:  normalizedRecurrence,
	}, nil
}

func (s *Service) materializeRecurringTasks(ctx context.Context, now time.Time) error {
	templates, err := s.repo.ListRecurringTemplates(ctx)
	if err != nil {
		return err
	}

	for i := range templates {
		if err := s.materializeTemplate(ctx, &templates[i], now); err != nil {
			return err
		}
	}

	return nil
}

func (s *Service) materializeTemplate(ctx context.Context, template *taskdomain.Task, now time.Time) error {
	if template.Kind != taskdomain.KindRecurringTemplate || template.Recurrence == nil || template.ScheduledAt == nil {
		return nil
	}

	from := beginningOfDay(now)
	templateStart := beginningOfDay(*template.ScheduledAt)
	if templateStart.After(from) {
		from = templateStart
	}

	to := endOfDay(now.AddDate(0, 0, materializationHorizonDays))
	occurrences, err := template.Recurrence.OccurrencesBetween(*template.ScheduledAt, from, to)
	if err != nil {
		return fmt.Errorf("calculate occurrences for template %d: %w", template.ID, err)
	}

	existingOccurrences, err := s.repo.ListOccurrencesInRange(ctx, template.ID, from, to)
	if err != nil {
		return err
	}

	existingByTimestamp := make(map[string]struct{}, len(existingOccurrences))
	for i := range existingOccurrences {
		if existingOccurrences[i].ScheduledAt == nil {
			continue
		}

		existingByTimestamp[existingOccurrences[i].ScheduledAt.UTC().Format(time.RFC3339Nano)] = struct{}{}
	}

	for _, occurrenceTime := range occurrences {
		key := occurrenceTime.UTC().Format(time.RFC3339Nano)
		if _, exists := existingByTimestamp[key]; exists {
			continue
		}

		scheduledAt := occurrenceTime.UTC()
		instance := &taskdomain.Task{
			Title:       template.Title,
			Description: template.Description,
			Status:      template.Status,
			Kind:        taskdomain.KindRecurringOccurrence,
			TemplateID:  &template.ID,
			ScheduledAt: &scheduledAt,
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		if _, err := s.repo.Create(ctx, instance); err != nil {
			return err
		}
	}

	return nil
}

func resolvedTaskKind(existing taskdomain.Kind, recurrence *taskdomain.Recurrence) taskdomain.Kind {
	if recurrence != nil {
		return taskdomain.KindRecurringTemplate
	}

	if existing == taskdomain.KindRecurringOccurrence {
		return existing
	}

	return taskdomain.KindOneTime
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}

	cloned := value.UTC()
	return &cloned
}

func cloneInt64Ptr(value *int64) *int64 {
	if value == nil {
		return nil
	}

	cloned := *value
	return &cloned
}

func beginningOfDay(value time.Time) time.Time {
	value = value.UTC()
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}

func endOfDay(value time.Time) time.Time {
	value = value.UTC()
	return time.Date(value.Year(), value.Month(), value.Day(), 23, 59, 59, int(time.Second-time.Nanosecond), time.UTC)
}
