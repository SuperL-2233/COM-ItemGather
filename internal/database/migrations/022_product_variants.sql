-- A group identifies a product; its members retain their original item IDs.
-- Shared fields stay on products for compatibility with existing foreign keys,
-- searches and exports. Application transactions update them together.
CREATE TABLE product_variant_groups (
    id TEXT PRIMARY KEY
);
ALTER TABLE products ADD COLUMN variant_group_id TEXT REFERENCES product_variant_groups(id);
ALTER TABLE products ADD COLUMN variant_name TEXT NOT NULL DEFAULT '';
ALTER TABLE products ADD COLUMN variant_description TEXT NOT NULL DEFAULT '';

-- Never infer groups from names: even identically named legacy items stay apart.
INSERT INTO product_variant_groups(id) SELECT id FROM products WHERE item_kind='product';
UPDATE products SET variant_group_id=id WHERE item_kind='product';
CREATE INDEX products_variant_group_idx ON products(variant_group_id,deleted_at,created_at,id);
CREATE UNIQUE INDEX products_variant_name_idx ON products(variant_group_id,variant_name COLLATE NOCASE)
    WHERE variant_group_id IS NOT NULL AND deleted_at IS NULL;
