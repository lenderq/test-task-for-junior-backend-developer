package task

import (
	"context"
	"reflect"
	"sort"
	"testing"
	"time"

	taskdomain "example.com/taskservice/internal/domain/task"
)

func TestServiceCreateMaterializesRecurringTasks(t *testing.T) {
	t.Parallel()

	repo := newMemoryRepository()
	service := NewService(repo)
	now := mustTime(t, "2026-04-13T10:00:00Z")
	service.now = func() time.Time { return now }

	scheduledAt := mustTime(t, "2026-04-13T09:00:00Z")
	created, err := service.Create(context.Background(), CreateInput{
		Title:       "Daily patient calls",
		Description: "Call all post-op patients",
		Status:      taskdomain.StatusNew,
		ScheduledAt: &scheduledAt,
		Recurrence: &taskdomain.Recurrence{
			Type:       taskdomain.RecurrenceEveryNDays,
			EveryNDays: 2,
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if created.Kind != taskdomain.KindRecurringTemplate {
		t.Fatalf("Create() kind = %q, want %q", created.Kind, taskdomain.KindRecurringTemplate)
	}

	got := occurrenceSchedule(t, repo)
	wantPrefix := []string{
		"2026-04-13T09:00:00Z",
		"2026-04-15T09:00:00Z",
		"2026-04-17T09:00:00Z",
	}

	if len(got) < len(wantPrefix) {
		t.Fatalf("Create() materialized too few occurrences: got %#v", got)
	}

	if !reflect.DeepEqual(got[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("Create() first occurrences mismatch\nwant: %#v\ngot:  %#v", wantPrefix, got[:len(wantPrefix)])
	}
}

func TestServiceUpdateRegeneratesFutureOccurrences(t *testing.T) {
	t.Parallel()

	repo := newMemoryRepository()
	service := NewService(repo)
	now := mustTime(t, "2026-04-13T10:00:00Z")
	service.now = func() time.Time { return now }

	scheduledAt := mustTime(t, "2026-04-13T09:00:00Z")
	created, err := service.Create(context.Background(), CreateInput{
		Title:       "Inventory check",
		Description: "Prepare stock sheet",
		Status:      taskdomain.StatusNew,
		ScheduledAt: &scheduledAt,
		Recurrence: &taskdomain.Recurrence{
			Type:       taskdomain.RecurrenceEveryNDays,
			EveryNDays: 1,
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	_, err = service.Update(context.Background(), created.ID, UpdateInput{
		Title:       "Inventory check",
		Description: "Prepare stock sheet",
		Status:      taskdomain.StatusNew,
		ScheduledAt: &scheduledAt,
		Recurrence: &taskdomain.Recurrence{
			Type:       taskdomain.RecurrenceEveryNDays,
			EveryNDays: 3,
		},
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	got := occurrenceSchedule(t, repo)
	wantPrefix := []string{
		"2026-04-13T09:00:00Z",
		"2026-04-16T09:00:00Z",
		"2026-04-19T09:00:00Z",
	}

	if len(got) < len(wantPrefix) {
		t.Fatalf("Update() materialized too few occurrences: got %#v", got)
	}

	if !reflect.DeepEqual(got[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("Update() first occurrences mismatch\nwant: %#v\ngot:  %#v", wantPrefix, got[:len(wantPrefix)])
	}
}

type memoryRepository struct {
	nextID int64
	tasks  map[int64]*taskdomain.Task
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{
		nextID: 1,
		tasks:  make(map[int64]*taskdomain.Task),
	}
}

func (r *memoryRepository) Create(_ context.Context, task *taskdomain.Task) (*taskdomain.Task, error) {
	cloned := cloneTask(task)
	cloned.ID = r.nextID
	r.nextID++
	r.tasks[cloned.ID] = cloned

	return cloneTask(cloned), nil
}

func (r *memoryRepository) GetByID(_ context.Context, id int64) (*taskdomain.Task, error) {
	task, ok := r.tasks[id]
	if !ok {
		return nil, taskdomain.ErrNotFound
	}

	return cloneTask(task), nil
}

func (r *memoryRepository) Update(_ context.Context, task *taskdomain.Task) (*taskdomain.Task, error) {
	existing, ok := r.tasks[task.ID]
	if !ok {
		return nil, taskdomain.ErrNotFound
	}

	cloned := cloneTask(task)
	cloned.CreatedAt = existing.CreatedAt
	r.tasks[task.ID] = cloned

	return cloneTask(cloned), nil
}

func (r *memoryRepository) Delete(_ context.Context, id int64) error {
	if _, ok := r.tasks[id]; !ok {
		return taskdomain.ErrNotFound
	}

	delete(r.tasks, id)
	for taskID, task := range r.tasks {
		if task.TemplateID != nil && *task.TemplateID == id {
			delete(r.tasks, taskID)
		}
	}

	return nil
}

func (r *memoryRepository) List(_ context.Context) ([]taskdomain.Task, error) {
	return r.collect(func(task *taskdomain.Task) bool { return true }), nil
}

func (r *memoryRepository) ListRecurringTemplates(_ context.Context) ([]taskdomain.Task, error) {
	return r.collect(func(task *taskdomain.Task) bool {
		return task.Kind == taskdomain.KindRecurringTemplate
	}), nil
}

func (r *memoryRepository) ListOccurrencesInRange(_ context.Context, templateID int64, from, to time.Time) ([]taskdomain.Task, error) {
	return r.collect(func(task *taskdomain.Task) bool {
		if task.TemplateID == nil || *task.TemplateID != templateID || task.ScheduledAt == nil {
			return false
		}

		return !task.ScheduledAt.Before(from) && !task.ScheduledAt.After(to)
	}), nil
}

func (r *memoryRepository) DeleteOccurrencesFrom(_ context.Context, templateID int64, from time.Time) error {
	for taskID, task := range r.tasks {
		if task.TemplateID == nil || *task.TemplateID != templateID || task.ScheduledAt == nil {
			continue
		}

		if !task.ScheduledAt.Before(from) {
			delete(r.tasks, taskID)
		}
	}

	return nil
}

func (r *memoryRepository) collect(filter func(task *taskdomain.Task) bool) []taskdomain.Task {
	result := make([]taskdomain.Task, 0)
	for _, task := range r.tasks {
		if !filter(task) {
			continue
		}

		result = append(result, *cloneTask(task))
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})

	return result
}

func cloneTask(task *taskdomain.Task) *taskdomain.Task {
	if task == nil {
		return nil
	}

	cloned := *task
	if task.TemplateID != nil {
		templateID := *task.TemplateID
		cloned.TemplateID = &templateID
	}
	if task.ScheduledAt != nil {
		scheduledAt := task.ScheduledAt.UTC()
		cloned.ScheduledAt = &scheduledAt
	}
	if task.Recurrence != nil {
		cloned.Recurrence = task.Recurrence.Clone()
	}

	return &cloned
}

func occurrenceSchedule(t *testing.T, repo *memoryRepository) []string {
	t.Helper()

	tasks, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	schedule := make([]string, 0)
	for _, task := range tasks {
		if task.Kind != taskdomain.KindRecurringOccurrence || task.ScheduledAt == nil {
			continue
		}

		schedule = append(schedule, task.ScheduledAt.UTC().Format(time.RFC3339))
	}

	sort.Strings(schedule)
	return schedule
}

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()

	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("time.Parse(%q) error = %v", value, err)
	}

	return parsed.UTC()
}
