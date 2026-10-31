ALTER TABLE ai_tasks ADD COLUMN source_version INTEGER NOT NULL DEFAULT 0;
UPDATE ai_tasks SET max_attempts = 4 WHERE max_attempts > 4;

-- Existing tasks have no reliable record-version snapshot. Keep their manual
-- fields protected until a newly requested task explicitly permits replacement.
