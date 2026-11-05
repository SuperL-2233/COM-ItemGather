-- Keep the constrained legacy column so item identity and all foreign keys stay
-- intact while extending the supported kinds.
DROP INDEX products_kind_idx;
ALTER TABLE products RENAME COLUMN item_kind TO legacy_item_kind;
ALTER TABLE products ADD COLUMN item_kind TEXT NOT NULL DEFAULT 'product'
    CHECK(item_kind IN ('product','tool','other') AND
          (item_kind<>'other' OR (company_id IS NULL AND company='')));
UPDATE products SET item_kind=legacy_item_kind;
CREATE INDEX products_kind_idx ON products(item_kind,deleted_at,created_at DESC);

CREATE TABLE other_item_categories (
    name TEXT PRIMARY KEY COLLATE NOCASE,
    created_at TEXT NOT NULL,
    deleted_at TEXT,
    version INTEGER NOT NULL DEFAULT 1,
    position INTEGER NOT NULL DEFAULT 0
);
