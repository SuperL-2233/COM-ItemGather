package app

import (
	"context"
	"net/url"
	"slices"
	"testing"
)

func TestOtherItemsLifecycleNamesAttributesAndCompanyRestriction(t *testing.T) {
	env := newTestEnvironment(t, nil)
	c := newAdminClient(t, env)
	company := catalogCompany(t, c, "商品公司")
	for _, fields := range []map[string]string{{"company_id": company.ID}, {"company": "不应自动创建公司"}} {
		status, data := c.request(t, "POST", "/api/other-items", fields)
		assertStatus(t, status, 422, data)
	}
	idea := createTestIdea(t, c, "other-item", "可拿出的任务物品")
	item := catalogItem(t, c, "other-items", "旧钥匙", map[string]any{
		"english_name": "Old Key", "category": "任务物品", "idea_id": idea.ID,
		"attributes": []Attribute{{Key: "重量", Value: "20g", Certainty: "tentative"}},
	})
	if item.ItemKind != "other" || item.Company != "" || item.CompanyID != "" || item.EnglishName != "Old Key" || len(item.Attributes) != 1 || len(item.Links) != 1 {
		t.Fatalf("incomplete other item: %+v", item)
	}
	for _, path := range []string{"/api/products/" + item.ID, "/api/tools/" + item.ID, "/api/products/" + item.ID + "/revisions"} {
		status, data := c.request(t, "GET", path, nil)
		assertStatus(t, status, 404, data)
	}
	status, data := c.request(t, "PATCH", "/api/other-items/"+item.ID, map[string]any{"version": 1, "company_id": company.ID})
	assertStatus(t, status, 422, data)
	status, data = c.request(t, "PATCH", "/api/other-items/"+item.ID, map[string]any{"version": 1, "english_name": "Rusty Key", "attributes": []Attribute{{Key: "重量", Value: "25g", Certainty: "asserted"}}})
	assertStatus(t, status, 200, data)
	updated := decodeTestJSON[Product](t, data)
	if updated.Version != 2 || updated.Attributes[0].Value != "25g" {
		t.Fatalf("edit was lost: %s", data)
	}
	status, data = c.request(t, "PATCH", "/api/other-items/"+item.ID, map[string]any{"version": 1, "chinese_name": "过期修改"})
	assertStatus(t, status, 409, data)
	status, data = c.request(t, "GET", "/api/other-items/"+item.ID+"/revisions", nil)
	assertStatus(t, status, 200, data)
	revisions := decodeTestJSON[struct{ Items []Revision }](t, data).Items
	status, data = c.request(t, "POST", "/api/other-items/"+item.ID+"/revisions/"+revisions[len(revisions)-1].ID+"/restore", map[string]int{"version": 2})
	assertStatus(t, status, 200, data)
	status, data = c.request(t, "GET", "/api/other-items/"+item.ID, nil)
	assertStatus(t, status, 200, data)
	restored := decodeTestJSON[Product](t, data)
	if restored.EnglishName != "Old Key" || restored.CompanyID != "" || restored.Category != "任务物品" || restored.Attributes[0].Value != "20g" || restored.Version != 3 {
		t.Fatalf("history restore changed item identity: %s", data)
	}
	for _, query := range []string{"Old Key", "重量", "20g", "任务物品"} {
		status, data = c.request(t, "GET", "/api/other-items?q="+url.QueryEscape(query), nil)
		assertStatus(t, status, 200, data)
		if decodeTestJSON[struct{ Total int }](t, data).Total != 1 {
			t.Fatalf("search omitted %q: %s", query, data)
		}
	}
	status, data = c.request(t, "GET", "/api/other-items/filters", nil)
	assertStatus(t, status, 200, data)
	filters := decodeTestJSON[struct{ Companies, Categories, Attributes []string }](t, data)
	if len(filters.Companies) != 0 || !slices.Equal(filters.Categories, []string{"任务物品"}) || !slices.Equal(filters.Attributes, []string{"重量"}) {
		t.Fatalf("wrong filters: %s", data)
	}
}

