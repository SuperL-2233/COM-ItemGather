package database

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
)

func TestMCPMigrationRetiresSnapshotTokensAndPreservesContent(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "upgrade.db"))
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
		if entry.Name() >= "015_" {
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
	_, err = db.Exec(`INSERT INTO users(id,username,nickname,password_hash,created_at,updated_at) VALUES('u','migration-user','成员','hash','2026-01-01','2026-01-01');
INSERT INTO ideas(id,author_id,content,created_at,updated_at) VALUES('idea','u','保留的创意','2026-01-01','2026-01-01');
INSERT INTO agent_tokens(id,token_hash,created_by,created_at,expires_at) VALUES('legacy','hash','u','2026-01-01','2099-01-01');`)
	if err != nil {
		t.Fatal(err)
	}
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	var revoked, content string
	if err := db.QueryRow("SELECT COALESCE(revoked_at,'') FROM agent_tokens WHERE id='legacy'").Scan(&revoked); err != nil || revoked == "" {
		t.Fatalf("legacy credentials were not retired: %q %v", revoked, err)
	}
	if err := db.QueryRow("SELECT content FROM ideas WHERE id='idea'").Scan(&content); err != nil || content != "保留的创意" {
		t.Fatalf("migration changed creative content: %q %v", content, err)
	}
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal("migration is not idempotent:", err)
	}
}
