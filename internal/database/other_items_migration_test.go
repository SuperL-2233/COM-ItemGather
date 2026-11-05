package database

import (
	"context"
	"testing"
)

func TestOtherItemsMigrationPreservesExistingItemsAndReferences(t *testing.T) {
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
		if entry.Name() >= "021_" {
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
INSERT INTO companies(id,chinese_name,created_at,updated_at) VALUES('c','原公司','2026-01-01','2026-01-01');
INSERT INTO products(id,chinese_name,company_id,company,item_kind,category,created_by,created_at,updated_at,version) VALUES
('p','原商品','c','原公司','product','原分类','u','2026-01-01','2026-01-01',7),
('t','原道具',NULL,'','tool','','u','2026-01-01','2026-01-01',3);
INSERT INTO ideas(id,content,author_id,created_at,updated_at) VALUES('i','原灵感','u','2026-01-01','2026-01-01');
INSERT INTO idea_product_links(id,idea_id,product_id,created_by,created_at,updated_at) VALUES('l','i','t','u','2026-01-01','2026-01-01');
INSERT INTO product_attributes(id,product_id,key,value) VALUES('pa','p','原属性','原值');
INSERT INTO attachments(id,product_id,uploader_id,original_name,mime_type,size,file_path,created_at) VALUES('a','p','u','原图.png','image/png',100,'a.png','2026-01-01');
INSERT INTO recipe_groups(id,output_id,created_by,created_at,updated_at) VALUES('g','t','u','2026-01-01','2026-01-01');
INSERT INTO recipes(id,group_id,created_by,created_at,updated_at) VALUES('r','g','u','2026-01-01','2026-01-01');
INSERT INTO recipe_ingredients(id,recipe_id,product_id,quantity,position) VALUES('ri','r','p',2,0);
INSERT INTO revisions(id,entity_type,entity_id,version,action,after_data,created_at) VALUES('v','product','p',7,'update','{"chinese_name":"原商品"}','2026-01-01');`)
	if err != nil {
		t.Fatal(err)
	}
	if err = Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	var kind, company, name string
	var version int
	if err = db.QueryRow("SELECT item_kind,company,chinese_name,version FROM products WHERE id='p'").Scan(&kind, &company, &name, &version); err != nil || kind != "product" || company != "原公司" || name != "原商品" || version != 7 {
		t.Fatalf("product changed: %s %s %s %d %v", kind, company, name, version, err)
	}
	if err = db.QueryRow("SELECT item_kind,version FROM products WHERE id='t'").Scan(&kind, &version); err != nil || kind != "tool" || version != 3 {
		t.Fatalf("tool changed: %s %d %v", kind, version, err)
	}
	for _, table := range []string{"idea_product_links", "product_attributes", "attachments", "recipe_groups", "recipes", "recipe_ingredients", "revisions"} {
		var count int
		if err = db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 1 {
			t.Fatalf("lost %s: %d %v", table, count, err)
		}
	}
	if _, err = db.Exec("INSERT INTO products(id,item_kind,created_by,created_at,updated_at) VALUES('o','other','u','2026-01-01','2026-01-01')"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("UPDATE products SET company_id='c',company='原公司' WHERE id='o'"); err == nil {
		t.Fatal("other items must never acquire a company")
	}
	if err = Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query("PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("migration broke an existing reference")
	}
}
