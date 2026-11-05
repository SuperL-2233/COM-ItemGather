package app

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type readmeTestState struct {
	Draft            readmeDraft          `json:"draft"`
	Authors          []readmeCommitAuthor `json:"authors"`
	SelectedAuthorID string               `json:"selected_author_id"`
	GitHub           struct {
		Configured bool `json:"configured"`
	} `json:"github"`
}

type readmeGitHubMock struct {
	mu          sync.Mutex
	content     string
	sha         string
	tokens      int
	pushes      int
	reject      bool
	lastAuthor  readmeAuthor
	lastMessage string
}

func readmeBlobSHA(content string) string {
	digest := sha1.Sum([]byte(fmt.Sprintf("blob %d\x00%s", len(content), content)))
	return hex.EncodeToString(digest[:])
}

func newReadmeMock(t *testing.T, environment *testEnvironment) *readmeGitHubMock {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	keyFile := filepath.Join(t.TempDir(), "app.pem")
	if err = os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0600); err != nil {
		t.Fatal(err)
	}
	mock := &readmeGitHubMock{content: "# 游戏说明\n\n## 世界观\n\n原始内容。\n"}
	mock.sha = readmeBlobSHA(mock.content)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mock.mu.Lock()
		defer mock.mu.Unlock()
		if r.URL.Path == "/app/installations/456/access_tokens" && r.Method == http.MethodPost {
			parts := strings.Split(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), ".")
			if len(parts) != 3 {
				t.Error("missing app JWT")
				w.WriteHeader(401)
				return
			}
			digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
			signature, _ := base64.RawURLEncoding.DecodeString(parts[2])
			if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, digest[:], signature); err != nil {
				t.Error("invalid JWT signature")
				w.WriteHeader(401)
				return
			}
			claimsData, _ := base64.RawURLEncoding.DecodeString(parts[1])
			claims := decodeTestJSON[map[string]any](t, claimsData)
			if claims["iss"] != "123" || claims["exp"].(float64) > float64(time.Now().Add(10*time.Minute).Unix()) {
				t.Error("unexpected JWT claims")
			}
			var scope struct {
				Repositories []string          `json:"repositories"`
				Permissions  map[string]string `json:"permissions"`
			}
			if json.NewDecoder(r.Body).Decode(&scope) != nil || len(scope.Repositories) != 1 || scope.Repositories[0] != "CallOfMommy" || len(scope.Permissions) != 1 || scope.Permissions["contents"] != "write" {
				t.Error("installation token was not restricted to the configured repository and contents permission")
			}
			mock.tokens++
			writeJSON(w, 201, map[string]any{"token": "mock-installation-token", "expires_at": time.Now().Add(time.Hour).Format(time.RFC3339)})
			return
		}
		if r.URL.Path != "/repos/SuperL-2233/CallOfMommy/contents/README.md" || r.Header.Get("Authorization") != "Bearer mock-installation-token" {
			t.Errorf("unexpected GitHub access: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
			return
		}
		if r.Method == http.MethodGet {
			if r.URL.Query().Get("ref") != "main" {
				t.Error("wrong read branch")
			}
			writeJSON(w, 200, map[string]any{"type": "file", "path": "README.md", "encoding": "base64", "content": base64.StdEncoding.EncodeToString([]byte(mock.content)), "sha": mock.sha})
			return
		}
		if r.Method == http.MethodPut {
			if mock.reject {
				writeJSON(w, 403, map[string]string{"message": "protected branch; secret diagnostic must not leak"})
				return
			}
			var body struct {
				Content string       `json:"content"`
				SHA     string       `json:"sha"`
				Branch  string       `json:"branch"`
				Author  readmeAuthor `json:"author"`
				Message string       `json:"message"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
				w.WriteHeader(400)
				return
			}
			if body.SHA != mock.sha {
				w.WriteHeader(409)
				return
			}
			if body.Branch != "main" || body.Author.Email == "" {
				t.Error("wrong write branch or missing human author")
			}
			content, err := base64.StdEncoding.DecodeString(body.Content)
			if err != nil {
				t.Error(err)
				w.WriteHeader(400)
				return
			}
			mock.content, mock.sha = string(content), readmeBlobSHA(string(content))
			mock.pushes++
			mock.lastAuthor = body.Author
			mock.lastMessage = body.Message
			writeJSON(w, 200, map[string]any{"content": map[string]string{"sha": mock.sha}, "commit": map[string]string{"sha": strings.Repeat("a", 40)}})
			return
		}
		w.WriteHeader(405)
	}))
	t.Cleanup(server.Close)
	environment.app.cfg.ReadmeRepository = "SuperL-2233/CallOfMommy"
	environment.app.cfg.ReadmeBranch = "main"
	environment.app.cfg.ReadmeAppID = "123"
	environment.app.cfg.ReadmeInstallationID = "456"
	environment.app.cfg.ReadmePrivateKeyFile = keyFile
	environment.app.readmeGitHub = newReadmeGitHub(environment.app.cfg)
	environment.app.readmeGitHub.baseURL = server.URL
	return mock
}

func readReadme(t *testing.T, client *testClient) readmeTestState {
	t.Helper()
	status, data := client.request(t, "GET", "/api/readme", nil)
	assertStatus(t, status, 200, data)
	return decodeTestJSON[readmeTestState](t, data)
}

func saveTestReadme(t *testing.T, client *testClient, version int, content string) readmeDraft {
	t.Helper()
	status, data := client.request(t, "PUT", "/api/readme", map[string]any{"version": version, "content": content})
	assertStatus(t, status, 200, data)
	return decodeTestJSON[readmeDraft](t, data)
}

func createTestReadmeAuthor(t *testing.T, client *testClient, name, email string) readmeCommitAuthor {
	t.Helper()
	status, data := client.request(t, "POST", "/api/readme/authors", map[string]string{"name": name, "email": email})
	assertStatus(t, status, 201, data)
	return decodeTestJSON[readmeCommitAuthor](t, data)
}

func selectTestReadmeAuthor(t *testing.T, client *testClient, author readmeCommitAuthor) {
	t.Helper()
	status, data := client.request(t, "PUT", "/api/readme/author", map[string]any{"author_id": author.ID, "author_version": author.Version})
	assertStatus(t, status, 200, data)
}

func TestReadmeDraftIsolationVersioningAndAuthentication(t *testing.T) {
	environment := newTestEnvironment(t, nil)
	admin := newAdminClient(t, environment)
	createTestUser(t, environment.db, "reader", "member")
	member := newTestClient(t, environment.server)
	member.login(t, "reader", "correct-horse-42")
	anonymous := newTestClient(t, environment.server)
	for _, path := range []string{"/api/readme", "/api/readme/remote", "/api/readme/authors"} {
		status, data := anonymous.request(t, "GET", path, nil)
		assertStatus(t, status, 401, data)
	}
	initial := readReadme(t, admin)
	if initial.GitHub.Configured || initial.Draft.Content != "" {
		t.Fatal("unconfigured README should still allow an empty local draft")
	}
	saved := saveTestReadme(t, admin, initial.Draft.Version, "# 个人草稿\n")
	if saved.Version != initial.Draft.Version+1 || readReadme(t, member).Draft.Content != "" {
		t.Fatal("drafts are not isolated or versioned")
	}
	status, data := admin.request(t, "PUT", "/api/readme", map[string]any{"version": initial.Draft.Version, "content": "stale overwrite"})
	assertStatus(t, status, 409, data)
	if testError(t, data) != "readme_draft_conflict" || readReadme(t, admin).Draft.Content != saved.Content {
		t.Fatal("stale draft overwrote saved content")
	}
	status, data = admin.request(t, "PUT", "/api/readme", map[string]any{"version": saved.Version, "content": "bad", "path": "game.cs"})
	assertStatus(t, status, 400, data)
	admin.csrf = "wrong"
	status, data = admin.request(t, "PUT", "/api/readme", map[string]any{"version": saved.Version, "content": "csrf"})
	assertStatus(t, status, 403, data)
}

func TestReadmeAppPushUsesHumanAuthorAndPreservesConflicts(t *testing.T) {
	environment := newTestEnvironment(t, nil)
	mock := newReadmeMock(t, environment)
	admin := newAdminClient(t, environment)
	createTestUser(t, environment.db, "writer", "member")
	member := newTestClient(t, environment.server)
	member.login(t, "writer", "correct-horse-42")
	first := readReadme(t, admin)
	second := readReadme(t, member)
	if first.Draft.BaseSHA == "" || first.Draft.Content != second.Draft.Content || !first.GitHub.Configured {
		t.Fatal("README was not loaded from GitHub")
	}
	mainAuthor := createTestReadmeAuthor(t, admin, "主创", "123+creator@users.noreply.github.com")
	memberAuthor := createTestReadmeAuthor(t, member, "成员", "456+writer@users.noreply.github.com")
	selectTestReadmeAuthor(t, admin, memberAuthor)
	selectTestReadmeAuthor(t, member, mainAuthor)
	draft := saveTestReadme(t, admin, first.Draft.Version, first.Draft.Content+"\n主创修改。\n")
	status, data := admin.request(t, "POST", "/api/readme/push", map[string]any{"version": draft.Version, "message": "docs: 主创更新", "author_id": mainAuthor.ID, "author_version": mainAuthor.Version})
	assertStatus(t, status, 200, data)
	pushed := decodeTestJSON[readmeDraft](t, data)
	if pushed.BaseContent != draft.Content || pushed.BaseSHA == first.Draft.BaseSHA || !strings.HasPrefix(pushed.LastCommitURL, "https://github.com/SuperL-2233/CallOfMommy/commit/") {
		t.Fatal("push did not advance the base or retain its commit link")
	}
	mock.mu.Lock()
	if mock.pushes != 1 || mock.tokens != 1 || mock.lastAuthor.Email != "123+creator@users.noreply.github.com" || mock.lastMessage != "docs: 主创更新" {
		t.Error("wrong push identity or installation token was not cached")
	}
	mock.mu.Unlock()
	var actor string
	if err := environment.db.QueryRow("SELECT actor_id FROM audit_logs WHERE entity_type='readme' AND action='push'").Scan(&actor); err != nil || actor != admin.user.ID {
		t.Fatal("human actor was not audited")
	}
	memberDraft := saveTestReadme(t, member, second.Draft.Version, second.Draft.Content+"\n成员修改。\n")
	status, data = member.request(t, "POST", "/api/readme/push", map[string]any{"version": memberDraft.Version, "message": "docs: 成员更新", "author_id": memberAuthor.ID, "author_version": memberAuthor.Version})
	assertStatus(t, status, 409, data)
	if testError(t, data) != "readme_remote_conflict" || readReadme(t, member).Draft.Content != memberDraft.Content {
		t.Fatal("remote conflict did not preserve personal draft")
	}
	status, data = member.request(t, "POST", "/api/readme/refresh", map[string]any{"version": memberDraft.Version})
	assertStatus(t, status, 409, data)
	merged := pushed.Content + "\n成员修改。\n"
	status, data = member.request(t, "POST", "/api/readme/rebase", map[string]any{"version": memberDraft.Version, "remote_sha": second.Draft.BaseSHA, "content": merged})
	assertStatus(t, status, 409, data)
	status, data = member.request(t, "POST", "/api/readme/rebase", map[string]any{"version": memberDraft.Version, "remote_sha": pushed.BaseSHA, "content": merged})
	assertStatus(t, status, 200, data)
	memberDraft = decodeTestJSON[readmeDraft](t, data)
	status, data = member.request(t, "POST", "/api/readme/push", map[string]any{"version": memberDraft.Version, "message": "docs: 合并成员修改", "author_id": memberAuthor.ID, "author_version": memberAuthor.Version})
	assertStatus(t, status, 200, data)
	mock.mu.Lock()
	if mock.content != merged || mock.pushes != 2 || mock.lastAuthor.Email != "456+writer@users.noreply.github.com" {
		t.Error("merge was not published with the member's identity")
	}
	mock.mu.Unlock()
}

func TestReadmeRejectsInvalidAuthorsAndRetainsRejectedPush(t *testing.T) {
	environment := newTestEnvironment(t, nil)
	mock := newReadmeMock(t, environment)
	admin := newAdminClient(t, environment)
	initial := readReadme(t, admin)
	for _, email := range []string{"nickname", "Name <name@example.com>", "name@example.com\nInjected: bad"} {
		status, data := admin.request(t, "POST", "/api/readme/authors", map[string]string{"name": "成员", "email": email})
		assertStatus(t, status, 422, data)
	}
	draft := saveTestReadme(t, admin, initial.Draft.Version, "# 更新内容\n")
	status, data := admin.request(t, "POST", "/api/readme/push", map[string]any{"version": draft.Version, "message": "update"})
	assertStatus(t, status, 422, data)
	if testError(t, data) != "readme_author_required" {
		t.Fatal("push should require an explicit human author")
	}
	author := createTestReadmeAuthor(t, admin, "成员", "author@example.com")
	mock.mu.Lock()
	mock.reject = true
	mock.mu.Unlock()
	status, data = admin.request(t, "POST", "/api/readme/push", map[string]any{"version": draft.Version, "message": "update", "author_id": author.ID, "author_version": author.Version})
	assertStatus(t, status, 502, data)
	if strings.Contains(string(data), "secret diagnostic") || strings.Contains(string(data), "mock-installation-token") {
		t.Fatal("upstream diagnostics or credentials leaked")
	}
	retained := readReadme(t, admin).Draft
	if retained.Content != draft.Content || retained.BaseSHA != initial.Draft.BaseSHA {
		t.Fatal("failed push changed or discarded draft")
	}
}

func TestReadmePushPreservesExactMarkdownSource(t *testing.T) {
	environment := newTestEnvironment(t, nil)
	mock := newReadmeMock(t, environment)
	original := "\ufeff# 标题\r\n\r\n\r\n第一段  \r\n\r\n\n| 名称 |价格|\r\n|:-----|---:|\r\n|苹果|  10 |\r\n\r\n只改这里\r\n\r\n"
	mock.mu.Lock()
	mock.content, mock.sha = original, readmeBlobSHA(original)
	mock.mu.Unlock()
	admin := newAdminClient(t, environment)
	initial := readReadme(t, admin)
	if initial.Draft.Content != original {
		t.Fatal("reading README changed its original bytes")
	}
	author := createTestReadmeAuthor(t, admin, "保真作者", "source@example.com")
	changed := strings.Replace(original, "只改这里", "只改这一处", 1)
	draft := saveTestReadme(t, admin, initial.Draft.Version, changed)
	status, data := admin.request(t, "POST", "/api/readme/push", map[string]any{"version": draft.Version, "message": "docs: 局部修改", "author_id": author.ID, "author_version": author.Version})
	assertStatus(t, status, 200, data)
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.content != changed {
		t.Fatal("GitHub received reformatted content instead of the exact saved Markdown")
	}
}
