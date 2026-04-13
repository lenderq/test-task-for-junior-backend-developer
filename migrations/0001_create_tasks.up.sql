CREATE TABLE IF NOT EXISTS tasks (
	id BIGSERIAL PRIMARY KEY,
	title TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL,
	kind TEXT NOT NULL DEFAULT 'one_time',
	scheduled_at TIMESTAMPTZ NULL,
	recurrence JSONB NULL,
	template_id BIGINT NULL REFERENCES tasks(id) ON DELETE CASCADE,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks (status);
CREATE INDEX IF NOT EXISTS idx_tasks_kind ON tasks (kind);
CREATE INDEX IF NOT EXISTS idx_tasks_template_id ON tasks (template_id);
CREATE UNIQUE INDEX IF NOT EXISTS ux_tasks_template_occurrence
	ON tasks (template_id, scheduled_at)
	WHERE template_id IS NOT NULL AND scheduled_at IS NOT NULL;
