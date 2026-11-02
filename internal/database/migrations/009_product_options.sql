CREATE TABLE product_categories (
    name TEXT PRIMARY KEY COLLATE NOCASE,
    created_at TEXT NOT NULL
);
INSERT OR IGNORE INTO product_categories(name,created_at)
SELECT trim(category),min(created_at) FROM products WHERE trim(category)<>'' GROUP BY trim(category);

CREATE TABLE product_statuses (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE COLLATE NOCASE,
    position INTEGER NOT NULL DEFAULT 4,
    created_at TEXT NOT NULL
);
INSERT INTO product_statuses(id,name,position,created_at) VALUES
('concept','构思中',0,CURRENT_TIMESTAMP),
('design','设计中',1,CURRENT_TIMESTAMP),
('production','制作中',2,CURRENT_TIMESTAMP),
('completed','已完成',3,CURRENT_TIMESTAMP);

-- Keep the original constrained column for compatibility without rebuilding
-- the item table and its attachment, inspiration and recipe foreign keys.
ALTER TABLE products ADD COLUMN status_option_id TEXT REFERENCES product_statuses(id) ON DELETE RESTRICT;
UPDATE products SET status_option_id=development_status;
