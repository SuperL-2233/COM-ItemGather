package app

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestIdeaProductWorkflow(t *testing.T) {
	environment := newTestEnvironment(t, nil)
	client := newAdminClient(t, environment)
	idea := createTestIdea(t, client, "same-request", "绿色风暴")
	duplicate := createTestIdea(t, client, "same-request", "绿色风暴")
	if duplicate.ID != idea.ID {
		t.Fatal("idempotent retry created another idea")
	}
	idea = waitAIStatus(t, client, idea.ID, "succeeded")
	if idea.OriginalContent != "绿色风暴" || len(idea.Attributes) != 1 || idea.Attributes[0].Certainty != "tentative" {
		t.Fatalf("unexpected organized idea %+v", idea)
	}
	status, data := client.request(t, "PATCH", "/api/ideas/"+idea.ID, map[string]any{"version": idea.Version, "display_title": "人工标题"})
	assertStatus(t, status, 200, data)
	updated := decodeTestJSON[Idea](t, data)
	status, data = client.request(t, "PATCH", "/api/ideas/"+idea.ID, map[string]any{"version": idea.Version, "display_title": "覆盖别人的修改"})
	assertStatus(t, status, 409, data)
	if testError(t, data) != "version_conflict" {
		t.Fatal("expected version conflict")
	}
	products := []Product{}
	for _, name := range []string{"青苹果汽水", "加速饮料"} {
		status, data = client.request(t, "POST", "/api/products", map[string]any{"internal_name": name, "company": "青苹果公司", "development_status": "concept"})
		assertStatus(t, status, 201, data)
		product := decodeTestJSON[Product](t, data)
		if product.Company != "青苹果公司" {
			t.Fatalf("product company was not saved: %+v", product)
		}
		products = append(products, product)
	}
	status, data = client.request(t, "PATCH", "/api/products/"+products[0].ID, map[string]any{"version": products[0].Version, "company": "新的所属公司"})
	assertStatus(t, status, 200, data)
	products[0] = decodeTestJSON[Product](t, data)
	status, data = client.request(t, "GET", "/api/products?q="+url.QueryEscape("新的所属公司"), nil)
	assertStatus(t, status, 200, data)
	companyPage := decodeTestJSON[struct {
		Total int `json:"total"`
	}](t, data)
	if companyPage.Total != 1 {
		t.Fatalf("product company search returned %d results", companyPage.Total)
	}
	for _, product := range products {
		status, data = client.request(t, "POST", "/api/links", map[string]string{"idea_id": idea.ID, "product_id": product.ID, "status": "candidate"})
		assertStatus(t, status, 201, data)
	}
	status, data = client.request(t, "GET", "/api/ideas/"+idea.ID, nil)
	assertStatus(t, status, 200, data)
	updated = decodeTestJSON[Idea](t, data)
	if updated.ProductCount != 2 {
		t.Fatalf("expected two linked products, got %d", updated.ProductCount)
	}
	status, data = client.request(t, "POST", "/api/ideas/"+idea.ID+"/favorite", map[string]bool{"favorite": true})
	assertStatus(t, status, 200, data)
	createTestUser(t, environment.db, "other", "member")
	other := newTestClient(t, environment.server)
	other.login(t, "other", "correct-horse-42")
	status, data = other.request(t, "GET", "/api/ideas/"+idea.ID, nil)
	assertStatus(t, status, 200, data)
	if decodeTestJSON[Idea](t, data).Favorite {
		t.Fatal("favorite leaked to another user")
	}
	status, data = client.request(t, "GET", "/api/products?limit=30", nil)
	assertStatus(t, status, 200, data)
	status, data = client.request(t, "DELETE", "/api/ideas/"+idea.ID, map[string]int{"version": updated.Version})
	assertStatus(t, status, 200, data)
	status, data = client.request(t, "GET", "/api/products/"+products[0].ID, nil)
	assertStatus(t, status, 200, data)
	deleted := decodeTestJSON[map[string]any](t, data)
	version := int(deleted["version"].(float64))
	_ = version
	status, data = client.request(t, "GET", "/api/trash", nil)
	assertStatus(t, status, 200, data)
	trash := decodeTestJSON[map[string][]Idea](t, data)
	_ = trash
	status, data = client.request(t, "GET", "/api/ideas/"+idea.ID, nil)
	assertStatus(t, status, 404, data)
	status, data = client.request(t, "GET", "/api/trash", nil)
	trashData := decodeTestJSON[struct {
		Ideas    []Idea    `json:"ideas"`
		Products []Product `json:"products"`
	}](t, data)
	deletedIdea := trashData.Ideas[0]
	status, data = client.request(t, "POST", "/api/ideas/"+idea.ID+"/restore", map[string]int{"version": deletedIdea.Version})
	assertStatus(t, status, 200, data)
	status, data = client.request(t, "GET", "/api/search?q="+url.QueryEscape("青苹果"), nil)
	assertStatus(t, status, 200, data)
	search := decodeTestJSON[struct {
		Ideas    []Idea    `json:"ideas"`
		Products []Product `json:"products"`
	}](t, data)
	if len(search.Ideas) == 0 || len(search.Products) == 0 {
		t.Fatal("Chinese content, attributes, or product search failed")
	}
	status, data = client.request(t, "GET", "/api/ideas/"+idea.ID+"/revisions", nil)
	assertStatus(t, status, 200, data)
	revisions := decodeTestJSON[map[string][]Revision](t, data)["items"]
	if len(revisions) < 3 {
		t.Fatal("missing revision history")
	}
	current := waitAIStatus(t, client, idea.ID, "succeeded")
	status, data = client.request(t, "POST", "/api/ideas/"+idea.ID+"/revisions/"+revisions[len(revisions)-1].ID+"/restore", map[string]int{"version": current.Version})
	assertStatus(t, status, 200, data)
	status, data = client.request(t, "GET", "/api/export", nil)
	assertStatus(t, status, 200, data)
	if bytes.Contains(data, []byte("password_hash")) || bytes.Contains(data, []byte("token_hash")) {
		t.Fatal("export leaked authentication data")
	}
	var exported map[string]json.RawMessage
	if json.Unmarshal(data, &exported) != nil || exported["ideas"] == nil || exported["links"] == nil {
		t.Fatal("invalid export")
	}
}

