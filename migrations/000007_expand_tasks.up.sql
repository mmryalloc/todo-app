ALTER TABLE tasks
  ADD COLUMN IF NOT EXISTS status VARCHAR(20) NOT NULL DEFAULT 'open',
  ADD COLUMN IF NOT EXISTS due_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS due_timezone VARCHAR(64),
  ADD COLUMN IF NOT EXISTS position INTEGER NOT NULL DEFAULT 0;

UPDATE tasks
SET status = CASE WHEN completed THEN 'done' ELSE 'open' END
WHERE status IS NULL OR status = 'open';

WITH ranked AS (
  SELECT id, ROW_NUMBER() OVER (PARTITION BY project_id ORDER BY created_at ASC, id ASC) - 1 AS new_position
  FROM tasks
)
UPDATE tasks
SET position = ranked.new_position
FROM ranked
WHERE tasks.id = ranked.id;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'tasks_status_check'
  ) THEN
    ALTER TABLE tasks
      ADD CONSTRAINT tasks_status_check
      CHECK (status IN ('open', 'in_progress', 'done'));
  END IF;
END $$;

CREATE INDEX IF NOT EXISTS tasks_project_id_position_idx
  ON tasks (project_id, position);

CREATE INDEX IF NOT EXISTS tasks_project_id_status_idx
  ON tasks (project_id, status);

CREATE INDEX IF NOT EXISTS tasks_due_at_idx
  ON tasks (due_at);
