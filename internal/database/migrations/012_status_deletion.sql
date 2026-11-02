ALTER TABLE product_statuses ADD COLUMN deleted_at TEXT;
ALTER TABLE product_statuses ADD COLUMN version INTEGER NOT NULL DEFAULT 1;

ALTER TABLE recipes ADD COLUMN status_option_id TEXT REFERENCES product_statuses(id) ON DELETE RESTRICT;
UPDATE recipes SET status_option_id=development_status;
