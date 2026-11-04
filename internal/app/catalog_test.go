package app

import (
	"bytes"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"testing"
)

func catalogItem(t *testing.T, c *testClient, kind, name string, extra map[string]any) Product {
	t.Helper()
	if extra == nil {
		extra = map[string]any{}
	}
	extra["chinese_name"] = name
	status, data := c.request(t, "POST", "/api/"+kind, extra)
	assertStatus(t, status, 201, data)
	return decodeTestJSON[Product](t, data)
}
func catalogCompany(t *testing.T, c *testClient, name string) Company {
	t.Helper()
	status, data := c.request(t, "POST", "/api/companies", map[string]string{"chinese_name": name, "english_name": "Floating Bubble", "aliases": "泡泡"})
	assertStatus(t, status, 201, data)
	return decodeTestJSON[Company](t, data)
}
func catalogGroup(t *testing.T, c *testClient, item Product) RecipeGroup {
	t.Helper()
	status, data := c.request(t, "POST", "/api/recipe-groups", map[string]string{"output_id": item.ID})
	assertStatus(t, status, 201, data)
	return decodeTestJSON[RecipeGroup](t, data)
}
func TestCompanyIdentitySearchMergeAndHistory(t *testing.T) {
	env := newTestEnvironment(t, nil)
	c := newAdminClient(t, env)
	company := catalogCompany(t, c, "浮泡饮料")
	item := catalogItem(t, c, "products", "色乐", map[string]any{"company_id": company.ID, "english_name": "Color"})
	legacy := catalogItem(t, c, "products", "范特", map[string]any{"company": "浮泡饮料"})
	if item.CompanyID != legacy.CompanyID {
		t.Fatal("legacy name did not resolve to existing company")
	}
	company.ChineseName = "浮泡饮品"
	status, data := c.request(t, "PATCH", "/api/companies/"+company.ID, map[string]any{"version": company.Version, "chinese_name": company.ChineseName, "english_name": company.EnglishName, "aliases": company.Aliases, "description": "商战中的饮料公司"})
	assertStatus(t, status, 200, data)
	company = decodeTestJSON[Company](t, data)
	status, data = c.request(t, "GET", "/api/products/"+item.ID, nil)
	assertStatus(t, status, 200, data)
	renamed := decodeTestJSON[Product](t, data)
	if renamed.Company != "浮泡饮品" || renamed.CompanyID != company.ID {
		t.Fatal("company rename was not reflected")
	}
	status, data = c.request(t, "GET", "/api/products?q="+url.QueryEscape("泡泡"), nil)
	assertStatus(t, status, 200, data)
	if len(decodeTestJSON[struct {
		Items []Product `json:"items"`
	}](t, data).Items) != 2 {
		t.Fatal("company alias is not searchable")
	}
	status, data = c.request(t, "PATCH", "/api/companies/"+company.ID, map[string]any{"version": 1, "chinese_name": "旧修改"})
	assertStatus(t, status, 409, data)
	status, data = c.request(t, "DELETE", "/api/companies/"+company.ID, map[string]int{"version": company.Version})
	assertStatus(t, status, 409, data)
	target := catalogCompany(t, c, "浮泡集团")
	status, data = c.request(t, "POST", "/api/companies/"+company.ID+"/merge", map[string]any{"version": company.Version, "target_id": target.ID, "target_version": target.Version})
	assertStatus(t, status, 200, data)
	status, data = c.request(t, "GET", "/api/products/"+item.ID, nil)
	assertStatus(t, status, 200, data)
	merged := decodeTestJSON[Product](t, data)
	if merged.CompanyID != target.ID || merged.Version <= item.Version {
		t.Fatal("merge did not move item with a new revision")
	}
	status, data = c.request(t, "GET", "/api/companies/"+company.ID, nil)
	assertStatus(t, status, 404, data)
	status, data = c.request(t, "GET", "/api/companies/"+target.ID+"/revisions", nil)
	assertStatus(t, status, 200, data)
	revisions := decodeTestJSON[struct {
		Items []Revision `json:"items"`
	}](t, data).Items
	if len(revisions) != 1 {
		t.Fatalf("company history missing: %s", data)
	}
	status, data = c.request(t, "POST", "/api/companies/"+target.ID+"/revisions/"+revisions[0].ID+"/restore", map[string]int{"version": target.Version})
	assertStatus(t, status, 200, data)
}
func TestToolCategoriesSourcesAndItemSeparation(t *testing.T) {
	env := newTestEnvironment(t, nil)
	c := newAdminClient(t, env)
	for _, name := range []string{" 主动＋被动 ", "主动＋被动"} {
		status, data := c.request(t, "POST", "/api/tool-categories", map[string]string{"name": name})
		assertStatus(t, status, 200, data)
		categories := decodeTestJSON[struct {
			Items []string `json:"items"`
		}](t, data).Items
		if len(categories) != 3 {
			t.Fatalf("categories not shared/deduplicated: %v", categories)
		}
	}
	settings := ToolSettings{Trigger: "按 F 后免疫下次伤害", Effect: "免疫伤害", Sources: []AcquisitionSource{{Method: "购买", Provider: "商店", Price: "0"}, {Method: "NPC 赠送", Provider: "奶奶", Timing: "局外"}}, ModelStatus: "design", FunctionStatus: "production"}
	tool := catalogItem(t, c, "tools", "奶奶的玉镯", map[string]any{"use_category": "主动＋被动", "tool_settings": settings})
	goods := catalogItem(t, c, "products", "猕猴桃", nil)
	if tool.ItemKind != "tool" || goods.ItemKind != "product" || len(tool.ToolSettings.Sources) != 2 {
		t.Fatal("tool definition was not retained")
	}
	for _, path := range []string{"/api/products", "/api/tools?use_category=" + url.QueryEscape("主动＋被动"), "/api/tools?source=" + url.QueryEscape("购买"), "/api/tools?q=" + url.QueryEscape("奶奶")} {
		status, data := c.request(t, "GET", path, nil)
		assertStatus(t, status, 200, data)
		items := decodeTestJSON[struct {
			Items []Product `json:"items"`
		}](t, data).Items
		if len(items) != 1 {
			t.Fatalf("wrong item list for %s: %s", path, data)
		}
	}
	status, data := c.request(t, "GET", "/api/tools?source="+url.QueryEscape("任务奖励"), nil)
	assertStatus(t, status, 200, data)
	if len(decodeTestJSON[struct {
		Items []Product `json:"items"`
	}](t, data).Items) != 0 {
		t.Fatal("source filtering is not applied on server")
	}
	status, data = c.request(t, "GET", "/api/search?kind=tools&q="+url.QueryEscape("免疫"), nil)
	assertStatus(t, status, 200, data)
	result := decodeTestJSON[struct {
		Products []Product `json:"products"`
		Tools    []Product `json:"tools"`
	}](t, data)
	if len(result.Tools) != 1 || len(result.Products) != 0 {
		t.Fatal("tool search leaked into product results")
	}
	anonymous := newTestClient(t, env.server)
	status, data = anonymous.request(t, "GET", "/api/tools", nil)
	assertStatus(t, status, 401, data)
}
func TestRecipesCrossKindQuantitiesConflictsAndTrash(t *testing.T) {
	env := newTestEnvironment(t, nil)
	c := newAdminClient(t, env)
	fruit := catalogItem(t, c, "products", "猕猴桃", map[string]any{"category": "水果"})
	chip := catalogItem(t, c, "tools", "水果芯片", nil)
	output := catalogItem(t, c, "tools", "智能玉镯", nil)
	group := catalogGroup(t, c, output)
	status, data := c.request(t, "POST", "/api/recipe-groups", map[string]string{"output_id": output.ID})
	assertStatus(t, status, 200, data)
	if decodeTestJSON[RecipeGroup](t, data).ID != group.ID {
		t.Fatal("duplicate output created another group")
	}
	fields := recipeFields{Name: "基础配方", OutputQuantity: 1, Verification: "untested", DevelopmentStatus: "design", Ingredients: []Ingredient{{ProductID: fruit.ID, Quantity: 1}, {ProductID: chip.ID, Quantity: 2}, {ProductID: fruit.ID, Quantity: 3}, {Category: "水果", Quantity: 1}}}
	status, data = c.request(t, "POST", "/api/recipe-groups/"+group.ID+"/recipes", fields)
	assertStatus(t, status, 201, data)
	recipe := decodeTestJSON[Recipe](t, data)
	if len(recipe.Ingredients) != 3 || recipe.Ingredients[0].Quantity != 4 || recipe.Ingredients[1].Item.ItemKind != "tool" {
		t.Fatalf("ingredients not combined correctly: %+v", recipe.Ingredients)
	}
	fields.Name = "另一套方案"
	status, data = c.request(t, "POST", "/api/recipe-groups/"+group.ID+"/recipes", fields)
	assertStatus(t, status, 201, data)
	fields.Version = recipe.Version
	fields.Notes = "更正验证记录"
	status, data = c.request(t, "PATCH", "/api/recipes/"+recipe.ID, fields)
	assertStatus(t, status, 200, data)
	updated := decodeTestJSON[Recipe](t, data)
	status, data = c.request(t, "PATCH", "/api/recipes/"+recipe.ID, fields)
	assertStatus(t, status, 409, data)
	if !bytes.Contains(data, []byte("current")) {
		t.Fatal("conflict omitted current version")
	}
	status, data = c.request(t, "GET", "/api/recipe-groups?q="+url.QueryEscape("猕猴桃"), nil)
	assertStatus(t, status, 200, data)
	if len(decodeTestJSON[struct {
		Items []RecipeGroup `json:"items"`
	}](t, data).Items) != 1 {
		t.Fatal("material search failed")
	}
	status, data = c.request(t, "GET", "/api/recipes/"+recipe.ID+"/revisions", nil)
	assertStatus(t, status, 200, data)
	revisions := decodeTestJSON[struct {
		Items []Revision `json:"items"`
	}](t, data).Items
	status, data = c.request(t, "POST", "/api/recipes/"+recipe.ID+"/revisions/"+revisions[len(revisions)-1].ID+"/restore", map[string]int{"version": updated.Version})
	assertStatus(t, status, 200, data)
	restored := decodeTestJSON[Recipe](t, data)
	if restored.Notes != "" || restored.Version != updated.Version+1 {
		t.Fatal("recipe history was not restored")
	}
	status, data = c.request(t, "DELETE", "/api/products/"+fruit.ID, map[string]int{"version": fruit.Version})
	assertStatus(t, status, 200, data)
	status, data = c.request(t, "GET", "/api/recipe-groups/"+group.ID, nil)
	assertStatus(t, status, 200, data)
	g := decodeTestJSON[RecipeGroup](t, data)
	if len(g.Recipes) != 2 || g.Recipes[0].Ingredients[0].Item.DeletedAt == "" {
		t.Fatal("deleted material was not retained")
	}
	status, data = c.request(t, "DELETE", "/api/products/"+fruit.ID+"/permanent", map[string]int{"version": fruit.Version + 1})
	assertStatus(t, status, 409, data)
	status, data = c.request(t, "DELETE", "/api/recipes/"+recipe.ID, map[string]int{"version": restored.Version})
	assertStatus(t, status, 200, data)
	status, data = c.request(t, "GET", "/api/trash", nil)
	assertStatus(t, status, 200, data)
	trash := decodeTestJSON[struct {
		Recipes []Recipe `json:"recipes"`
	}](t, data)
	if len(trash.Recipes) != 1 {
		t.Fatal("recipe missing from recycle bin")
	}
	status, data = c.request(t, "POST", "/api/recipes/"+recipe.ID+"/restore", map[string]int{"version": restored.Version + 1})
	assertStatus(t, status, 200, data)
	status, data = c.request(t, "GET", "/api/export", nil)
	assertStatus(t, status, 200, data)
	exported := decodeTestJSON[map[string]any](t, data)
	if exported["format_version"] != float64(3) || len(exported["tools"].([]any)) != 2 || len(exported["recipes"].([]any)) != 2 {
		t.Fatal("export omitted catalog records")
	}
}
func TestRecipeValidationAndIdeaLinks(t *testing.T) {
	env := newTestEnvironment(t, nil)
	c := newAdminClient(t, env)
	item := catalogItem(t, c, "products", "待命名爆炸物", nil)
	group := catalogGroup(t, c, item)
	for _, ingredients := range [][]Ingredient{{{ProductID: item.ID, Quantity: 0}}, {{ProductID: item.ID, Category: "水果", Quantity: 1}}, {{ProductID: "missing", Quantity: 1}}, {{Quantity: 1}}} {
		status, data := c.request(t, "POST", "/api/recipe-groups/"+group.ID+"/recipes", recipeFields{OutputQuantity: 1, Ingredients: ingredients})
		if status < 400 || status >= 500 {
			t.Fatalf("invalid ingredient accepted: %d %s", status, data)
		}
	}
	status, data := c.request(t, "POST", "/api/recipe-groups", map[string]string{"effect": "爆炸"})
	assertStatus(t, status, 400, data)
	status, data = c.request(t, "POST", "/api/recipe-groups/"+group.ID+"/recipes", recipeFields{OutputQuantity: 1})
	assertStatus(t, status, 201, data)
	recipe := decodeTestJSON[Recipe](t, data)
	idea := createTestIdea(t, c, "recipe-link", "合成灵感")
	company := catalogCompany(t, c, "配方公司")
	for kind, id := range map[string]string{"recipe": recipe.ID, "company": company.ID} {
		status, data = c.request(t, "POST", "/api/setting-links", map[string]string{"idea_id": idea.ID, "entity_type": kind, "entity_id": id})
		assertStatus(t, status, 201, data)
	}
	status, data = c.request(t, "GET", "/api/setting-links?idea_id="+idea.ID, nil)
	assertStatus(t, status, 200, data)
	if len(decodeTestJSON[struct {
		Items []settingLink `json:"items"`
	}](t, data).Items) != 2 {
		t.Fatal("idea associations missing")
	}
	status, data = c.request(t, "DELETE", "/api/recipe-groups/"+group.ID, map[string]int{"version": group.Version})
	assertStatus(t, status, 200, data)
	status, data = c.request(t, "GET", "/api/setting-links?idea_id="+idea.ID, nil)
	assertStatus(t, status, 200, data)
	if len(decodeTestJSON[struct {
		Items []settingLink `json:"items"`
	}](t, data).Items) != 1 {
		t.Fatal("archived group's associations should be hidden")
	}
}
func companyUpload(t *testing.T, c *testClient, companyID, replaceID string, data []byte) (int, []byte) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	writer.WriteField("company_id", companyID)
	writer.WriteField("replace_id", replaceID)
	part, err := writer.CreateFormFile("file", "logo.png")
	if err != nil {
		t.Fatal(err)
	}
	part.Write(data)
	writer.Close()
	req, _ := http.NewRequest("POST", c.base+"/api/attachments", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-CSRF-Token", c.csrf)
	response, err := c.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(response.Body)
	return response.StatusCode, raw
}
func TestCompanyLogoReplacementAndThreeMiBLimit(t *testing.T) {
	env := newTestEnvironment(t, nil)
	env.app.cfg.MaxUploadBytes = 5 * 1024 * 1024
	c := newAdminClient(t, env)
	company := catalogCompany(t, c, "Logo 公司")
	var picture bytes.Buffer
	png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 16, 16)))
	status, data := companyUpload(t, c, company.ID, "", picture.Bytes())
	assertStatus(t, status, 201, data)
	logo := decodeTestJSON[Attachment](t, data)
	status, data = companyUpload(t, c, company.ID, logo.ID, bytes.Repeat([]byte("x"), 3*1024*1024+1))
	assertStatus(t, status, 413, data)
	status, data = companyUpload(t, c, company.ID, logo.ID, []byte("broken image"))
	assertStatus(t, status, 415, data)
	status, data = c.request(t, "GET", "/api/attachments/"+logo.ID, nil)
	assertStatus(t, status, 200, data)
	status, data = companyUpload(t, c, company.ID, "", picture.Bytes())
	assertStatus(t, status, 409, data)
	status, data = companyUpload(t, c, company.ID, logo.ID, picture.Bytes())
	assertStatus(t, status, 201, data)
	replacement := decodeTestJSON[Attachment](t, data)
	if replacement.ID == logo.ID {
		t.Fatal("logo was not replaced")
	}
	status, data = c.request(t, "GET", "/api/companies/"+company.ID, nil)
	assertStatus(t, status, 200, data)
	saved := decodeTestJSON[Company](t, data)
	if len(saved.Attachments) != 1 || saved.Attachments[0].ID != replacement.ID {
		t.Fatal("company must have exactly one logo")
	}
	status, data = c.request(t, "GET", "/api/attachments/"+logo.ID, nil)
	assertStatus(t, status, 404, data)
	anonymous := newTestClient(t, env.server)
	status, data = anonymous.request(t, "GET", "/api/attachments/"+replacement.ID, nil)
	assertStatus(t, status, 401, data)
}
