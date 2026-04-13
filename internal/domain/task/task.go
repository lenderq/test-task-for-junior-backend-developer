package task

import "time"

type Status string

const (
	StatusNew        Status = "new"
	StatusInProgress Status = "in_progress"
	StatusDone       Status = "done"
)

type Kind string

const (
	KindOneTime             Kind = "one_time"
	KindRecurringTemplate   Kind = "recurring_template"
	KindRecurringOccurrence Kind = "recurring_occurrence"
)

type Task struct {
	ID          int64       `json:"id"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Status      Status      `json:"status"`
	Kind        Kind        `json:"kind"`
	TemplateID  *int64      `json:"template_id,omitempty"`
	ScheduledAt *time.Time  `json:"scheduled_at,omitempty"`
	Recurrence  *Recurrence `json:"recurrence,omitempty"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

func (s Status) Valid() bool {
	switch s {
	case StatusNew, StatusInProgress, StatusDone:
		return true
	default:
		return false
	}
}

func (k Kind) Valid() bool {
	switch k {
	case KindOneTime, KindRecurringTemplate, KindRecurringOccurrence:
		return true
	default:
		return false
	}
}
