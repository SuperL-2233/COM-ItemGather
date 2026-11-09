package app

import (
	"fmt"
	"net/url"
	"testing"
)

func readVariantProduct(t *testing.T, c *testClient, id string) Product {
	t.Helper()
	status, data := c.request(t, "GET", "/api/products/"+id, nil)
	assertStatus(t, status, 200, data)
	return decodeTestJSON[Product](t, data)
}

func TestProductVariantsShareMetadataAndKeepIndependentIdentities(t *testing.T) {
	env := newTestEnvironment(t, nil)
	c := newAdminClient(t, env)
	company := catalogCompany(t, c, "牛奶公司")
	base := catalogItem(t, c, "products", "咕噜牛奶", map[string]any{"company_id": company.ID, "category": "饮料", "description": "共用瓶子模型", "variants": []variantInput{{Name: "原味", Status: "completed"}, {Name: "草莓味", Description: "粉色贴图", Status: "design"}, {Name: "巧克力味"}}})
	if base.VariantCount != 3 || len(base.Variants) != 3 || base.VariantName != "原味" {
		t.Fatalf("unexpected variants or extra default: %+v", base)
	}
	var strawberry Product
	ids := map[string]bool{}
	for _, member := range base.Variants {
		if ids[member.ID] || member.VariantGroupID != base.VariantGroupID || member.ChineseName != base.ChineseName || member.CompanyID != company.ID || member.Description != base.Description {
			t.Fatalf("identity or shared metadata lost: %+v", member)
		}
		ids[member.ID] = true
		if member.VariantName == "草莓味" {
			strawberry = member
		}
	}
	if strawberry.ID == "" || strawberry.VariantDescription != "粉色贴图" || strawberry.DevelopmentStatus != "design" {
		t.Fatal("per-variant fields missing")
	}
	// Existing item-based recipe relations must keep pointing to the exact flavor.
	group := catalogGroup(t, c, base)
	status, data := c.request(t, "POST", "/api/recipe-groups/"+group.ID+"/recipes", recipeFields{Name: "草莓配方", OutputQuantity: 1, Verification: "untested", Ingredients: []Ingredient{{ProductID: strawberry.ID, Quantity: 2}}})
	assertStatus(t, status, 201, data)
	status, data = c.request(t, "PATCH", "/api/products/"+strawberry.ID, map[string]any{"version": strawberry.Version, "chinese_name": "新牛奶", "category": "乳饮料", "description": "新的共用模型说明", "variant_description": "新的粉色贴图", "development_status": "production"})
	assertStatus(t, status, 200, data)
	updated := decodeTestJSON[Product](t, data)
	for _, original := range base.Variants {
		member := readVariantProduct(t, c, original.ID)
		if member.ChineseName != "新牛奶" || member.Category != "乳饮料" || member.Description != "新的共用模型说明" || member.Version != original.Version+1 {
			t.Fatalf("shared edit missing version/history: %+v", member)
		}
		if original.ID != strawberry.ID && (member.DevelopmentStatus != original.DevelopmentStatus || member.VariantDescription != original.VariantDescription) {
			t.Fatal("shared edit overwrote per-variant details")
		}
	}
	status, data = c.request(t, "PATCH", "/api/products/"+base.ID, map[string]any{"version": base.Version, "company_id": ""})
	assertStatus(t, status, 409, data)
	status, data = c.request(t, "GET", "/api/recipe-groups/"+group.ID, nil)
	assertStatus(t, status, 200, data)
	recipeGroup := decodeTestJSON[RecipeGroup](t, data)
	if recipeGroup.OutputID != base.ID || recipeGroup.Recipes[0].Ingredients[0].ProductID != strawberry.ID || recipeGroup.Recipes[0].Ingredients[0].Item.VariantName != "草莓味" {
		t.Fatal("shared edit retargeted a recipe")
	}
	status, data = c.request(t, "POST", "/api/products/"+updated.ID+"/variants", map[string]any{"version": updated.Version, "variant_name": "香蕉味"})
	assertStatus(t, status, 201, data)
	added := decodeTestJSON[Product](t, data)
	if added.VariantCount != 4 || added.DevelopmentStatus != "concept" || added.ChineseName != "新牛奶" {
		t.Fatalf("new variant copied wrong defaults: %+v", added)
	}
	status, data = c.request(t, "POST", "/api/products/"+updated.ID+"/variants", map[string]any{"version": updated.Version, "variant_name": " 草莓味 "})
	assertStatus(t, status, 422, data)
	if readVariantProduct(t, c, updated.ID).VariantCount != 4 {
		t.Fatal("duplicate created a partial variant")
	}
}

