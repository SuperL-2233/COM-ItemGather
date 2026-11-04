-- Retire the old snapshot credentials without rewriting deployed migrations.
UPDATE agent_tokens SET revoked_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE revoked_at IS NULL;

CREATE TABLE mcp_oauth_clients (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    redirect_uris TEXT NOT NULL,
    metadata_at TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE TABLE mcp_oauth_requests (
    token_hash TEXT PRIMARY KEY,
    parameters TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    consumed_at TEXT
);
CREATE TABLE mcp_oauth_grants (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    client_id TEXT NOT NULL REFERENCES mcp_oauth_clients(id),
    client_name TEXT NOT NULL,
    resource TEXT NOT NULL,
    scope TEXT NOT NULL,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    last_used_at TEXT,
    revoked_at TEXT
);
CREATE INDEX mcp_grants_user_idx ON mcp_oauth_grants(user_id,created_at DESC);
CREATE TABLE mcp_oauth_tokens (
    id TEXT PRIMARY KEY,
    grant_id TEXT NOT NULL REFERENCES mcp_oauth_grants(id) ON DELETE CASCADE,
    code_hash TEXT UNIQUE,
    access_hash TEXT UNIQUE,
    refresh_hash TEXT UNIQUE,
    data TEXT NOT NULL,
    code_used_at TEXT,
    access_revoked_at TEXT,
    refresh_used_at TEXT
);
CREATE INDEX mcp_tokens_grant_idx ON mcp_oauth_tokens(grant_id);
