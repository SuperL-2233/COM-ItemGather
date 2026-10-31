ALTER TABLE attachments ADD COLUMN client_id TEXT;
CREATE UNIQUE INDEX attachments_submission_idx ON attachments(uploader_id,client_id)
    WHERE client_id IS NOT NULL;
