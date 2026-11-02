CREATE TABLE companies (
    id TEXT PRIMARY KEY,
    chinese_name TEXT NOT NULL DEFAULT '',
    english_name TEXT NOT NULL DEFAULT '',
    aliases TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    created_by TEXT REFERENCES users(id),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    deleted_at TEXT
);
CREATE INDEX companies_active_idx ON companies(deleted_at, updated_at DESC);
ALTER TABLE products ADD COLUMN company_id TEXT REFERENCES companies(id) ON DELETE RESTRICT;
ALTER TABLE products ADD COLUMN brand_name TEXT NOT NULL DEFAULT '';
ALTER TABLE products ADD COLUMN item_kind TEXT NOT NULL DEFAULT 'product' CHECK(item_kind IN ('product','tool'));
ALTER TABLE products ADD COLUMN use_category TEXT NOT NULL DEFAULT '';
ALTER TABLE products ADD COLUMN tool_settings_json TEXT NOT NULL DEFAULT '{}';
CREATE INDEX products_kind_idx ON products(item_kind, deleted_at, created_at DESC);
CREATE INDEX products_company_idx ON products(company_id);
INSERT INTO companies(id,chinese_name,created_at,updated_at)
SELECT lower(hex(randomblob(16))),trim(company),strftime('%Y-%m-%dT%H:%M:%fZ','now'),strftime('%Y-%m-%dT%H:%M:%fZ','now')
FROM products WHERE trim(company)<>'' GROUP BY trim(company);
UPDATE products SET company_id=(SELECT id FROM companies WHERE chinese_name=trim(products.company)),company=trim(company) WHERE trim(company)<>'';

CREATE TABLE tool_use_categories(name TEXT PRIMARY KEY COLLATE NOCASE, created_at TEXT NOT NULL);
INSERT INTO tool_use_categories VALUES ('主动',CURRENT_TIMESTAMP),('被动',CURRENT_TIMESTAMP);

CREATE TABLE recipe_groups (
    id TEXT PRIMARY KEY,
    output_id TEXT NOT NULL UNIQUE REFERENCES products(id) ON DELETE RESTRICT,
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    deleted_at TEXT
);
CREATE TABLE recipes (
    id TEXT PRIMARY KEY,
    group_id TEXT NOT NULL REFERENCES recipe_groups(id) ON DELETE CASCADE,
    name TEXT NOT NULL DEFAULT '',
    output_quantity INTEGER NOT NULL DEFAULT 1 CHECK(output_quantity>0),
    notes TEXT NOT NULL DEFAULT '',
    verification TEXT NOT NULL DEFAULT 'untested' CHECK(verification IN ('untested','verified','invalid')),
    development_status TEXT NOT NULL DEFAULT 'concept' CHECK(development_status IN ('concept','design','production','completed')),
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    deleted_at TEXT
);
CREATE INDEX recipes_group_idx ON recipes(group_id,deleted_at,created_at);
CREATE TABLE recipe_ingredients (
    id TEXT PRIMARY KEY,
    recipe_id TEXT NOT NULL REFERENCES recipes(id) ON DELETE CASCADE,
    product_id TEXT REFERENCES products(id) ON DELETE RESTRICT,
    category TEXT NOT NULL DEFAULT '',
    quantity INTEGER NOT NULL CHECK(quantity>0),
    position INTEGER NOT NULL,
    CHECK ((product_id IS NOT NULL AND category='') OR (product_id IS NULL AND category<>''))
);
CREATE INDEX recipe_material_idx ON recipe_ingredients(product_id);
CREATE INDEX recipe_ingredients_recipe_idx ON recipe_ingredients(recipe_id,position);
CREATE TABLE setting_idea_links (
    id TEXT PRIMARY KEY,
    idea_id TEXT NOT NULL REFERENCES ideas(id) ON DELETE CASCADE,
    entity_type TEXT NOT NULL CHECK(entity_type IN ('company','recipe')),
    entity_id TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'candidate' CHECK(status IN ('candidate','adopted','rejected')),
    created_at TEXT NOT NULL,
    UNIQUE(idea_id,entity_type,entity_id)
);

ALTER TABLE attachments RENAME TO attachments_legacy;
CREATE TABLE attachments (
    id TEXT PRIMARY KEY,
    idea_id TEXT REFERENCES ideas(id) ON DELETE CASCADE,
    product_id TEXT REFERENCES products(id) ON DELETE CASCADE,
    company_id TEXT REFERENCES companies(id) ON DELETE CASCADE,
    uploader_id TEXT NOT NULL REFERENCES users(id),
    original_name TEXT NOT NULL,
    mime_type TEXT NOT NULL,
    size INTEGER NOT NULL,
    file_path TEXT NOT NULL UNIQUE,
    thumbnail_path TEXT,
    created_at TEXT NOT NULL,
    client_id TEXT,
    CHECK ((idea_id IS NOT NULL)+(product_id IS NOT NULL)+(company_id IS NOT NULL)=1)
);
INSERT INTO attachments(id,idea_id,product_id,uploader_id,original_name,mime_type,size,file_path,thumbnail_path,created_at,client_id)
SELECT id,idea_id,product_id,uploader_id,original_name,mime_type,size,file_path,thumbnail_path,created_at,client_id FROM attachments_legacy;
DROP TABLE attachments_legacy;
CREATE INDEX attachments_idea_idx ON attachments(idea_id);
CREATE INDEX attachments_product_idx ON attachments(product_id);
CREATE UNIQUE INDEX attachments_company_idx ON attachments(company_id) WHERE company_id IS NOT NULL;
CREATE UNIQUE INDEX attachments_client_id_idx ON attachments(uploader_id,client_id) WHERE client_id IS NOT NULL;

ALTER TABLE revisions RENAME TO revisions_legacy;
CREATE TABLE revisions (
    id TEXT PRIMARY KEY,
    entity_type TEXT NOT NULL CHECK(entity_type IN ('idea','product','company','recipe','recipe_group')),
    entity_id TEXT NOT NULL,
    version INTEGER NOT NULL,
    actor_id TEXT REFERENCES users(id),
    action TEXT NOT NULL,
    before_data TEXT,
    after_data TEXT NOT NULL,
    created_at TEXT NOT NULL
);
INSERT INTO revisions SELECT * FROM revisions_legacy;
DROP TABLE revisions_legacy;
CREATE INDEX revisions_entity_idx ON revisions(entity_type,entity_id,version DESC);

INSERT INTO revisions(id,entity_type,entity_id,version,action,after_data,created_at)
SELECT lower(hex(randomblob(16))),'company',id,1,'create',
       json_object('chinese_name',chinese_name,'english_name',english_name,'aliases',aliases,'description',description),created_at
FROM companies;
