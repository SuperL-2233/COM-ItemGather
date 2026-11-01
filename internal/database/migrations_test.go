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
	_, err = db.Exec(`CREATE TABLE products (
		id TEXT PRIMARY KEY,
		internal_name TEXT NOT NULL,
		final_brand_name TEXT NOT NULL DEFAULT '',
		company TEXT NOT NULL DEFAULT ''
	);
	INSERT INTO products(id,internal_name,final_brand_name) VALUES
		('fallback','原内部名称',''),
		('brand','不应采用的内部名称','原最终品牌名');
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
}
