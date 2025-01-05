-- Add active column to check_ins table
ALTER TABLE check_ins ADD COLUMN IF NOT EXISTS active BOOLEAN NOT NULL DEFAULT TRUE;

-- Update existing records
UPDATE check_ins SET active = (end_time IS NULL); 