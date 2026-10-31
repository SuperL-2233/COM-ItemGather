package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"com-itemgather/internal/config"
	"com-itemgather/internal/database"
	"com-itemgather/internal/security"
)

type testEnvironment struct {
	app      *App
	db       *sql.DB
	server   *httptest.Server
	aiServer *httptest.Server
	cancel   context.CancelFunc
}
type testClient struct {
	http *http.Client
	base string
	csrf string
	user User
}

func validAIHandler(w http.ResponseWriter, r *http.Request) {
	result := AIResult{DisplayTitle: "绿色风暴", Summary: "青苹果饮料", NameCandidates: []string{"绿色风暴"}, IdeaTypes: []string{"名称", "玩法"}, ProductDirections: []string{"饮料"}, Tags: []string{"青苹果"}, Attributes: []Attribute{{Key: "口味", Value: "青苹果", Certainty: "tentative"}}}
	content := encodeJSON(result)
	writeJSON(w, 200, map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": content}, "finish_reason": "stop"}}})
}
func newTestEnvironment(t *testing.T, handler http.HandlerFunc) *testEnvironment {
	t.Helper()
	if handler == nil {
		handler = validAIHandler
	}
	aiServer := httptest.NewServer(handler)
	dir := t.TempDir()
	db, err := database.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err = database.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{AppEnv: "test", Addr: "127.0.0.1:0", DatabasePath: filepath.Join(dir, "test.db"), AttachmentDir: filepath.Join(dir, "attachments"), WebDist: dir, AIBaseURL: aiServer.URL, AIAPIKey: "test-key", AIModel: "test-model", AITimeout: 300 * time.Millisecond, AIMaxRetries: 3, AIWorkers: 1, SessionTTL: 7 * 24 * time.Hour, RememberTTL: 90 * 24 * time.Hour, MaxUploadBytes: 1024 * 1024, MaxAttachments: 5, MaxStorageBytes: 10 * 1024 * 1024, BackupDir: filepath.Join(dir, "backups"), BackupInterval: 24 * time.Hour, BackupKeep: 3, BackupRetentionDays: 30, SecureCookie: false}
	application := New(db, cfg, log.New(io.Discard, "", 0))
	application.apiLimiter = newRequestLimiter(100000)
	server := httptest.NewServer(application)
	application.cfg.PublicURL = server.URL
	ctx, cancel := context.WithCancel(context.Background())
	application.Start(ctx)
	environment := &testEnvironment{app: application, db: db, server: server, aiServer: aiServer, cancel: cancel}
	t.Cleanup(func() { cancel(); server.Close(); aiServer.Close(); db.Close() })
	return environment
}
func newTestClient(t *testing.T, server *httptest.Server) *testClient {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &testClient{http: &http.Client{Jar: jar, Timeout: 10 * time.Second}, base: server.URL}
}
func (c *testClient) request(t *testing.T, method, path string, body any) (int, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		switch value := body.(type) {
		case []byte:
			reader = bytes.NewReader(value)
		case string:
			reader = strings.NewReader(value)
		default:
			data, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			reader = bytes.NewReader(data)
		}
	}
	req, err := http.NewRequest(method, c.base+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.csrf != "" {
		req.Header.Set("X-CSRF-Token", c.csrf)
	}
	response, err := c.http.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, data
}
func decodeTestJSON[T any](t *testing.T, data []byte) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatalf("decode %s: %v", data, err)
	}
	return value
}
func (c *testClient) login(t *testing.T, username, password string) {
	t.Helper()
	status, data := c.request(t, "POST", "/api/auth/login", map[string]any{"username": username, "password": password, "remember": true, "device_name": "测试浏览器"})
	if status != 200 {
		t.Fatalf("login status %d: %s", status, data)
	}
	result := decodeTestJSON[map[string]any](t, data)
	c.csrf = result["csrf_token"].(string)
	raw, _ := json.Marshal(result["user"])
	json.Unmarshal(raw, &c.user)
}
func createTestUser(t *testing.T, db *sql.DB, username, role string) User {
	t.Helper()
	hash, err := security.HashPassword("correct-horse-42")
	if err != nil {
		t.Fatal(err)
	}
	user := User{ID: security.ID(), Username: username, Nickname: username, Role: role, Status: "active", CreatedAt: now()}
	_, err = db.Exec("INSERT INTO users(id,username,nickname,password_hash,role,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)", user.ID, user.Username, user.Nickname, hash, user.Role, user.Status, user.CreatedAt, user.CreatedAt)
	if err != nil {
		t.Fatal(err)
	}
	return user
}
func newAdminClient(t *testing.T, environment *testEnvironment) *testClient {
	t.Helper()
	createTestUser(t, environment.db, "admin", "admin")
	client := newTestClient(t, environment.server)
	client.login(t, "admin", "correct-horse-42")
	return client
}
func (c *testClient) refreshCSRF(t *testing.T) {
	t.Helper()
	status, data := c.request(t, "GET", "/api/auth/me", nil)
	if status != 200 {
		t.Fatalf("me: %d %s", status, data)
	}
	c.csrf = decodeTestJSON[map[string]any](t, data)["csrf_token"].(string)
}
func createTestIdea(t *testing.T, client *testClient, clientID, content string) Idea {
	t.Helper()
	status, data := client.request(t, "POST", "/api/ideas", map[string]string{"client_id": clientID, "content": content})
	if status != 201 && status != 200 {
		t.Fatalf("create idea: %d %s", status, data)
	}
	return decodeTestJSON[Idea](t, data)
}
func waitAIStatus(t *testing.T, client *testClient, id, want string) Idea {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	var item Idea
	for time.Now().Before(deadline) {
		status, data := client.request(t, "GET", "/api/ideas/"+id, nil)
		if status != 200 {
			t.Fatalf("get idea: %d %s", status, data)
		}
		item = decodeTestJSON[Idea](t, data)
		if item.AIStatus == want {
			return item
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("AI status did not become %s; last %+v", want, item)
	return item
}
func cookieForURL(client *http.Client, rawURL string) []*http.Cookie {
	parsed, _ := url.Parse(rawURL)
	return client.Jar.Cookies(parsed)
}
func testError(t *testing.T, data []byte) string {
	return decodeTestJSON[map[string]any](t, data)["error"].(map[string]any)["code"].(string)
}
func (c *testClient) multipart(t *testing.T, path, ideaID string, data []byte, filename string, clientID ...string) (int, []byte) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.WriteField("idea_id", ideaID)
	if len(clientID) > 0 {
		writer.WriteField("client_id", clientID[0])
	}
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	part.Write(data)
	writer.Close()
	req, _ := http.NewRequest("POST", c.base+path, body)
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
func assertStatus(t *testing.T, got, want int, data []byte) {
	t.Helper()
	if got != want {
		t.Fatalf("status=%d want=%d body=%s", got, want, data)
	}
}
func debugName(value any) string { return fmt.Sprint(value) }
