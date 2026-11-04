package app

import (
	"bytes"
	"net/http"
	"testing"
)

type testAgentCatalog struct {
	Products     []Product     `json:"products"`
	Tools        []Product     `json:"tools"`
	Groups       []RecipeGroup `json:"recipe_groups"`
	Recipes      []Recipe      `json:"recipes"`
	SettingLinks []settingLink `json:"setting_links"`
}

func TestAgentCatalogOmitsDeletedNestedProducts(t *testing.T) {
	environment := newTestEnvironment(t, nil)
	client := newAdminClient(t, environment)
	material := catalogItem(t, client, "products", "已删除的秘密材料", map[string]any{"description": "不应提供给 AI 的材料说明"})
	tool := catalogItem(t, client, "tools", "已删除的秘密道具", nil)
	live := catalogItem(t, client, "products", "保留的材料", nil)
	output := catalogItem(t, client, "products", "即将删除的秘密产物", nil)
	group := catalogGroup(t, client, output)
	status, data := client.request(t, http.MethodPost, "/api/recipe-groups/"+group.ID+"/recipes", recipeFields{
		Name: "保留的配方", OutputQuantity: 1,
		Ingredients: []Ingredient{{ProductID: material.ID, Quantity: 2}, {ProductID: tool.ID, Quantity: 3}, {ProductID: live.ID, Quantity: 1}, {Category: "水果", Quantity: 4}},
	})
	assertStatus(t, status, 201, data)
	for _, item := range []Product{material, tool} {
		status, data = client.request(t, http.MethodDelete, "/api/products/"+item.ID, map[string]int{"version": item.Version})
		assertStatus(t, status, 200, data)
	}
	token := createTestAgentToken(t, client)
	status, data = agentRequest(t, environment, token.Token, "/api/agent/v1/catalog")
	assertStatus(t, status, 200, data)
	snapshot := decodeTestJSON[testAgentCatalog](t, data)
	if len(snapshot.Products) != 2 || len(snapshot.Tools) != 0 || len(snapshot.Groups) != 1 || len(snapshot.Recipes) != 1 || len(snapshot.Groups[0].Recipes) != 1 {
		t.Fatalf("快照应保留未删除的物品与配方: %s", data)
	}
	if snapshot.Groups[0].Output.ID != output.ID {
		t.Fatal("未删除的配方产物应保留完整内容")
	}
	for _, recipe := range []Recipe{snapshot.Recipes[0], snapshot.Groups[0].Recipes[0]} {
		if len(recipe.Ingredients) != 4 {
			t.Fatal("删除物品不应丢失配方的材料引用或数量")
		}
		for i, item := range []Product{material, tool} {
			ingredient := recipe.Ingredients[i]
			if ingredient.ProductID != item.ID || ingredient.Quantity != i+2 || ingredient.Item != nil {
				t.Fatal("已删除材料应只保留引用 ID 和数量")
			}
		}
		if ingredient := recipe.Ingredients[2]; ingredient.Item == nil || ingredient.Item.ID != live.ID {
			t.Fatal("未删除材料应保留完整内容")
		}
		if ingredient := recipe.Ingredients[3]; ingredient.Category != "水果" || ingredient.Quantity != 4 || ingredient.Item != nil {
			t.Fatal("类别材料应保留原有内容")
		}
	}
	for _, secret := range []string{material.ChineseName, material.Description, tool.ChineseName, `"created_by"`, `"deleted_at"`} {
		if bytes.Contains(data, []byte(secret)) {
			t.Errorf("快照不应包含已删除内容或私有字段 %q", secret)
		}
	}

	status, data = client.request(t, http.MethodDelete, "/api/products/"+output.ID, map[string]int{"version": output.Version})
	assertStatus(t, status, 200, data)
	status, data = agentRequest(t, environment, token.Token, "/api/agent/v1/catalog")
	assertStatus(t, status, 200, data)
	snapshot = decodeTestJSON[testAgentCatalog](t, data)
	if len(snapshot.Groups) != 1 || len(snapshot.Recipes) != 1 || snapshot.Groups[0].OutputID != output.ID {
		t.Fatal("已删除产物的引用与未删除的配方仍应保留")
	}
	raw := decodeTestJSON[map[string]any](t, data)
	groupFields := raw["recipe_groups"].([]any)[0].(map[string]any)
	if _, exists := groupFields["output"]; exists || bytes.Contains(data, []byte(output.ChineseName)) {
		t.Fatal("配方页面不应内嵌已删除产物的内容")
	}

	status, data = client.request(t, http.MethodGet, "/api/recipe-groups/"+group.ID, nil)
	assertStatus(t, status, 200, data)
	ordinary := decodeTestJSON[RecipeGroup](t, data)
	if ordinary.Output.DeletedAt == "" || ordinary.Recipes[0].Ingredients[0].Item.DeletedAt == "" {
		t.Fatal("网页登录后的配方仍应保留已删除物品及删除标识")
	}
	status, data = client.request(t, http.MethodGet, "/api/export", nil)
	assertStatus(t, status, 200, data)
	exported := decodeTestJSON[testAgentCatalog](t, data)
	if len(exported.Products) != 3 || len(exported.Tools) != 1 || exported.Groups[0].Output.DeletedAt == "" || exported.Recipes[0].Ingredients[0].Item.DeletedAt == "" {
		t.Fatal("完整导出仍应包含回收站内容和删除标识")
	}
}