func TestProductVariantsGroupedPaginationSearchAndExport(t *testing.T) {
	env := newTestEnvironment(t, nil)
	c := newAdminClient(t, env)
	first := catalogItem(t, c, "products", "同名饮料", map[string]any{"variants": []variantInput{{Name: "Original", Status: "completed"}, {Name: "苹果味", Description: "绿色限定贴图", Status: "design"}, {Name: "葡萄味"}}})
	second := catalogItem(t, c, "products", "同名饮料", nil)
	if first.VariantGroupID == second.VariantGroupID || second.VariantName != "" {
		t.Fatal("same-named independent items were merged")
	}
	type productPage struct {
		Items []Product `json:"items"`
		Total int       `json:"total"`
	}
	for _, entry := range []struct {
		query string
		total int
	}{{"grouped=1&limit=1", 2}, {"grouped=1&q=" + url.QueryEscape("苹果"), 1}, {"grouped=1&q=" + url.QueryEscape("限定贴图"), 1}, {"grouped=1&status=design", 1}, {"q=" + url.QueryEscape("同名饮料"), 4}} {
		status, data := c.request(t, "GET", "/api/products?"+entry.query, nil)
		assertStatus(t, status, 200, data)
		page := decodeTestJSON[productPage](t, data)
		if page.Total != entry.total || len(page.Items) == 0 {
			t.Fatalf("grouping/search lost records for %s: %s", entry.query, data)
		}
		if page.Items[0].VariantCount > 1 && entry.query != "q="+url.QueryEscape("同名饮料") && len(page.Items[0].Variants) != 3 {
			t.Fatal("group omitted non-matching flavors")
		}
	}
	status, data := c.request(t, "GET", "/api/products?grouped=1&limit=1&offset=1", nil)
	assertStatus(t, status, 200, data)
	if page := decodeTestJSON[productPage](t, data); len(page.Items) != 1 || page.Items[0].ID != first.ID {
		t.Fatalf("pagination split a product group: %s", data)
	}
	status, data = c.request(t, "GET", "/api/search?kind=products&q="+url.QueryEscape("苹果"), nil)
	assertStatus(t, status, 200, data)
	if len(decodeTestJSON[struct {
		Products []Product `json:"products"`
	}](t, data).Products) != 1 {
		t.Fatal("global search ignores variants")
	}
	page, err := env.app.mcpListRecords(t.Context(), mcpListArgs{Query: "苹果", Kinds: []string{"product"}, Limit: 10})
	if err != nil || len(page.Results) != 1 || page.Results[0].Title != "同名饮料 · 苹果味" {
		t.Fatalf("MCP variant search failed: %+v %v", page, err)
	}
	status, data = c.request(t, "GET", "/api/export", nil)
	assertStatus(t, status, 200, data)
	exported := decodeTestJSON[struct {
		Products []Product `json:"products"`
	}](t, data)
	if len(exported.Products) != 4 {
		t.Fatalf("export collapsed concrete variants: %s", data)
	}
}

