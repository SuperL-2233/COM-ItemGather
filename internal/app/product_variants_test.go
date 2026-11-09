package app

import (
	"net/url"
	"testing"
)

func readVariantProduct(t *testing.T, c *testClient, id string) Product {
	t.Helper()
	status, data := c.request(t, "GET", "/api/products/"+id, nil)
	assertStatus(t, status, 200, data)
	return decodeTestJSON[Product](t, data)
}

func milkWithFlavors(t *testing.T, c *testClient) Product {
	t.Helper()
	return catalogItem(t, c, "products", "白鲸 袋装奶", map[string]any{"description": "共用袋装奶模型", "variants": []variantInput{{Name: "玫瑰荔枝味", Status: "completed"}, {Name: "草莓味", Description: "粉色贴图", Status: "design"}, {Name: "巧克力味"}}})
}

func TestProductOwnsEqualFlavorChildren(t *testing.T) {
	env := newTestEnvironment(t, nil)
	c := newAdminClient(t, env)
	parent := milkWithFlavors(t, c)
	if parent.ParentProductID != "" || parent.VariantName != "" || parent.VariantDescription != "" || parent.VariantCount != 3 || len(parent.Variants) != 3 || parent.DevelopmentStatus != "concept" {
		t.Fatalf("product was promoted from its first flavor: %+v", parent)
	}
	ids := map[string]bool{parent.ID: true}
	for _, child := range parent.Variants {
		if ids[child.ID] || child.ParentProductID != parent.ID || child.ChineseName != parent.ChineseName || child.Description != parent.Description || child.VariantName == "" {
			t.Fatalf("flavors must be siblings under a distinct parent: %+v", child)
		}
		ids[child.ID] = true
	}
	rose, strawberry := parent.Variants[0], parent.Variants[1]
	if rose.DevelopmentStatus != "completed" || strawberry.DevelopmentStatus != "design" {
		t.Fatal("independent flavor statuses lost")
	}
	status, data := c.request(t, "PATCH", "/api/products/"+parent.ID+"/variants/"+strawberry.ID, map[string]any{"version": strawberry.Version, "variant_name": "草莓牛奶味", "variant_description": "新版粉色贴图", "development_status": "production"})
	assertStatus(t, status, 200, data)
	edited := decodeTestJSON[Product](t, data)
	if edited.ID != strawberry.ID || edited.ParentProductID != parent.ID || edited.VariantName != "草莓牛奶味" {
		t.Fatal("editing a flavor changed its identity")
	}
	current := readVariantProduct(t, c, parent.ID)
	if current.ID != parent.ID || current.ChineseName != parent.ChineseName || current.VariantName != "" || current.VariantCount != 3 || current.DevelopmentStatus != parent.DevelopmentStatus || current.Version != parent.Version+1 {
		t.Fatalf("editing a child replaced the parent: %+v", current)
	}
	status, data = c.request(t, "PATCH", "/api/products/"+parent.ID+"/variants/"+edited.ID, map[string]any{"version": edited.Version, "chinese_name": "口味不能改写父商品"})
	assertStatus(t, status, 422, data)
	if readVariantProduct(t, c, rose.ID).DevelopmentStatus != rose.DevelopmentStatus {
		t.Fatal("editing one sibling changed another")
	}
	status, data = c.request(t, "PATCH", "/api/products/"+parent.ID, map[string]any{"version": current.Version, "chinese_name": "白鲸 新袋装奶", "category": "乳制品", "description": "共用新模型"})
	assertStatus(t, status, 200, data)
	for _, child := range current.Variants {
		item := readVariantProduct(t, c, child.ID)
		if item.ChineseName != "白鲸 新袋装奶" || item.Category != "乳制品" || item.Description != "共用新模型" || item.ParentProductID != parent.ID || item.VariantName != child.VariantName {
			t.Fatal("parent metadata did not reach its children")
		}
	}
	status, data = c.request(t, "PATCH", "/api/products/"+parent.ID, map[string]any{"version": current.Version + 1, "variant_name": "不应写在商品上"})
	assertStatus(t, status, 422, data)
}

