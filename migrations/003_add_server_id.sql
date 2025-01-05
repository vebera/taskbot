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

-- Make server_id NOT NULL if not already
DO $$ 
BEGIN
    BEGIN
        ALTER TABLE tasks ALTER COLUMN server_id SET NOT NULL;
    EXCEPTION
        WHEN others THEN NULL;
    END;
    BEGIN
        ALTER TABLE check_ins ALTER COLUMN server_id SET NOT NULL;
    EXCEPTION
        WHEN others THEN NULL;
    END;
END $$;

-- Add indexes if they don't exist
DO $$ 
BEGIN
    BEGIN
        CREATE INDEX idx_tasks_server_id ON tasks(server_id);
    EXCEPTION
        WHEN duplicate_table THEN NULL;
    END;
    BEGIN
        CREATE INDEX idx_check_ins_server_id ON check_ins(server_id);
    EXCEPTION
        WHEN duplicate_table THEN NULL;
    END;
END $$; 