func TestProductVariantGroupingDeletionBatchAndHistory(t *testing.T) {
	env := newTestEnvironment(t, nil)
	c := newAdminClient(t, env)
	base := catalogItem(t, c, "products", "旧牛奶", map[string]any{"description": "共用瓶子", "variant_name": "原味"})
	legacy := catalogItem(t, c, "products", "旧牛奶草莓版", map[string]any{"description": "原始独立设定", "variant_description": "粉色", "development_status": "design"})
	idea := createTestIdea(t, c, "variant-legacy-link", "草莓包装")
	status, data := c.request(t, "POST", "/api/links", map[string]any{"idea_id": idea.ID, "product_id": legacy.ID, "status": "candidate"})
	assertStatus(t, status, 201, data)
	group := catalogGroup(t, c, legacy)
	status, data = c.request(t, "POST", "/api/products/"+legacy.ID+"/variant-group", map[string]any{"version": legacy.Version, "target_id": base.ID, "target_version": base.Version, "variant_name": "草莓味"})
	assertStatus(t, status, 200, data)
	moved := decodeTestJSON[Product](t, data)
	if moved.ID != legacy.ID || moved.VariantGroupID != base.VariantGroupID || moved.Description != base.Description || moved.VariantDescription != "粉色" || moved.DevelopmentStatus != "design" || len(moved.Links) != 1 {
		t.Fatalf("grouping destroyed independent data: %+v", moved)
	}
	status, data = c.request(t, "DELETE", "/api/products/"+moved.ID, map[string]int{"version": moved.Version})
	assertStatus(t, status, 200, data)
	// A shared edit also updates trashed members, so restoring cannot revive stale metadata.
	base = readVariantProduct(t, c, base.ID)
	status, data = c.request(t, "PATCH", "/api/products/batch", map[string]any{"items": batchSelection(base), "category": "乳制品", "development_status": "completed"})
	assertStatus(t, status, 200, data)
	trashed, err := loadProduct(t.Context(), env.db, moved.ID, true, false)
	if err != nil || trashed.Category != "乳制品" || trashed.DevelopmentStatus != "design" || trashed.Version != moved.Version+2 {
		t.Fatalf("trash metadata or independent state lost: %+v %v", trashed, err)
	}
	status, data = c.request(t, "POST", "/api/products/"+moved.ID+"/restore", map[string]int{"version": trashed.Version})
	assertStatus(t, status, 200, data)
	moved = readVariantProduct(t, c, moved.ID)
	status, data = c.request(t, "POST", "/api/products/"+moved.ID+"/variant-group", map[string]any{"version": moved.Version, "target_id": "", "variant_name": moved.VariantName})
	assertStatus(t, status, 200, data)
	detached := decodeTestJSON[Product](t, data)
	if detached.VariantGroupID == base.VariantGroupID || detached.Category != "乳制品" || detached.ID != legacy.ID {
		t.Fatal("detach changed identity or data")
	}
	status, data = c.request(t, "GET", "/api/recipe-groups/"+group.ID, nil)
	assertStatus(t, status, 200, data)
	if decodeTestJSON[RecipeGroup](t, data).OutputID != legacy.ID {
		t.Fatal("grouping changed recipe output")
	}
	status, data = c.request(t, "GET", "/api/products/"+base.ID+"/revisions", nil)
	assertStatus(t, status, 200, data)
	revisions := decodeTestJSON[struct {
		Items []Revision `json:"items"`
	}](t, data).Items
	base = readVariantProduct(t, c, base.ID)
	status, data = c.request(t, "POST", fmt.Sprintf("/api/products/%s/revisions/%s/restore", base.ID, revisions[len(revisions)-1].ID), map[string]int{"version": base.Version})
	assertStatus(t, status, 200, data)
	if readVariantProduct(t, c, detached.ID).Category != "乳制品" {
		t.Fatal("history restore changed a detached member")
	}
}

func TestProductVariantsInvalidCreatesRollbackAndRestoreConflicts(t *testing.T) {
	env := newTestEnvironment(t, nil)
	c := newAdminClient(t, env)
	for _, collection := range []string{"products", "tools", "other-items"} {
		status, data := c.request(t, "POST", "/api/"+collection, map[string]any{"chinese_name": "不应保存", "variants": []variantInput{{Name: "Original"}, {Name: " original "}}})
		assertStatus(t, status, 422, data)
	}
	var count int
	if err := env.db.QueryRow("SELECT COUNT(*) FROM products").Scan(&count); err != nil || count != 0 {
		t.Fatal("invalid multi-create left partial records")
	}
	base := catalogItem(t, c, "products", "饮料", map[string]any{"variants": []variantInput{{Name: "原味"}, {Name: "草莓味"}}})
	member := base.Variants[1]
	status, data := c.request(t, "DELETE", "/api/products/"+member.ID, map[string]int{"version": member.Version})
	assertStatus(t, status, 200, data)
	status, data = c.request(t, "POST", "/api/products/"+base.ID+"/variants", map[string]any{"version": base.Version, "variant_name": member.VariantName})
	assertStatus(t, status, 201, data)
	status, data = c.request(t, "POST", "/api/products/"+member.ID+"/restore", map[string]int{"version": member.Version + 1})
	assertStatus(t, status, 422, data)
	status, data = c.request(t, "GET", "/api/products/"+member.ID, nil)
	assertStatus(t, status, 404, data)
}

