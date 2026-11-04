package database

import (
	"context"
	"encoding/json"
	"testing"
)

func TestSimplifyIdeasRemovesRetiredDataAndHistory(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec("CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY,applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)"); err != nil {
		t.Fatal(err)
	}
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	for index, entry := range entries {
		if entry.Name() >= "018_" {
			continue
		}
		body, err := migrations.ReadFile("migrations/" + entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(string(body)); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec("INSERT INTO schema_migrations(version) VALUES(?)", index+1); err != nil {
			t.Fatal(err)
		}
	}
	_, err = db.Exec(`INSERT INTO users(id,username,nickname,password_hash,created_at,updated_at) VALUES('u','u','成员','hash','2026-01-01','2026-01-01');
INSERT INTO ideas(id,author_id,content,idea_types_json,product_directions_json,name_candidates_json,tags_json,manual_fields_json,created_at,updated_at)
VALUES('active','u','保留正文','["玩法"]','["饮料"]','["旧名称"]','["旧标签"]','["content","name_candidates","tags","attributes","summary"]','2026-01-01','2026-01-01'),
('trash','u','回收站正文','[]','[]','["回收站旧名称"]','["回收站旧标签"]','["tags","attributes"]','2026-01-01','2026-01-01');
UPDATE ideas SET deleted_at='2026-01-02' WHERE id='trash';
INSERT INTO tags(id,name,created_at) VALUES('tag','旧标签','2026-01-01');
INSERT INTO idea_tags(idea_id,tag_id) VALUES('active','tag');
INSERT INTO idea_attributes(id,idea_id,key,value) VALUES('a','active','旧属性','旧值'),('b','trash','旧属性','旧值');
INSERT INTO products(id,chinese_name,created_by,created_at,updated_at) VALUES('p','保留商品','u','2026-01-01','2026-01-01');
INSERT INTO product_attributes(id,product_id,key,value) VALUES('pa','p','商品属性','保留值');`)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := `{"content":"保留正文","idea_types":["玩法"],"product_directions":["饮料"],"name_candidates":["旧名称"],"tags":["旧标签"],"attributes":[{"key":"旧属性","value":"旧值"}],"manual_fields":["content","tags","attributes","name_candidates","summary"]}`
	for _, item := range []struct{ id, kind, entity string }{{"create", "idea", "active"}, {"edit", "idea", "active"}, {"deleted", "idea", "trash"}, {"product", "product", "p"}} {
		var before any = snapshot
		if item.id == "create" {
			before = nil
		}
		if _, err = db.Exec("INSERT INTO revisions(id,entity_type,entity_id,version,action,before_data,after_data,created_at) VALUES(?,?,?,1,'update',?,?, '2026-01-01')", item.id, item.kind, item.entity, before, snapshot); err != nil {
			t.Fatal(err)
		}
	}
	if err = Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('idea_attributes','idea_tags','tags')").Scan(&count); err != nil || count != 0 {
		t.Fatalf("retired tables remain: count=%d err=%v", count, err)
	}
	if err = db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('ideas') WHERE name IN ('name_candidates_json','tags_json')").Scan(&count); err != nil || count != 0 {
		t.Fatalf("retired columns remain: count=%d err=%v", count, err)
	}
	for _, id := range []string{"create", "edit", "deleted"} {
		var before *string
		var after string
		if err = db.QueryRow("SELECT before_data,after_data FROM revisions WHERE id=?", id).Scan(&before, &after); err != nil {
			t.Fatal(err)
		}
		if id == "create" && before != nil {
			t.Fatal("creation history lost its null before snapshot")
		}
		snapshots := []string{after}
		if before != nil {
			snapshots = append(snapshots, *before)
		}
		for _, raw := range snapshots {
			var value map[string]json.RawMessage
			if err = json.Unmarshal([]byte(raw), &value); err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"name_candidates", "tags", "attributes"} {
				if _, exists := value[key]; exists {
					t.Fatalf("history retained %s: %s", key, raw)
				}
			}
			if string(value["content"]) != `"保留正文"` || string(value["manual_fields"]) != `["content","summary"]` || string(value["idea_types"]) != `["玩法"]` || string(value["product_directions"]) != `["饮料"]` {
				t.Fatalf("remaining history fields changed: %s", raw)
			}
		}
	}
	var content, types, directions, manual string
	if err = db.QueryRow("SELECT content,idea_types_json,product_directions_json,manual_fields_json FROM ideas WHERE id='active'").Scan(&content, &types, &directions, &manual); err != nil || content != "保留正文" || types != `["玩法"]` || directions != `["饮料"]` || manual != `["content","summary"]` {
		t.Fatalf("remaining idea fields changed: %q %q %q %q err=%v", content, types, directions, manual, err)
	}
	if err = db.QueryRow("SELECT manual_fields_json FROM ideas WHERE id='trash'").Scan(&manual); err != nil || manual != "[]" {
		t.Fatalf("trash manual locks were not cleared: %s %v", manual, err)
	}
	var productSnapshot, productValue string
	if err = db.QueryRow("SELECT after_data FROM revisions WHERE id='product'").Scan(&productSnapshot); err != nil || productSnapshot != snapshot {
		t.Fatalf("product history changed: %s %v", productSnapshot, err)
	}
	if err = db.QueryRow("SELECT value FROM product_attributes WHERE id='pa'").Scan(&productValue); err != nil || productValue != "保留值" {
		t.Fatalf("product attribute changed: %s %v", productValue, err)
	}
	if err = Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow("SELECT COUNT(*) FROM ideas").Scan(&count); err != nil || count != 2 {
		t.Fatalf("migration damaged ideas: count=%d err=%v", count, err)
	}
	rows, err := db.Query("PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("migration left invalid references")
	}
}
