package database

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
)

func TestMCPPreRegisteredClientsUpgradePreservesConnections(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "mcp-clients.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() >= "016_" {
			continue
		}
		body, err := migrations.ReadFile("migrations/" + entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(body)); err != nil {
			t.Fatal(err)
		}
		var version int
		if _, err := fmt.Sscanf(entry.Name(), "%d_", &version); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("INSERT INTO schema_migrations(version) VALUES(?)", version); err != nil {
			t.Fatal(err)
		}
	}
	_, err = db.Exec(`INSERT INTO mcp_oauth_clients(id,name,redirect_uris,metadata_at,created_at) VALUES
('https://chatgpt.com/oauth/client.json','Old ChatGPT','["https://old.example/callback"]','2000-01-01','2026-01-01'),
('https://other.example/client.json','Other CIMD','["https://other.example/callback"]','2026-01-01','2026-01-01'),
('mcp_existing','Existing DCR','["http://127.0.0.1/callback"]','2026-01-01','2026-01-01');
INSERT INTO users(id,username,nickname,password_hash,created_at,updated_at) VALUES('u','migration-user','成员','hash','2026-01-01','2026-01-01');
INSERT INTO mcp_oauth_grants(id,user_id,client_id,client_name,resource,scope,created_at,expires_at) VALUES
('grant','u','https://chatgpt.com/oauth/client.json','ChatGPT','https://ideas.example/api/mcp','creative-library:read','2026-01-01','2099-01-01');
INSERT INTO mcp_oauth_tokens(id,grant_id,access_hash,data) VALUES('token','grant','existing-access-hash','{}');`)
	if err != nil {
		t.Fatal(err)
	}
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	for id, expected := range map[string]string{
		"https://chatgpt.com/oauth/client.json":       "pre_registered",
		"https://chatgpt.com/oauth/codex/client.json": "pre_registered",
		"https://other.example/client.json":           "cimd",
		"mcp_existing":                                "dcr",
	} {
		var actual string
		if err := db.QueryRow("SELECT registration_type FROM mcp_oauth_clients WHERE id=?", id).Scan(&actual); err != nil || actual != expected {
			t.Fatalf("client %s: type=%q, expected=%q, error=%v", id, actual, expected, err)
		}
	}
	var redirects, createdAt, accessHash string
	if err := db.QueryRow("SELECT redirect_uris,created_at FROM mcp_oauth_clients WHERE id='https://chatgpt.com/oauth/client.json'").Scan(&redirects, &createdAt); err != nil || redirects != `["https://chatgpt.com/connector_platform_oauth_redirect"]` || createdAt != "2026-01-01" {
		t.Fatalf("stable client was not updated in place: %q %q %v", redirects, createdAt, err)
	}
	if err := db.QueryRow("SELECT access_hash FROM mcp_oauth_tokens t JOIN mcp_oauth_grants g ON g.id=t.grant_id WHERE g.id='grant'").Scan(&accessHash); err != nil || accessHash != "existing-access-hash" {
		t.Fatalf("existing connection or token was changed: %q %v", accessHash, err)
	}
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal("migration is not idempotent:", err)
	}
}
