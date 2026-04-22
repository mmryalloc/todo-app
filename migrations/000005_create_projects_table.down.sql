DROP INDEX IF EXISTS tasks_project_id_idx;

ALTER TABLE tasks
  DROP CONSTRAINT IF EXISTS tasks_project_id_fkey,
  DROP COLUMN IF EXISTS project_id;

DROP INDEX IF EXISTS projects_one_default_per_user_idx;
DROP INDEX IF EXISTS projects_user_id_created_at_idx;
DROP TABLE IF EXISTS projects;