func TestOtherItemCategoriesRemainIndependentDuringDeletionAndHistoryRestore(t *testing.T) {
	env := newTestEnvironment(t, nil)
	c := newAdminClient(t, env)
	goods := catalogItem(t, c, "products", "商品材料", map[string]any{"category": "材料"})
	other := catalogItem(t, c, "other-items", "废旧零件", map[string]any{"category": "材料"})
	newProductOption(t, c, "other-categories", "任务物品")
	group := catalogGroup(t, c, goods)
	status, data := c.request(t, "POST", "/api/recipe-groups/"+group.ID+"/recipes", recipeFields{OutputQuantity: 1, Ingredients: []Ingredient{{Category: "材料", Quantity: 1}, {ProductID: other.ID, Quantity: 2}}})
	assertStatus(t, status, 201, data)
	recipe := decodeTestJSON[Recipe](t, data)
	status, data = c.request(t, "GET", "/api/product-options/other-categories", nil)
	assertStatus(t, status, 200, data)
	state := decodeTestJSON[optionState](t, data)
	status, data = c.request(t, "PATCH", "/api/product-options/other-categories/order", map[string]any{"order": []string{"任务物品", "材料"}, "revision": state.Revision})
	assertStatus(t, status, 200, data)
	status, data = c.request(t, "GET", "/api/product-options/other-categories/usage?name="+url.QueryEscape("材料"), nil)
	assertStatus(t, status, 200, data)
	usage := decodeTestJSON[categoryUsage](t, data)
	if usage.ItemCount != 1 || usage.RecipeCount != 0 {
		t.Fatalf("other category included product references: %s", data)
	}
	status, data = c.request(t, "DELETE", "/api/product-options/other-categories", map[string]string{"name": "材料", "token": usage.Token, "replacement": "任务物品"})
	assertStatus(t, status, 200, data)
	status, data = c.request(t, "GET", "/api/products/"+goods.ID, nil)
	assertStatus(t, status, 200, data)
	if item := decodeTestJSON[Product](t, data); item.Category != "材料" || item.Version != goods.Version {
		t.Fatal("deleting other category changed a product")
	}
	storedRecipe, err := loadRecipe(context.Background(), env.db, recipe.ID, false)
	if err != nil || storedRecipe.Version != recipe.Version || storedRecipe.Ingredients[0].Category != "材料" {
		t.Fatalf("deleting other category changed a recipe: %+v %v", storedRecipe, err)
	}
	status, data = c.request(t, "PATCH", "/api/other-items/"+other.ID, map[string]any{"version": 2, "category": "材料"})
	assertStatus(t, status, 409, data)
	if testError(t, data) != "category_deleted" {
		t.Fatalf("old draft resurrected the category: %s", data)
	}
	status, data = c.request(t, "GET", "/api/other-items/"+other.ID+"/revisions", nil)
	assertStatus(t, status, 200, data)
	revisions := decodeTestJSON[struct{ Items []Revision }](t, data).Items
	status, data = c.request(t, "POST", "/api/other-items/"+other.ID+"/revisions/"+revisions[len(revisions)-1].ID+"/restore", map[string]int{"version": 2})
	assertStatus(t, status, 409, data)
	newProductOption(t, c, "other-categories", "材料")
	status, data = c.request(t, "POST", "/api/other-items/"+other.ID+"/revisions/"+revisions[len(revisions)-1].ID+"/restore", map[string]int{"version": 2})
	assertStatus(t, status, 200, data)
	// Deleting the product category must likewise leave the other item intact.
	status, data = c.request(t, "GET", "/api/product-options/categories/usage?name="+url.QueryEscape("材料"), nil)
	assertStatus(t, status, 200, data)
	usage = decodeTestJSON[categoryUsage](t, data)
	if usage.ItemCount != 1 || usage.RecipeCount != 1 {
		t.Fatalf("product category included other references: %s", data)
	}
	newProductOption(t, c, "categories", "商品替代分类")
	status, data = c.request(t, "DELETE", "/api/product-options/categories", map[string]string{"name": "材料", "token": usage.Token, "replacement": "商品替代分类"})
	assertStatus(t, status, 200, data)
	status, data = c.request(t, "GET", "/api/other-items/"+other.ID, nil)
	assertStatus(t, status, 200, data)
	if item := decodeTestJSON[Product](t, data); item.Category != "材料" || item.Version != 3 {
		t.Fatalf("product category deletion changed other item: %s", data)
	}
}

func TestOtherItemsSearchMCPTrashAndExport(t *testing.T) {
	env := newTestEnvironment(t, nil)
	c := newAdminClient(t, env)
	item := catalogItem(t, c, "other-items", "独立物品搜索", map[string]any{"english_name": "Other Search Item", "category": "独立分类"})
	for _, path := range []string{"/api/products", "/api/tools"} {
		status, data := c.request(t, "GET", path, nil)
		assertStatus(t, status, 200, data)
		if decodeTestJSON[struct{ Total int }](t, data).Total != 0 {
			t.Fatalf("other item leaked into %s", path)
		}
	}
	status, data := c.request(t, "GET", "/api/search?kind=other-items&q=Other", nil)
	assertStatus(t, status, 200, data)
	result := decodeTestJSON[struct {
		OtherItems      []Product `json:"other_items"`
		Products, Tools []Product
	}](t, data)
	if len(result.OtherItems) != 1 || len(result.Products)+len(result.Tools) != 0 {
		t.Fatalf("wrong search grouping: %s", data)
	}
	page, err := env.app.mcpListRecords(context.Background(), mcpListArgs{Kinds: []string{"other"}, Query: "Other"})
	if err != nil || len(page.Results) != 1 || page.Results[0].URL != env.server.URL+"/other-items/"+item.ID {
		t.Fatalf("MCP cannot find other items: %+v %v", page, err)
	}
	detail, err := env.app.mcpFetch(context.Background(), "other:"+item.ID)
	if err != nil || detail["kind"] != "other" {
		t.Fatalf("MCP cannot fetch other item: %+v %v", detail, err)
	}
	status, data = c.request(t, "DELETE", "/api/other-items/"+item.ID, map[string]int{"version": 1})
	assertStatus(t, status, 200, data)
	for _, endpoint := range []string{"/api/trash", "/api/export"} {
		status, data = c.request(t, "GET", endpoint, nil)
		assertStatus(t, status, 200, data)
		result = decodeTestJSON[struct {
			OtherItems      []Product `json:"other_items"`
			Products, Tools []Product
		}](t, data)
		if len(result.OtherItems) != 1 || len(result.Products)+len(result.Tools) != 0 || result.OtherItems[0].DeletedAt == "" {
			t.Fatalf("wrong lifecycle grouping at %s: %s", endpoint, data)
		}
	}
	status, data = c.request(t, "POST", "/api/other-items/"+item.ID+"/restore", map[string]int{"version": 2})
	assertStatus(t, status, 200, data)
	status, data = c.request(t, "DELETE", "/api/other-items/"+item.ID, map[string]int{"version": 3})
	assertStatus(t, status, 200, data)
	status, data = c.request(t, "DELETE", "/api/other-items/"+item.ID+"/permanent", map[string]int{"version": 4})
	assertStatus(t, status, 200, data)
	status, data = c.request(t, "GET", "/api/other-items/"+item.ID, nil)
	assertStatus(t, status, 404, data)
}
