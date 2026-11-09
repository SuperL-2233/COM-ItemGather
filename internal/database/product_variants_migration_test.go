package database

import (
	"context"
	"encoding/json"
	"testing"
)

func TestProductVariantsMigrationPreservesLegacyDataAndReferences(t *testing.T) {
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
		if entry.Name() >= "022_" {
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
INSERT INTO products(id,chinese_name,english_name,company_id,company,item_kind,category,description,created_by,created_at,updated_at,version,deleted_at) VALUES
('p','同名商品','Legacy','c','原公司','product','饮料','原设定','u','2026-01-01','2026-02-02',7,NULL),
('p2','同名商品','Legacy','c','原公司','product','饮料','另一份设定','u','2026-01-01','2026-02-02',3,NULL),
('p3','','',NULL,'','product','','回收站未命名商品','u','2026-01-01','2026-01-02',4,'2026-01-02'),
('t','原道具','',NULL,'','tool','','','u','2026-01-01','2026-01-01',3,NULL),
('o','其他物品','',NULL,'','other','','','u','2026-01-01','2026-01-01',2,NULL);
INSERT INTO ideas(id,content,author_id,created_at,updated_at) VALUES('i','原灵感','u','2026-01-01','2026-01-01');
INSERT INTO idea_product_links(id,idea_id,product_id,created_by,created_at,updated_at) VALUES('l','i','p','u','2026-01-01','2026-01-01');
INSERT INTO product_attributes(id,product_id,key,value) VALUES('pa','p','原属性','原值');
INSERT INTO attachments(id,product_id,uploader_id,original_name,mime_type,size,file_path,created_at) VALUES('a','p','u','原图.png','image/png',100,'a.png','2026-01-01');
INSERT INTO recipe_groups(id,output_id,created_by,created_at,updated_at) VALUES('g','p2','u','2026-01-01','2026-01-01');
INSERT INTO recipes(id,group_id,created_by,created_at,updated_at) VALUES('r','g','u','2026-01-01','2026-01-01');
INSERT INTO recipe_ingredients(id,recipe_id,product_id,quantity,position) VALUES('ri','r','p',2,0);
INSERT INTO revisions(id,entity_type,entity_id,version,action,after_data,created_at) VALUES('v','product','p',7,'update','{"chinese_name":"原商品"}','2026-01-01');`)
	if err != nil {
		t.Fatal(err)
	}
	// Compare every pre-existing column, not only counts or names.
	queries := map[string]string{"products": "SELECT * FROM products ORDER BY id"}
	for _, table := range []string{"companies", "idea_product_links", "product_attributes", "attachments", "recipe_groups", "recipes", "recipe_ingredients", "revisions"} {
		queries[table] = "SELECT * FROM " + table + " ORDER BY id"
	}
	columns := map[string][]string{}
	dump := func(table string, before bool) string {
		t.Helper()
		query := queries[table]
		if !before && table == "products" {
			query = "SELECT "
			for index, column := range columns[table] {
				if index > 0 {
					query += ","
				}
				query += column
			}
			query += " FROM products ORDER BY id"
		}
		rows, err := db.Query(query)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		cols, err := rows.Columns()
		if err != nil {
			t.Fatal(err)
		}
		if before {
			columns[table] = cols
		}
		data := [][]any{}
		for rows.Next() {
			values, pointers := make([]any, len(cols)), make([]any, len(cols))
			for i := range values {
				pointers[i] = &values[i]
			}
			if err := rows.Scan(pointers...); err != nil {
				t.Fatal(err)
			}
			data = append(data, values)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(data)
		if err != nil {
			t.Fatal(err)
		}
		return string(encoded)
	}
	before := map[string]string{}
	for table := range queries {
		before[table] = dump(table, true)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if err = Migrate(context.Background(), db); err != nil {
			t.Fatal(err)
		}
		for table := range queries {
			if after := dump(table, false); after != before[table] {
				t.Fatalf("migration changed legacy %s: %s -> %s", table, before[table], after)
			}
		}
	}
	var groups, productGroups, otherGroups, populated int
	if err = db.QueryRow("SELECT COUNT(*) FROM product_variant_groups").Scan(&groups); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow("SELECT COUNT(DISTINCT variant_group_id) FROM products WHERE item_kind='product'").Scan(&productGroups); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow("SELECT COUNT(*) FROM products WHERE item_kind<>'product' AND variant_group_id IS NOT NULL").Scan(&otherGroups); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow("SELECT COUNT(*) FROM products WHERE variant_name<>'' OR variant_description<>''").Scan(&populated); err != nil {
		t.Fatal(err)
	}
	if groups != 3 || productGroups != 3 || otherGroups != 0 || populated != 0 {
		t.Fatalf("legacy groups were inferred or labels invented: %d %d %d %d", groups, productGroups, otherGroups, populated)
	}
	rows, err := db.Query("PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("migration broke existing references")
	}
}
