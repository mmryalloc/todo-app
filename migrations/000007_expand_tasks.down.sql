DROP INDEX IF EXISTS tasks_due_at_idx;
DROP INDEX IF EXISTS tasks_project_id_status_idx;
DROP INDEX IF EXISTS tasks_project_id_position_idx;

ALTER TABLE tasks
  DROP CONSTRAINT IF EXISTS tasks_status_check,
  DROP COLUMN IF EXISTS position,
  DROP COLUMN IF EXISTS due_timezone,
  DROP COLUMN IF EXISTS due_at,
  DROP COLUMN IF EXISTS status;
