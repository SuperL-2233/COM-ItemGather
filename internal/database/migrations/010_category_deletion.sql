ALTER TABLE product_categories ADD COLUMN deleted_at TEXT;
ALTER TABLE product_categories ADD COLUMN version INTEGER NOT NULL DEFAULT 1;
