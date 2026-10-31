-- Before this migration, running requests were counted after completion.
UPDATE ai_tasks SET attempts = attempts + 1 WHERE status = 'running';
UPDATE ai_tasks SET max_attempts = 4 WHERE max_attempts > 4;
