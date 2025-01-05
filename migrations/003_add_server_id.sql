-- Add server_id column to tasks table
ALTER TABLE tasks ADD COLUMN server_id VARCHAR(64);

-- Add server_id column to check_ins table
ALTER TABLE check_ins ADD COLUMN server_id VARCHAR(64);

-- Set default server_id for existing records (using the first server from server_settings)
WITH first_server AS (
    SELECT server_id FROM server_settings LIMIT 1
)
UPDATE tasks SET server_id = (SELECT server_id FROM first_server)
WHERE server_id IS NULL;

WITH first_server AS (
    SELECT server_id FROM server_settings LIMIT 1
)
UPDATE check_ins SET server_id = (SELECT server_id FROM first_server)
WHERE server_id IS NULL;

-- Make server_id NOT NULL after setting defaults
ALTER TABLE tasks ALTER COLUMN server_id SET NOT NULL;
ALTER TABLE check_ins ALTER COLUMN server_id SET NOT NULL;

-- Add indexes for server_id columns
CREATE INDEX IF NOT EXISTS idx_tasks_server_id ON tasks(server_id);
CREATE INDEX IF NOT EXISTS idx_check_ins_server_id ON check_ins(server_id); 