func TestProductFlavorAddRemoveAndOwnership(t *testing.T) {
	env := newTestEnvironment(t, nil)
	c := newAdminClient(t, env)
	parent := catalogItem(t, c, "products", "白鲸 袋装奶", map[string]any{"development_status": "completed"})
	if parent.VariantCount != 0 || len(parent.Variants) != 0 {
		t.Fatal("plain product acquired a hidden default flavor")
	}
	status, data := c.request(t, "POST", "/api/products/"+parent.ID+"/variants", map[string]any{"version": parent.Version, "variant_name": "草莓味"})
	assertStatus(t, status, 201, data)
	child := decodeTestJSON[Product](t, data)
	if child.ParentProductID != parent.ID || child.ID == parent.ID || child.DevelopmentStatus != "concept" {
		t.Fatal("adding a flavor replaced the product")
	}
	parent = readVariantProduct(t, c, parent.ID)
	if parent.VariantName != "" || parent.VariantCount != 1 || parent.DevelopmentStatus != "completed" {
		t.Fatal("parent became its only flavor")
	}
	other := catalogItem(t, c, "products", "其他商品", nil)
	status, data = c.request(t, "PATCH", "/api/products/"+other.ID+"/variants/"+child.ID, map[string]any{"version": child.Version, "variant_name": "错误归属"})
	assertStatus(t, status, 404, data)
	status, data = c.request(t, "PATCH", "/api/products/"+parent.ID+"/variants/"+child.ID, map[string]any{"version": child.Version, "variant_description": "粉色", "attributes": []Attribute{{Key: "含糖量", Value: "少糖", Certainty: "asserted"}}})
	assertStatus(t, status, 200, data)
	child = decodeTestJSON[Product](t, data)
	status, data = c.request(t, "PATCH", "/api/products/"+parent.ID+"/variants/"+child.ID, map[string]any{"version": child.Version - 1, "variant_name": "过期修改"})
	assertStatus(t, status, 409, data)
	status, data = c.request(t, "DELETE", "/api/products/"+parent.ID+"/variants/"+child.ID, map[string]int{"version": child.Version})
	assertStatus(t, status, 200, data)
	parent = readVariantProduct(t, c, parent.ID)
	if parent.VariantCount != 0 || len(parent.Variants) != 0 || parent.VariantName != "" || parent.ChineseName != "白鲸 袋装奶" {
		t.Fatal("removing the final flavor removed the product")
	}
	status, data = c.request(t, "POST", "/api/products/"+child.ID+"/restore", map[string]int{"version": child.Version + 1})
	assertStatus(t, status, 200, data)
	restored := readVariantProduct(t, c, child.ID)
	if restored.ParentProductID != parent.ID || restored.VariantDescription != "粉色" || len(restored.Attributes) != 1 {
		t.Fatal("flavor restore lost fields or parent identity")
	}
}

