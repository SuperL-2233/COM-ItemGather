package database

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
)

func TestReadmeAuthorsMigrationPreservesIdentitiesSelectionsAndDrafts(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "readme-authors.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY,applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)"); err != nil {
		t.Fatal(err)
	}
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() >= "020_" {
			continue
		}
		body, err := migrations.ReadFile("migrations/" + entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(string(body)); err != nil {
			t.Fatal(err)
		}
		var version int
		if _, err = fmt.Sscanf(entry.Name(), "%d_", &version); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec("INSERT INTO schema_migrations(version) VALUES(?)", version); err != nil {
			t.Fatal(err)
		}
	}
	_, err = db.Exec(`INSERT INTO users(id,username,nickname,password_hash,created_at,updated_at) VALUES
('u1','one','成员一','hash','2026-01-01','2026-01-01'),
('u2','two','成员二','hash','2026-01-01','2026-01-01'),
('u3','three','成员三','hash','2026-01-01','2026-01-01');
INSERT INTO readme_authors(user_id,name,email) VALUES
('u1','作者甲','ONE@example.com'),('u2','作者甲','one@example.com'),('u3','作者乙','two@example.com');
INSERT INTO readme_drafts(user_id,repository,branch,content,base_content,base_sha,version,updated_at) VALUES
('u1','owner/game','main','# 我的草稿','# 基线','base-sha',4,'2026-01-01');`)
	if err != nil {
		t.Fatal(err)
	}
	if err = Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.QueryRow("SELECT COUNT(*) FROM readme_authors").Scan(&count); err != nil || count != 2 {
		t.Fatalf("identical identities were not migrated: count=%d err=%v", count, err)
	}
	choices := map[string]string{}
	for _, userID := range []string{"u1", "u2", "u3"} {
		var authorID string
		if err = db.QueryRow("SELECT author_id FROM readme_author_choices WHERE user_id=?", userID).Scan(&authorID); err != nil {
			t.Fatal(err)
		}
		choices[userID] = authorID
	}
	if choices["u1"] != choices["u2"] || choices["u1"] == choices["u3"] {
		t.Fatal("original author selections were not preserved")
	}
	var content, baseSHA string
	var version int
	if err = db.QueryRow("SELECT content,base_sha,version FROM readme_drafts WHERE user_id='u1'").Scan(&content, &baseSHA, &version); err != nil || content != "# 我的草稿" || baseSHA != "base-sha" || version != 4 {
		t.Fatal("draft changed during migration", err)
	}
	if _, err = db.Exec("DELETE FROM readme_authors WHERE id=?", choices["u1"]); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow("SELECT COUNT(*) FROM readme_author_choices WHERE user_id IN ('u1','u2') AND author_id IS NULL").Scan(&count); err != nil || count != 2 {
		t.Fatal("deleted author selections were not cleared", err)
	}
	if err = Migrate(context.Background(), db); err != nil {
		t.Fatal("migration was not idempotent", err)
	}
}
