-- A product is a parent. Every flavor/style is a sibling child, never the
-- product itself. Existing concrete item IDs and all their references survive.
ALTER TABLE products ADD COLUMN parent_product_id TEXT REFERENCES products(id) ON DELETE RESTRICT;
ALTER TABLE products ADD COLUMN parent_deleted_at TEXT NOT NULL DEFAULT '';
DROP INDEX products_variant_name_idx;

CREATE TEMP TABLE variant_parent_map (
    group_id TEXT PRIMARY KEY,
    source_id TEXT NOT NULL,
    parent_id TEXT NOT NULL,
    create_parent INTEGER NOT NULL
);
INSERT INTO variant_parent_map(group_id,source_id,parent_id,create_parent)
SELECT g.id,
    (SELECT p.id FROM products p WHERE p.variant_group_id=g.id
        ORDER BY p.deleted_at IS NOT NULL,p.created_at,p.id LIMIT 1),
    COALESCE((SELECT p.id FROM products p WHERE p.variant_group_id=g.id
        AND p.variant_name='' AND p.variant_description=''
        AND (p.deleted_at IS NULL OR NOT EXISTS(SELECT 1 FROM products active
            WHERE active.variant_group_id=g.id AND active.deleted_at IS NULL))
        ORDER BY p.deleted_at IS NOT NULL,p.created_at,p.id LIMIT 1),lower(hex(randomblob(16)))),
    NOT EXISTS(SELECT 1 FROM products p WHERE p.variant_group_id=g.id
        AND p.variant_name='' AND p.variant_description=''
        AND (p.deleted_at IS NULL OR NOT EXISTS(SELECT 1 FROM products active
            WHERE active.variant_group_id=g.id AND active.deleted_at IS NULL)))
FROM product_variant_groups g WHERE EXISTS(SELECT 1 FROM products p WHERE p.variant_group_id=g.id);

-- Groups made entirely of named flavors need a separate parent container.
-- Its status starts independently; every original flavor keeps its own state,
-- photos, attributes, history, recipe references and inspiration links.
INSERT INTO products(id,chinese_name,english_name,company_id,company,category,description,
    development_status,status_option_id,created_by,created_at,updated_at,deleted_at,variant_group_id)
SELECT m.parent_id,p.chinese_name,p.english_name,p.company_id,p.company,p.category,p.description,
    'concept',COALESCE((SELECT id FROM product_statuses WHERE deleted_at IS NULL
        ORDER BY id<>'concept',position,id LIMIT 1),p.status_option_id),
    p.created_by,p.created_at,p.updated_at,
    CASE WHEN EXISTS(SELECT 1 FROM products a WHERE a.variant_group_id=m.group_id AND a.deleted_at IS NULL)
        THEN NULL ELSE p.deleted_at END,m.group_id
FROM variant_parent_map m JOIN products p ON p.id=m.source_id WHERE m.create_parent=1;

UPDATE products SET parent_product_id=(SELECT parent_id FROM variant_parent_map m WHERE m.group_id=products.variant_group_id)
WHERE variant_group_id IN (SELECT group_id FROM variant_parent_map)
    AND id<>(SELECT parent_id FROM variant_parent_map m WHERE m.group_id=products.variant_group_id);

DROP TABLE variant_parent_map;
CREATE INDEX products_parent_idx ON products(parent_product_id,deleted_at,created_at,id);
CREATE UNIQUE INDEX products_variant_name_idx ON products(parent_product_id,variant_name COLLATE NOCASE)
    WHERE parent_product_id IS NOT NULL AND deleted_at IS NULL;