func TestProductFlavorCatalogSearchAndConcreteRecipeIdentity(t *testing.T) {
	env := newTestEnvironment(t, nil)
	c := newAdminClient(t, env)
	parent := milkWithFlavors(t, c)
	sameName := catalogItem(t, c, "products", parent.ChineseName, nil)
	type productPage struct {
		Items []Product `json:"items"`
		Total int       `json:"total"`
	}
	for _, entry := range []struct {
		query string
		total int
	}{{"", 2}, {"grouped=1", 2}, {"include_variants=1", 5}, {"q=" + url.QueryEscape("草莓"), 1}, {"q=" + url.QueryEscape("粉色贴图"), 1}, {"limit=1&offset=1", 2}} {
		status, data := c.request(t, "GET", "/api/products?"+entry.query, nil)
		assertStatus(t, status, 200, data)
		page := decodeTestJSON[productPage](t, data)
		if page.Total != entry.total || len(page.Items) == 0 {
			t.Fatalf("parent catalog failed for %s: %s", entry.query, data)
		}
		if entry.query != "include_variants=1" {
			for _, item := range page.Items {
				if item.ParentProductID != "" || item.VariantName != "" {
					t.Fatal("flavors leaked into the product catalog")
				}
			}
		}
	}
	if sameName.ID == parent.ID {
		t.Fatal("independent same-named products were combined")
	}
	strawberry := parent.Variants[1]
	group := catalogGroup(t, c, strawberry)
	status, data := c.request(t, "POST", "/api/recipe-groups/"+group.ID+"/recipes", recipeFields{Name: "精确口味配方", OutputQuantity: 1, Verification: "untested", Ingredients: []Ingredient{{ProductID: parent.Variants[0].ID, Quantity: 2}}})
	assertStatus(t, status, 201, data)
	status, data = c.request(t, "GET", "/api/recipe-groups/"+group.ID, nil)
	assertStatus(t, status, 200, data)
	recipe := decodeTestJSON[RecipeGroup](t, data)
	if recipe.OutputID != strawberry.ID || recipe.Output.ParentProductID != parent.ID || recipe.Recipes[0].Ingredients[0].ProductID != parent.Variants[0].ID {
		t.Fatal("recipe stopped referring to an exact original flavor")
	}
	status, data = c.request(t, "GET", "/api/search?kind=products&q="+url.QueryEscape("草莓"), nil)
	assertStatus(t, status, 200, data)
	search := decodeTestJSON[struct {
		Products []Product `json:"products"`
	}](t, data)
	if len(search.Products) != 1 || search.Products[0].ID != parent.ID {
		t.Fatal("global search did not return the parent product")
	}
	graph := readTestMindMap(t, c)
	if len(graph.Items) != 2 {
		t.Fatal("mind map displayed flavors as products")
	}
	for _, item := range graph.Items {
		if item.ID == parent.ID && (item.VariantCount != 3 || len(item.VariantNames) != 3) {
			t.Fatal("mind map lost child names")
		}
	}
	related, err := env.app.mcpListRecords(t.Context(), mcpListArgs{RelatedTo: "product:" + parent.ID, Kinds: []string{"product"}, Limit: 10})
	if err != nil || len(related.Results) != 3 {
		t.Fatalf("MCP parent-child relation missing: %+v %v", related, err)
	}
	status, data = c.request(t, "GET", "/api/export", nil)
	assertStatus(t, status, 200, data)
	exported := decodeTestJSON[struct {
		Products []Product `json:"products"`
	}](t, data)
	if len(exported.Products) != 5 {
		t.Fatal("export collapsed original concrete identities")
	}
}

func TestProductFlavorTrashCascadesOnlyWithItsParent(t *testing.T) {
	env := newTestEnvironment(t, nil)
	c := newAdminClient(t, env)
	parent := milkWithFlavors(t, c)
	removed := parent.Variants[1]
	status, data := c.request(t, "DELETE", "/api/products/"+parent.ID+"/variants/"+removed.ID, map[string]int{"version": removed.Version})
	assertStatus(t, status, 200, data)
	parent = readVariantProduct(t, c, parent.ID)
	status, data = c.request(t, "DELETE", "/api/products/"+parent.ID, map[string]int{"version": parent.Version})
	assertStatus(t, status, 200, data)
	// A repeated delete must not replace the timestamp used to restore children.
	status, data = c.request(t, "DELETE", "/api/products/"+parent.ID, map[string]int{"version": parent.Version + 1})
	assertStatus(t, status, 200, data)
	for _, child := range append(parent.Variants, removed) {
		status, data = c.request(t, "GET", "/api/products/"+child.ID, nil)
		assertStatus(t, status, 404, data)
	}
	status, data = c.request(t, "GET", "/api/trash", nil)
	assertStatus(t, status, 200, data)
	trash := decodeTestJSON[struct {
		Products []Product `json:"products"`
	}](t, data)
	if len(trash.Products) != 2 {
		t.Fatalf("cascaded flavors duplicated the parent in trash: %s", data)
	}
	status, data = c.request(t, "POST", "/api/products/"+parent.ID+"/restore", map[string]int{"version": parent.Version + 1})
	assertStatus(t, status, 200, data)
	restored := readVariantProduct(t, c, parent.ID)
	if restored.VariantCount != 2 {
		t.Fatal("parent restore revived an individually removed flavor")
	}
	status, data = c.request(t, "GET", "/api/products/"+removed.ID, nil)
	assertStatus(t, status, 404, data)
	catalogGroup(t, c, restored.Variants[0])
	status, data = c.request(t, "DELETE", "/api/products/"+restored.ID, map[string]int{"version": restored.Version})
	assertStatus(t, status, 200, data)
	status, data = c.request(t, "DELETE", "/api/products/"+restored.ID+"/permanent", map[string]int{"version": restored.Version + 1})
	assertStatus(t, status, 409, data)
	if _, err := loadProduct(t.Context(), env.db, restored.Variants[0].ID, true, false); err != nil {
		t.Fatal("failed parent purge deleted a referenced flavor")
	}
}