func TestAgentCatalogOmitsRecipesFromDeletedGroups(t *testing.T) {
	environment := newTestEnvironment(t, nil)
	client := newAdminClient(t, environment)
	liveGroup := catalogGroup(t, client, catalogItem(t, client, "products", "保留的产物", nil))
	deletedGroup := catalogGroup(t, client, catalogItem(t, client, "products", "另一件产物", nil))
	idea := createTestIdea(t, client, "agent-group-links", "配方关联灵感")
	recipes := make([]Recipe, 3)
	for i, group := range []RecipeGroup{liveGroup, deletedGroup, liveGroup} {
		status, data := client.request(t, http.MethodPost, "/api/recipe-groups/"+group.ID+"/recipes", recipeFields{Name: []string{"保留的配方", "页面回收站中的秘密配方", "单独删除的秘密配方"}[i], OutputQuantity: 1})
		assertStatus(t, status, 201, data)
		recipes[i] = decodeTestJSON[Recipe](t, data)
		status, data = client.request(t, http.MethodPost, "/api/setting-links", map[string]string{"idea_id": idea.ID, "entity_type": "recipe", "entity_id": recipes[i].ID})
		assertStatus(t, status, 201, data)
	}
	status, data := client.request(t, http.MethodDelete, "/api/recipe-groups/"+deletedGroup.ID, map[string]int{"version": deletedGroup.Version})
	assertStatus(t, status, 200, data)
	status, data = client.request(t, http.MethodDelete, "/api/recipes/"+recipes[2].ID, map[string]int{"version": recipes[2].Version})
	assertStatus(t, status, 200, data)
	token := createTestAgentToken(t, client)
	status, data = agentRequest(t, environment, token.Token, "/api/agent/v1/catalog")
	assertStatus(t, status, 200, data)
	snapshot := decodeTestJSON[testAgentCatalog](t, data)
	if len(snapshot.Groups) != 1 || snapshot.Groups[0].ID != liveGroup.ID || len(snapshot.Groups[0].Recipes) != 1 || len(snapshot.Recipes) != 1 || snapshot.Recipes[0].ID != recipes[0].ID {
		t.Fatalf("快照应只包含未删除页面及其未删除配方: %s", data)
	}
	if len(snapshot.SettingLinks) != 1 || snapshot.SettingLinks[0].EntityID != recipes[0].ID {
		t.Fatal("已删除页面或配方的关联不应出现在快照中")
	}
	for _, deleted := range recipes[1:] {
		if bytes.Contains(data, []byte(deleted.Name)) {
			t.Errorf("快照泄露已删除配方 %q", deleted.Name)
		}
	}
	status, data = client.request(t, http.MethodGet, "/api/export", nil)
	assertStatus(t, status, 200, data)
	exported := decodeTestJSON[testAgentCatalog](t, data)
	if len(exported.Groups) != 2 || len(exported.Recipes) != 3 || len(exported.SettingLinks) != 3 {
		t.Fatal("完整导出仍应包含已删除页面、配方和关联")
	}
	status, data = client.request(t, http.MethodPost, "/api/recipe-groups/"+deletedGroup.ID+"/restore", map[string]int{"version": deletedGroup.Version + 1})
	assertStatus(t, status, 200, data)
	status, data = agentRequest(t, environment, token.Token, "/api/agent/v1/catalog")
	assertStatus(t, status, 200, data)
	snapshot = decodeTestJSON[testAgentCatalog](t, data)
	if len(snapshot.Groups) != 2 || len(snapshot.Recipes) != 2 || len(snapshot.SettingLinks) != 2 {
		t.Fatal("恢复页面后，应重新提供其未删除配方和关联")
	}
}
