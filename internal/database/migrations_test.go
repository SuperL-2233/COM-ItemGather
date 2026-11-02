package database

import (
	"context"
	"path/filepath"
	"testing"
)

func TestProductChineseNameMigration(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "legacy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, name := range []string{"001_init.sql", "002_ai_source_version.sql", "003_attachment_idempotency.sql", "004_ai_attempt_budget.sql", "005_product_company.sql"} {
		body, e := migrations.ReadFile("migrations/" + name)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = db.Exec(string(body)); e != nil {
			t.Fatal(e)
		}
	}
	_, err = db.Exec(`INSERT INTO users(id,username,nickname,password_hash,created_at,updated_at) VALUES('u','legacy','旧用户','test','2026-01-01','2026-01-01');
	INSERT INTO products(id,internal_name,final_brand_name,company,created_by,created_at,updated_at) VALUES
	('fallback','原内部名称','',' 浮泡饮料 ','u','2026-01-01','2026-01-01'),
	('brand','不应采用的内部名称','原最终品牌名','浮泡饮料','u','2026-01-01','2026-01-01');
	INSERT INTO ideas(id,author_id,created_at,updated_at) VALUES('i','u','2026-01-01','2026-01-01');
	INSERT INTO attachments(id,idea_id,uploader_id,original_name,mime_type,size,file_path,created_at,client_id) VALUES('a','i','u','old.png','image/png',10,'old.png','2026-01-01','submission');
	INSERT INTO revisions(id,entity_type,entity_id,version,actor_id,action,after_data,created_at) VALUES('r','product','fallback',1,'u','create','{}','2026-01-01');
	CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP);
	INSERT INTO schema_migrations(version) VALUES (1),(2),(3),(4),(5);`)
	if err != nil {
		t.Fatal(err)
	}
	if err = Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]string{"fallback": "原内部名称", "brand": "原最终品牌名"} {
		var got string
		if err = db.QueryRow("SELECT chinese_name FROM products WHERE id=?", id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("product %s chinese_name = %q, want %q", id, got, want)
		}
	}
	var legacyColumns int
	if err = db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('products') WHERE name IN ('internal_name','final_brand_name')").Scan(&legacyColumns); err != nil {
		t.Fatal(err)
	}
	if legacyColumns != 0 {
		t.Fatal("legacy product name columns remain after migration")
	}
	var count int
	if err = db.QueryRow("SELECT COUNT(*) FROM companies WHERE chinese_name='浮泡饮料'").Scan(&count); err != nil || count != 1 {
		t.Fatalf("legacy companies were not consolidated: count=%d err=%v", count, err)
	}
	if err = db.QueryRow("SELECT COUNT(*) FROM products WHERE company_id IS NOT NULL AND item_kind='product'").Scan(&count); err != nil || count != 2 {
		t.Fatalf("legacy references lost: count=%d err=%v", count, err)
	}
	if err = db.QueryRow("SELECT COUNT(*) FROM attachments WHERE id='a' AND client_id='submission'").Scan(&count); err != nil || count != 1 {
		t.Fatalf("legacy attachment lost: count=%d err=%v", count, err)
	}
	if err = db.QueryRow("SELECT COUNT(*) FROM revisions WHERE id='r'").Scan(&count); err != nil || count != 1 {
		t.Fatalf("legacy revision lost: count=%d err=%v", count, err)
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
		t.Fatal("migration left invalid foreign keys")
	}
}

func TestProductOptionsMigrationPreservesCatalogReferences(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() >= "009_" {
			continue
		}
		body, e := migrations.ReadFile("migrations/" + entry.Name())
		if e != nil {
			t.Fatal(e)
		}
		if _, e = db.Exec(string(body)); e != nil {
			t.Fatal(e)
		}
	}
	_, err = db.Exec(`CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY,applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP);
 INSERT INTO schema_migrations(version) VALUES(1),(2),(3),(4),(5),(6),(7),(8);
 INSERT INTO users(id,username,nickname,password_hash,created_at,updated_at) VALUES('u','u','成员','hash','2026-01-01','2026-01-01');
 INSERT INTO products(id,chinese_name,category,development_status,created_by,created_at,updated_at) VALUES('p','旧商品','旧分类','completed','u','2026-01-01','2026-01-01');
 INSERT INTO ideas(id,author_id,created_at,updated_at) VALUES('i','u','2026-01-01','2026-01-01');
 INSERT INTO idea_product_links(id,idea_id,product_id,created_by,created_at,updated_at) VALUES('l','i','p','u','2026-01-01','2026-01-01');
 INSERT INTO product_attributes(id,product_id,key,value) VALUES('attr','p','效果','旧效果');
 INSERT INTO attachments(id,product_id,uploader_id,original_name,mime_type,size,file_path,created_at) VALUES('a','p','u','p.png','image/png',5,'p.png','2026-01-01');
 INSERT INTO recipe_groups(id,output_id,created_by,created_at,updated_at) VALUES('g','p','u','2026-01-01','2026-01-01');
 INSERT INTO recipes(id,group_id,created_by,created_at,updated_at) VALUES('r','g','u','2026-01-01','2026-01-01');
 INSERT INTO recipe_ingredients(id,recipe_id,product_id,quantity,position) VALUES('m','r','p',2,0);`)
	if err != nil {
		t.Fatal(err)
	}
	if err = Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	var state string
	if err = db.QueryRow("SELECT status_option_id FROM products WHERE id='p'").Scan(&state); err != nil || state != "completed" {
		t.Fatalf("old status changed: %q %v", state, err)
	}
	for _, table := range []string{"products", "idea_product_links", "product_attributes", "attachments", "recipe_groups", "recipes", "recipe_ingredients", "product_categories"} {
		var count int
		if err = db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 1 {
			t.Fatalf("table %s lost data: %d %v", table, count, err)
		}
	}
	rows, err := db.Query("PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("migration broke a catalog foreign key")
	}
}
