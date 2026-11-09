package database

import (
	"context"
	"encoding/json"
	"testing"
)

func TestProductFlavorChildrenMigrationPreservesEveryConcreteItem(t *testing.T) {
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
		if entry.Name() >= "023_" {
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
INSERT INTO companies(id,chinese_name,created_at,updated_at) VALUES('c','白鲸','2026-01-01','2026-01-01');
INSERT INTO product_variant_groups(id) VALUES('milk'),('plain'),('legacy'),('same-name'),('archived'),('mixed');
INSERT INTO products(id,chinese_name,company_id,company,category,description,created_by,created_at,updated_at,version,development_status,status_option_id,variant_group_id,variant_name,variant_description,deleted_at) VALUES
('rose','白鲸 袋装奶','c','白鲸','饮料','共用模型','u','2026-01-01','2026-02-01',7,'completed','completed','milk','玫瑰荔枝味','粉色包装',NULL),
('strawberry','白鲸 袋装奶','c','白鲸','饮料','共用模型','u','2026-01-02','2026-02-02',4,'design','design','milk','草莓味','草莓贴图',NULL),
('chocolate','白鲸 袋装奶','c','白鲸','饮料','共用模型','u','2026-01-03','2026-02-03',3,'production','production','milk','巧克力味','棕色贴图',NULL),
('removed','白鲸 袋装奶','c','白鲸','饮料','共用模型','u','2026-01-04','2026-02-04',9,'concept','concept','milk','已删除口味','保留差异','2026-02-04'),
('plain','原独立商品',NULL,'','','原说明','u','2026-01-01','2026-01-01',2,'completed','completed','plain','','',NULL),
('mango','原独立商品',NULL,'','','原说明','u','2026-01-02','2026-01-02',5,'design','design','plain','芒果味','黄色',NULL),
('legacy','同名商品',NULL,'','','未使用变体的旧数据','u','2026-01-01','2026-01-01',8,'concept','concept','legacy','','',NULL),
('same-name','同名商品',NULL,'','','另一独立商品','u','2026-01-01','2026-01-01',1,'concept','concept','same-name','','',NULL),
('deleted-blank','不能复活的旧记录',NULL,'','','旧普通记录','u','2026-01-01','2026-01-02',3,'concept','concept','mixed','','','2026-01-02'),
('active-cherry','不能复活的旧记录',NULL,'','','旧普通记录','u','2026-01-03','2026-01-03',5,'design','design','mixed','樱桃味','红色贴图',NULL),
('old-flavor','回收站商品',NULL,'','','旧设定','u','2026-01-01','2026-01-02',6,'concept','concept','archived','旧口味','旧贴图','2026-01-02');
INSERT INTO ideas(id,content,author_id,created_at,updated_at) VALUES('i','原灵感','u','2026-01-01','2026-01-01');
INSERT INTO idea_product_links(id,idea_id,product_id,created_by,created_at,updated_at) VALUES('l','i','rose','u','2026-01-01','2026-01-01');
INSERT INTO product_attributes(id,product_id,key,value) VALUES('pa','strawberry','原属性','原值');
INSERT INTO attachments(id,product_id,uploader_id,original_name,mime_type,size,file_path,created_at) VALUES('a','strawberry','u','原图.png','image/png',100,'original.png','2026-01-01');
INSERT INTO recipe_groups(id,output_id,created_by,created_at,updated_at) VALUES('g','chocolate','u','2026-01-01','2026-01-01');
INSERT INTO recipes(id,group_id,created_by,created_at,updated_at) VALUES('r','g','u','2026-01-01','2026-01-01');
INSERT INTO recipe_ingredients(id,recipe_id,product_id,quantity,position) VALUES('ri','r','rose',2,0);
INSERT INTO revisions(id,entity_type,entity_id,version,action,after_data,created_at) VALUES('v','product','rose',7,'update','{"variant_name":"玫瑰荔枝味"}','2026-01-01');`)
	if err != nil {
		t.Fatal(err)
	}
	columns := map[string][]string{}
	tables := []string{"products", "idea_product_links", "product_attributes", "attachments", "recipe_groups", "recipes", "recipe_ingredients", "revisions"}
	dump := func(table string, initial bool) string {
		t.Helper()
		query := "SELECT * FROM " + table
		if !initial && table == "products" {
			query = "SELECT "
			for i, column := range columns[table] {
				if i > 0 {
					query += ","
				}
				query += column
			}
			query += " FROM products WHERE id IN ('rose','strawberry','chocolate','removed','plain','mango','legacy','same-name','old-flavor','deleted-blank','active-cherry')"
		}
		query += " ORDER BY id"
		rows, err := db.Query(query)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		cols, err := rows.Columns()
		if err != nil {
			t.Fatal(err)
		}
		if initial {
			columns[table] = cols
		}
		values := [][]any{}
		for rows.Next() {
			row, pointers := make([]any, len(cols)), make([]any, len(cols))
			for i := range row {
				pointers[i] = &row[i]
			}
			if err := rows.Scan(pointers...); err != nil {
				t.Fatal(err)
			}
			values = append(values, row)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(values)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	before := map[string]string{}
	for _, table := range tables {
		before[table] = dump(table, true)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if err = Migrate(context.Background(), db); err != nil {
			t.Fatal(err)
		}
		for _, table := range tables {
			if after := dump(table, false); after != before[table] {
				t.Fatalf("old data or reference changed in %s: %s -> %s", table, before[table], after)
			}
		}
	}
	var parentID string
	if err = db.QueryRow("SELECT parent_product_id FROM products WHERE id='rose'").Scan(&parentID); err != nil || parentID == "rose" || parentID == "strawberry" || parentID == "chocolate" {
		t.Fatalf("a flavor was promoted to parent: %s %v", parentID, err)
	}
	var count int
	if err = db.QueryRow("SELECT COUNT(*) FROM products WHERE parent_product_id=?", parentID).Scan(&count); err != nil || count != 4 {
		t.Fatalf("not all original flavors became siblings: %d %v", count, err)
	}
	var name, flavor, status string
	if err = db.QueryRow("SELECT chinese_name,variant_name,development_status FROM products WHERE id=? AND parent_product_id IS NULL", parentID).Scan(&name, &flavor, &status); err != nil || name != "白鲸 袋装奶" || flavor != "" || status != "concept" {
		t.Fatalf("new parent is not independent: %s %s %s %v", name, flavor, status, err)
	}
	if err = db.QueryRow("SELECT parent_product_id FROM products WHERE id='mango'").Scan(&name); err != nil || name != "plain" {
		t.Fatal("original unnamed product ID was not retained as parent")
	}
	if err = db.QueryRow("SELECT COUNT(*) FROM products WHERE id IN ('legacy','same-name','plain') AND parent_product_id IS NULL").Scan(&count); err != nil || count != 3 {
		t.Fatal("plain or same-named old products were grouped or lost")
	}
	if err = db.QueryRow("SELECT COUNT(*) FROM products p JOIN products child ON child.parent_product_id=p.id WHERE child.id='old-flavor' AND p.deleted_at IS NOT NULL").Scan(&count); err != nil || count != 1 {
		t.Fatal("migration revived an archived product")
	}
	if err = db.QueryRow("SELECT p.id FROM products p JOIN products child ON child.parent_product_id=p.id WHERE child.id='active-cherry' AND p.deleted_at IS NULL").Scan(&name); err != nil || name == "deleted-blank" {
		t.Fatal("an archived blank record hid its still-active flavor")
	}
	rows, err := db.Query("PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("migration broke an old reference")
	}
}
