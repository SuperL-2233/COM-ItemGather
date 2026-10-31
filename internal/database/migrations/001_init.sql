CREATE TABLE users (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL COLLATE NOCASE UNIQUE,
    nickname TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('admin', 'member')),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE sessions (
    token_hash TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    csrf_token TEXT NOT NULL,
    device_name TEXT NOT NULL DEFAULT '',
    remember INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    last_seen_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    revoked_at TEXT
);
CREATE INDEX sessions_user_idx ON sessions(user_id);

CREATE TABLE invitations (
    id TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    created_by TEXT NOT NULL REFERENCES users(id),
    expires_at TEXT NOT NULL,
    used_at TEXT,
    used_by TEXT REFERENCES users(id),
    created_at TEXT NOT NULL
);

CREATE TABLE password_reset_tokens (
    id TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_by TEXT,
    expires_at TEXT NOT NULL,
    used_at TEXT,
    created_at TEXT NOT NULL
);

CREATE TABLE ideas (
    id TEXT PRIMARY KEY,
    client_id TEXT,
    original_content TEXT NOT NULL DEFAULT '',
    content TEXT NOT NULL DEFAULT '',
    display_title TEXT NOT NULL DEFAULT '',
    summary TEXT NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT '',
    name_candidates_json TEXT NOT NULL DEFAULT '[]',
    idea_types_json TEXT NOT NULL DEFAULT '[]',
    product_directions_json TEXT NOT NULL DEFAULT '[]',
    tags_json TEXT NOT NULL DEFAULT '[]',
    manual_fields_json TEXT NOT NULL DEFAULT '[]',
    author_id TEXT NOT NULL REFERENCES users(id),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    content_version INTEGER NOT NULL DEFAULT 1,
    ai_status TEXT NOT NULL DEFAULT 'idle' CHECK (ai_status IN ('idle', 'pending', 'running', 'succeeded', 'failed', 'skipped')),
    ai_error TEXT NOT NULL DEFAULT '',
    deleted_at TEXT,
    UNIQUE(author_id, client_id)
);
CREATE INDEX ideas_active_created_idx ON ideas(deleted_at, created_at DESC);
CREATE INDEX ideas_author_idx ON ideas(author_id);

CREATE TABLE idea_attributes (
    id TEXT PRIMARY KEY,
    idea_id TEXT NOT NULL REFERENCES ideas(id) ON DELETE CASCADE,
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    certainty TEXT NOT NULL DEFAULT 'asserted' CHECK (certainty IN ('asserted', 'tentative', 'undecided')),
    position INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idea_attributes_idea_idx ON idea_attributes(idea_id);

CREATE TABLE products (
    id TEXT PRIMARY KEY,
    internal_name TEXT NOT NULL,
    final_brand_name TEXT NOT NULL DEFAULT '',
    category TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    development_status TEXT NOT NULL DEFAULT 'concept' CHECK (development_status IN ('concept', 'design', 'production', 'completed')),
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    deleted_at TEXT
);
CREATE INDEX products_active_created_idx ON products(deleted_at, created_at DESC);

CREATE TABLE product_attributes (
    id TEXT PRIMARY KEY,
    product_id TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    certainty TEXT NOT NULL DEFAULT 'asserted' CHECK (certainty IN ('asserted', 'tentative', 'undecided')),
    position INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX product_attributes_product_idx ON product_attributes(product_id);

CREATE TABLE idea_product_links (
    id TEXT PRIMARY KEY,
    idea_id TEXT NOT NULL REFERENCES ideas(id) ON DELETE CASCADE,
    product_id TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'candidate' CHECK (status IN ('candidate', 'adopted', 'rejected')),
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(idea_id, product_id)
);

CREATE TABLE favorites (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    idea_id TEXT NOT NULL REFERENCES ideas(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL,
    PRIMARY KEY(user_id, idea_id)
);

CREATE TABLE tags (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL COLLATE NOCASE UNIQUE,
    created_at TEXT NOT NULL
);

CREATE TABLE idea_tags (
    idea_id TEXT NOT NULL REFERENCES ideas(id) ON DELETE CASCADE,
    tag_id TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY(idea_id, tag_id)
);

CREATE TABLE attachments (
    id TEXT PRIMARY KEY,
    idea_id TEXT REFERENCES ideas(id) ON DELETE CASCADE,
    product_id TEXT REFERENCES products(id) ON DELETE CASCADE,
    uploader_id TEXT NOT NULL REFERENCES users(id),
    original_name TEXT NOT NULL,
    mime_type TEXT NOT NULL,
    size INTEGER NOT NULL,
    file_path TEXT NOT NULL UNIQUE,
    thumbnail_path TEXT,
    created_at TEXT NOT NULL,
    CHECK ((idea_id IS NOT NULL AND product_id IS NULL) OR (idea_id IS NULL AND product_id IS NOT NULL))
);
CREATE INDEX attachments_idea_idx ON attachments(idea_id);
CREATE INDEX attachments_product_idx ON attachments(product_id);

CREATE TABLE revisions (
    id TEXT PRIMARY KEY,
    entity_type TEXT NOT NULL CHECK (entity_type IN ('idea', 'product')),
    entity_id TEXT NOT NULL,
    version INTEGER NOT NULL,
    actor_id TEXT REFERENCES users(id),
    action TEXT NOT NULL,
    before_data TEXT,
    after_data TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX revisions_entity_idx ON revisions(entity_type, entity_id, version DESC);

CREATE TABLE ai_tasks (
    id TEXT PRIMARY KEY,
    idea_id TEXT NOT NULL REFERENCES ideas(id) ON DELETE CASCADE,
    source_content TEXT NOT NULL,
    source_content_version INTEGER NOT NULL,
    mode TEXT NOT NULL DEFAULT 'normal' CHECK (mode IN ('normal', 'full')),
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'succeeded', 'failed', 'cancelled')),
    attempts INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 4,
    next_attempt_at TEXT NOT NULL,
    leased_until TEXT,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX ai_tasks_schedule_idx ON ai_tasks(status, next_attempt_at);
CREATE UNIQUE INDEX ai_tasks_one_active_idx ON ai_tasks(idea_id) WHERE status IN ('pending', 'running');

CREATE TABLE events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_type TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE audit_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    actor_id TEXT REFERENCES users(id),
    action TEXT NOT NULL,
    entity_type TEXT NOT NULL DEFAULT '',
    entity_id TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);