func TestProductVariantSharedBatchAndLegacyHistory(t *testing.T) {
	env := newTestEnvironment(t, nil)
	c := newAdminClient(t, env)
	base := catalogItem(t, c, "products", "原商品", map[string]any{"variants": []variantInput{{Name: "原味"}, {Name: "草莓味", Status: "design"}, {Name: "香蕉味", Status: "completed"}}})
	first, second, third := base.Variants[0], base.Variants[1], base.Variants[2]
	status, data := c.request(t, "PATCH", "/api/products/batch", map[string]any{"items": batchSelection(first, second), "category": "批量共用分类", "development_status": "production"})
	assertStatus(t, status, 200, data)
	for _, original := range base.Variants {
		current := readVariantProduct(t, c, original.ID)
		if current.Version != original.Version+1 || current.Category != "批量共用分类" {
			t.Fatalf("batch incremented a sibling twice or skipped it: %+v", current)
		}
		if original.ID == third.ID && current.DevelopmentStatus != original.DevelopmentStatus {
			t.Fatal("batch changed an unselected variant's status")
		}
	}
	// A patch from an older client omits the new fields and must preserve them.
	base = readVariantProduct(t, c, base.ID)
	status, data = c.request(t, "PATCH", "/api/products/"+base.ID, map[string]any{"version": base.Version, "description": "新版共用说明"})
	assertStatus(t, status, 200, data)
	base = decodeTestJSON[Product](t, data)
	if base.VariantName != "原味" {
		t.Fatal("old client patch cleared a variant label")
	}
	legacy := `{"chinese_name":"旧历史名称","company":"","category":"旧分类","description":"旧版说明","development_status":"concept","attributes":[]}`
	_, err := env.db.Exec("INSERT INTO revisions(id,entity_type,entity_id,version,action,after_data,created_at) VALUES('pre-variants','product',?,1,'update',?,?)", base.ID, legacy, now())
	if err != nil {
		t.Fatal(err)
	}
	status, data = c.request(t, "POST", "/api/products/"+base.ID+"/revisions/pre-variants/restore", map[string]int{"version": base.Version})
	assertStatus(t, status, 200, data)
	for _, original := range base.Variants {
		current := readVariantProduct(t, c, original.ID)
		if current.VariantName != original.VariantName || current.VariantGroupID != base.VariantGroupID || current.ChineseName != "旧历史名称" || current.Category != "旧分类" {
			t.Fatalf("legacy history broke variant identity or shared metadata: %+v", current)
		}
		if current.ID != base.ID && current.DevelopmentStatus != original.DevelopmentStatus {
			t.Fatal("history restore changed another variant's status")
		}
	}
	independent := catalogItem(t, c, "products", "独立商品", nil)
	status, data = c.request(t, "POST", "/api/products/"+independent.ID+"/variant-group", map[string]any{"version": independent.Version, "target_id": base.ID, "target_version": base.Version, "variant_name": "新口味"})
	assertStatus(t, status, 409, data)
	if current := readVariantProduct(t, c, independent.ID); current.VariantGroupID != independent.VariantGroupID || current.ChineseName != independent.ChineseName {
		t.Fatal("stale target left a partial regroup")
	}
}

func TestProductVariantGroupSurvivesOriginalItemPurge(t *testing.T) {
	env := newTestEnvironment(t, nil)
	c := newAdminClient(t, env)
	base := catalogItem(t, c, "products", "持久分组", map[string]any{"variants": []variantInput{{Name: "原味"}, {Name: "草莓味"}}})
	member := base.Variants[1]
	status, data := c.request(t, "DELETE", "/api/products/"+base.ID, map[string]int{"version": base.Version})
	assertStatus(t, status, 200, data)
	status, data = c.request(t, "DELETE", "/api/products/"+base.ID+"/permanent", map[string]int{"version": base.Version + 1})
	assertStatus(t, status, 200, data)
	member = readVariantProduct(t, c, member.ID)
	if member.VariantCount != 1 || member.VariantGroupID != base.VariantGroupID {
		t.Fatal("purging the first member destroyed the group")
	}
	status, data = c.request(t, "POST", "/api/products/"+member.ID+"/variants", map[string]any{"version": member.Version, "variant_name": "香蕉味"})
	assertStatus(t, status, 201, data)
	if added := decodeTestJSON[Product](t, data); added.VariantGroupID != base.VariantGroupID || added.VariantCount != 2 {
		t.Fatal("surviving group can no longer accept variants")
	}
}
