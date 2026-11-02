ALTER TABLE product_categories ADD COLUMN position INTEGER NOT NULL DEFAULT 0;
WITH ordered AS MATERIALIZED (
    SELECT name,row_number() OVER (ORDER BY name COLLATE NOCASE)-1 AS position
    FROM product_categories WHERE deleted_at IS NULL
)
UPDATE product_categories SET position=(SELECT position FROM ordered WHERE ordered.name=product_categories.name)
WHERE deleted_at IS NULL;

WITH ordered AS MATERIALIZED (
    SELECT id,row_number() OVER (ORDER BY position,created_at,id)-1 AS position
    FROM product_statuses WHERE deleted_at IS NULL
)
UPDATE product_statuses SET position=(SELECT position FROM ordered WHERE ordered.id=product_statuses.id)
WHERE deleted_at IS NULL;

ALTER TABLE product_statuses ADD COLUMN color TEXT NOT NULL DEFAULT '#7c3aed';
UPDATE product_statuses SET color=CASE id
    WHEN 'concept' THEN '#6d7169'
    WHEN 'design' THEN '#3d65a5'
    WHEN 'production' THEN '#8c611c'
    WHEN 'completed' THEN '#276749'
    ELSE color END;
