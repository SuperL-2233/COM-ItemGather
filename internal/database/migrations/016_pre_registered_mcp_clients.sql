-- OpenAI supports predefined public OAuth clients as well as CIMD and DCR.
-- Register only the published stable identities and exact callbacks, so these
-- clients do not depend on outbound access to ChatGPT during authorization.
ALTER TABLE mcp_oauth_clients ADD COLUMN registration_type TEXT NOT NULL DEFAULT 'dcr'
    CHECK (registration_type IN ('dcr', 'cimd', 'pre_registered'));
UPDATE mcp_oauth_clients SET registration_type='cimd' WHERE id LIKE 'https://%';

INSERT INTO mcp_oauth_clients(id,name,redirect_uris,metadata_at,created_at,registration_type) VALUES
    ('https://chatgpt.com/oauth/client.json','ChatGPT',
     '["https://chatgpt.com/connector_platform_oauth_redirect"]',
     strftime('%Y-%m-%dT%H:%M:%fZ','now'),strftime('%Y-%m-%dT%H:%M:%fZ','now'),'pre_registered'),
    ('https://chatgpt.com/oauth/codex/client.json','Codex',
     '["http://127.0.0.1/callback","http://localhost/callback"]',
     strftime('%Y-%m-%dT%H:%M:%fZ','now'),strftime('%Y-%m-%dT%H:%M:%fZ','now'),'pre_registered')
ON CONFLICT(id) DO UPDATE SET name=excluded.name,redirect_uris=excluded.redirect_uris,
    metadata_at=excluded.metadata_at,registration_type=excluded.registration_type;
