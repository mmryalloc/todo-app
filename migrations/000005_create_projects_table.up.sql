CREATE TABLE IF NOT EXISTS projects (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name VARCHAR(255) NOT NULL,
  color VARCHAR(7) NOT NULL,
  is_default BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMP NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS projects_user_id_created_at_idx
  ON projects (user_id, created_at DESC);

CREATE UNIQUE INDEX IF NOT EXISTS projects_one_default_per_user_idx
  ON projects (user_id)
  WHERE is_default = TRUE;

INSERT INTO projects (user_id, name, color, is_default)
SELECT id, 'Inbox', '#64748B', TRUE
FROM users
WHERE NOT EXISTS (
  SELECT 1 FROM projects
  WHERE projects.user_id = users.id
    AND projects.is_default = TRUE
);

ALTER TABLE tasks
  ADD COLUMN project_id BIGINT;

UPDATE tasks
SET project_id = projects.id
FROM projects
WHERE tasks.user_id = projects.user_id
  AND projects.is_default = TRUE
  AND tasks.project_id IS NULL;

ALTER TABLE tasks
  ALTER COLUMN project_id SET NOT NULL,
  ADD CONSTRAINT tasks_project_id_fkey
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS tasks_project_id_idx
  ON tasks (project_id);