func TestProductFlavorBatchAndLegacyRequests(t *testing.T) {
	env := newTestEnvironment(t, nil)
	c := newAdminClient(t, env)
	parent := milkWithFlavors(t, c)
	status, data := c.request(t, "PATCH", "/api/products/batch", map[string]any{"items": batchSelection(parent), "category": "乳制品", "development_status": "completed"})
	assertStatus(t, status, 200, data)
	for _, child := range parent.Variants {
		item := readVariantProduct(t, c, child.ID)
		if item.Category != "乳制品" || item.Version != child.Version+1 || item.DevelopmentStatus != child.DevelopmentStatus {
			t.Fatal("batch parent edit overwrote flavor status or incremented twice")
		}
	}
	// A legacy single-variant request becomes a normal parent and one child.
	legacy := catalogItem(t, c, "products", "旧客户端商品", map[string]any{"variant_name": "香草味", "variant_description": "白色贴图"})
	if legacy.VariantName != "" || legacy.VariantCount != 1 || legacy.Variants[0].ID == legacy.ID {
		t.Fatal("legacy request labeled the parent as a flavor")
	}
	child := legacy.Variants[0]
	snapshot := `{"chinese_name":"旧历史名称","company":"","category":"","description":"旧版说明","development_status":"concept","attributes":[]}`
	if _, err := env.db.Exec("INSERT INTO revisions(id,entity_type,entity_id,version,action,after_data,created_at) VALUES('legacy-flavor','product',?,1,'update',?,?)", child.ID, snapshot, now()); err != nil {
		t.Fatal(err)
	}
	status, data = c.request(t, "POST", "/api/products/"+child.ID+"/revisions/legacy-flavor/restore", map[string]int{"version": child.Version})
	assertStatus(t, status, 200, data)
	child = readVariantProduct(t, c, child.ID)
	if child.VariantName != "香草味" || child.VariantDescription != "白色贴图" || child.ParentProductID != legacy.ID {
		t.Fatal("legacy history lost original child details")
	}
	root := readVariantProduct(t, c, legacy.ID)
	if root.VariantName != "" || root.ChineseName != "旧客户端商品" {
		t.Fatal("legacy restore turned the parent into a selected flavor")
	}
}

func TestProductFlavorValidationRollsBackParentAndChildren(t *testing.T) {
	env := newTestEnvironment(t, nil)
	c := newAdminClient(t, env)
	for _, collection := range []string{"products", "tools", "other-items"} {
		status, data := c.request(t, "POST", "/api/"+collection, map[string]any{"chinese_name": "不应保存", "variants": []variantInput{{Name: "Original"}, {Name: " original "}}})
		assertStatus(t, status, 422, data)
	}
	var count int
	if err := env.db.QueryRow("SELECT COUNT(*) FROM products").Scan(&count); err != nil || count != 0 {
		t.Fatal("invalid create left a parent or child behind")
	}
	parent := milkWithFlavors(t, c)
	status, data := c.request(t, "POST", "/api/products/"+parent.ID+"/variants", map[string]any{"version": parent.Version, "variant_name": " 草莓味 "})
	assertStatus(t, status, 422, data)
	status, data = c.request(t, "POST", "/api/products/"+parent.Variants[0].ID+"/variants", map[string]any{"version": parent.Variants[0].Version, "variant_name": "孙级口味"})
	assertStatus(t, status, 422, data)
	if readVariantProduct(t, c, parent.ID).VariantCount != 3 {
		t.Fatal("invalid nested flavor was saved")
	}
}
