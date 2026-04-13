package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	taskdomain "example.com/taskservice/internal/domain/task"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Create(ctx context.Context, task *taskdomain.Task) (*taskdomain.Task, error) {
	const query = `
		INSERT INTO tasks (
			title,
			description,
			status,
			created_at,
			updated_at,
			kind,
			scheduled_at,
			recurrence,
			template_id
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, title, description, status, created_at, updated_at, kind, scheduled_at, recurrence, template_id
	`

	recurrencePayload, err := marshalRecurrence(task.Recurrence)
	if err != nil {
		return nil, err
	}

	row := r.pool.QueryRow(
		ctx,
		query,
		task.Title,
		task.Description,
		task.Status,
		task.CreatedAt,
		task.UpdatedAt,
		task.Kind,
		task.ScheduledAt,
		recurrencePayload,
		task.TemplateID,
	)

	created, err := scanTask(row)
	if err != nil {
		return nil, err
	}

	return created, nil
}

func (r *Repository) GetByID(ctx context.Context, id int64) (*taskdomain.Task, error) {
	const query = `
		SELECT id, title, description, status, created_at, updated_at, kind, scheduled_at, recurrence, template_id
		FROM tasks
		WHERE id = $1
	`

	row := r.pool.QueryRow(ctx, query, id)
	found, err := scanTask(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, taskdomain.ErrNotFound
		}

		return nil, err
	}

	return found, nil
}

func (r *Repository) Update(ctx context.Context, task *taskdomain.Task) (*taskdomain.Task, error) {
	const query = `
		UPDATE tasks
		SET title = $1,
			description = $2,
			status = $3,
			updated_at = $4,
			kind = $5,
			scheduled_at = $6,
			recurrence = $7,
			template_id = $8
		WHERE id = $9
		RETURNING id, title, description, status, created_at, updated_at, kind, scheduled_at, recurrence, template_id
	`

	recurrencePayload, err := marshalRecurrence(task.Recurrence)
	if err != nil {
		return nil, err
	}

	row := r.pool.QueryRow(
		ctx,
		query,
		task.Title,
		task.Description,
		task.Status,
		task.UpdatedAt,
		task.Kind,
		task.ScheduledAt,
		recurrencePayload,
		task.TemplateID,
		task.ID,
	)

	updated, err := scanTask(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, taskdomain.ErrNotFound
		}

		return nil, err
	}

	return updated, nil
}

func (r *Repository) Delete(ctx context.Context, id int64) error {
	const query = `DELETE FROM tasks WHERE id = $1`

	result, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return taskdomain.ErrNotFound
	}

	return nil
}

func (r *Repository) List(ctx context.Context) ([]taskdomain.Task, error) {
	const query = `
		SELECT id, title, description, status, created_at, updated_at, kind, scheduled_at, recurrence, template_id
		FROM tasks
		ORDER BY id DESC
	`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanTasks(rows)
}

func (r *Repository) ListRecurringTemplates(ctx context.Context) ([]taskdomain.Task, error) {
	const query = `
		SELECT id, title, description, status, created_at, updated_at, kind, scheduled_at, recurrence, template_id
		FROM tasks
		WHERE kind = $1
		ORDER BY id ASC
	`

	rows, err := r.pool.Query(ctx, query, taskdomain.KindRecurringTemplate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanTasks(rows)
}

func (r *Repository) ListOccurrencesInRange(ctx context.Context, templateID int64, from, to time.Time) ([]taskdomain.Task, error) {
	const query = `
		SELECT id, title, description, status, created_at, updated_at, kind, scheduled_at, recurrence, template_id
		FROM tasks
		WHERE template_id = $1
			AND scheduled_at >= $2
			AND scheduled_at <= $3
		ORDER BY scheduled_at ASC, id ASC
	`

	rows, err := r.pool.Query(ctx, query, templateID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanTasks(rows)
}

func (r *Repository) DeleteOccurrencesFrom(ctx context.Context, templateID int64, from time.Time) error {
	const query = `
		DELETE FROM tasks
		WHERE template_id = $1
			AND scheduled_at >= $2
	`

	_, err := r.pool.Exec(ctx, query, templateID, from)
	return err
}

type taskScanner interface {
	Scan(dest ...any) error
}

func scanTasks(rows pgx.Rows) ([]taskdomain.Task, error) {
	tasks := make([]taskdomain.Task, 0)
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}

		tasks = append(tasks, *task)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return tasks, nil
}

func scanTask(scanner taskScanner) (*taskdomain.Task, error) {
	var (
		task           taskdomain.Task
		status         string
		kind           string
		templateID     sql.NullInt64
		scheduledAt    sql.NullTime
		recurrenceJSON []byte
	)

	if err := scanner.Scan(
		&task.ID,
		&task.Title,
		&task.Description,
		&status,
		&task.CreatedAt,
		&task.UpdatedAt,
		&kind,
		&scheduledAt,
		&recurrenceJSON,
		&templateID,
	); err != nil {
		return nil, err
	}

	task.Status = taskdomain.Status(status)
	task.Kind = taskdomain.Kind(kind)
	if !task.Kind.Valid() {
		return nil, fmt.Errorf("unknown task kind %q", kind)
	}

	if scheduledAt.Valid {
		scheduled := scheduledAt.Time.UTC()
		task.ScheduledAt = &scheduled
	}

	if len(recurrenceJSON) > 0 {
		var recurrence taskdomain.Recurrence
		if err := json.Unmarshal(recurrenceJSON, &recurrence); err != nil {
			return nil, fmt.Errorf("unmarshal recurrence: %w", err)
		}

		task.Recurrence = &recurrence
	}

	if templateID.Valid {
		value := templateID.Int64
		task.TemplateID = &value
	}

	return &task, nil
}

func marshalRecurrence(recurrence *taskdomain.Recurrence) ([]byte, error) {
	if recurrence == nil {
		return nil, nil
	}

	payload, err := json.Marshal(recurrence)
	if err != nil {
		return nil, fmt.Errorf("marshal recurrence: %w", err)
	}

	return payload, nil
}