func TestPureImageAndUploadValidation(t *testing.T) {
	environment := newTestEnvironment(t, func(w http.ResponseWriter, r *http.Request) { t.Error("AI must not be called for image-only ideas") })
	client := newAdminClient(t, environment)
	idea := createTestIdea(t, client, "image-only", "")
	if idea.AIStatus != "skipped" {
		t.Fatalf("image idea status %s", idea.AIStatus)
	}
	imageData := &bytes.Buffer{}
	if err := png.Encode(imageData, image.NewRGBA(image.Rect(0, 0, 20, 20))); err != nil {
		t.Fatal(err)
	}
	status, data := client.multipart(t, "/api/attachments", idea.ID, imageData.Bytes(), "../../reference.png")
	assertStatus(t, status, 201, data)
	attachment := decodeTestJSON[Attachment](t, data)
	if !strings.HasPrefix(attachment.URL, "/api/attachments/") || attachment.ThumbnailURL == "" {
		t.Fatal("missing protected attachment URL or thumbnail")
	}
	anonymous := newTestClient(t, environment.server)
	status, data = anonymous.request(t, "GET", attachment.URL, nil)
	assertStatus(t, status, 401, data)
	status, data = client.request(t, "GET", attachment.URL, nil)
	assertStatus(t, status, 200, data)
	if !bytes.Equal(data, imageData.Bytes()) {
		t.Fatal("original image was not preserved")
	}
	status, data = client.request(t, "GET", attachment.ThumbnailURL, nil)
	assertStatus(t, status, 200, data)
	status, data = client.multipart(t, "/api/attachments", idea.ID, []byte("not an image"), "fake.png")
	assertStatus(t, status, 415, data)
	status, data = client.request(t, "DELETE", "/api/attachments/"+attachment.ID, nil)
	assertStatus(t, status, 200, data)
	status, data = client.request(t, "GET", "/api/ideas/"+idea.ID, nil)
	assertStatus(t, status, 200, data)
	if len(decodeTestJSON[Idea](t, data).Attachments) != 0 {
		t.Fatal("attachment reference was not removed")
	}
	var taskCount int
	environment.db.QueryRow("SELECT COUNT(*) FROM ai_tasks WHERE idea_id=?", idea.ID).Scan(&taskCount)
	if taskCount != 0 {
		t.Fatal("image-only idea created an AI task")
	}
}
func TestMultipleConceptsRemainOneIdea(t *testing.T) {
	environment := newTestEnvironment(t, nil)
	client := newAdminClient(t, environment)
	idea := createTestIdea(t, client, "multiple", "一个会打嗝的饮料；另一个报纸香肠包装")
	waitAIStatus(t, client, idea.ID, "succeeded")
	status, data := client.request(t, "GET", "/api/ideas", nil)
	assertStatus(t, status, 200, data)
	page := decodeTestJSON[struct {
		Total int `json:"total"`
	}](t, data)
	if page.Total != 1 {
		t.Fatalf("one submission created %d ideas", page.Total)
	}
}
