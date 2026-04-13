package handlers

import (
	"time"

	taskdomain "example.com/taskservice/internal/domain/task"
)

type taskMutationDTO struct {
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Status      taskdomain.Status `json:"status"`
	ScheduledAt *time.Time        `json:"scheduled_at,omitempty"`
	Recurrence  *recurrenceDTO    `json:"recurrence,omitempty"`
}

type taskDTO struct {
	ID          int64             `json:"id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Status      taskdomain.Status `json:"status"`
	Kind        taskdomain.Kind   `json:"kind"`
	TemplateID  *int64            `json:"template_id,omitempty"`
	ScheduledAt *time.Time        `json:"scheduled_at,omitempty"`
	Recurrence  *recurrenceDTO    `json:"recurrence,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

type recurrenceDTO struct {
	Type          taskdomain.RecurrenceType `json:"type"`
	EveryNDays    int                       `json:"every_n_days,omitempty"`
	DayOfMonth    int                       `json:"day_of_month,omitempty"`
	SpecificDates []taskdomain.Date        `json:"specific_dates,omitempty"`
	DayParity     taskdomain.DayParity     `json:"day_parity,omitempty"`
}

func newTaskDTO(task *taskdomain.Task) taskDTO {
	return taskDTO{
		ID:          task.ID,
		Title:       task.Title,
		Description: task.Description,
		Status:      task.Status,
		Kind:        task.Kind,
		TemplateID:  cloneTemplateID(task.TemplateID),
		ScheduledAt: cloneTime(task.ScheduledAt),
		Recurrence:  newRecurrenceDTO(task.Recurrence),
		CreatedAt:   task.CreatedAt,
		UpdatedAt:   task.UpdatedAt,
	}
}

func (dto *recurrenceDTO) toDomain() *taskdomain.Recurrence {
	if dto == nil {
		return nil
	}

	recurrence := &taskdomain.Recurrence{
		Type:          dto.Type,
		EveryNDays:    dto.EveryNDays,
		DayOfMonth:    dto.DayOfMonth,
		DayParity:     dto.DayParity,
		SpecificDates: append([]taskdomain.Date(nil), dto.SpecificDates...),
	}

	return recurrence
}

func newRecurrenceDTO(recurrence *taskdomain.Recurrence) *recurrenceDTO {
	if recurrence == nil {
		return nil
	}

	return &recurrenceDTO{
		Type:          recurrence.Type,
		EveryNDays:    recurrence.EveryNDays,
		DayOfMonth:    recurrence.DayOfMonth,
		SpecificDates: append([]taskdomain.Date(nil), recurrence.SpecificDates...),
		DayParity:     recurrence.DayParity,
	}
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}

	cloned := value.UTC()
	return &cloned
}

func cloneTemplateID(value *int64) *int64 {
	if value == nil {
		return nil
	}

	cloned := *value
	return &cloned
}
