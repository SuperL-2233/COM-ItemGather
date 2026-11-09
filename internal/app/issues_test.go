package app

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type issueMock struct {
	mu                                                           sync.Mutex
	issue                                                        githubIssue
	issueTokens, contentTokens, creates, commentCreates, updates int
	lastBody, lastTitle, lastReason                              string
	rejectIssues                                                 bool
	status                                                       int
}

func newIssueMock(t *testing.T, environment *testEnvironment) *issueMock {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	keyFile := filepath.Join(t.TempDir(), "issues.pem")
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0600); err != nil {
		t.Fatal(err)
	}
	mock := &issueMock{issue: githubIssue{Number: 7, Title: "测试问题", Body: "# 问题说明\n\n正文。\n", State: "open", User: issueUser{Login: "original-user"}, Comments: 31, CreatedAt: now(), UpdatedAt: now()}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mock.mu.Lock()
		defer mock.mu.Unlock()
		if r.URL.Path == "/app/installations/456/access_tokens" {
			var body struct {
				Repositories []string          `json:"repositories"`
				Permissions  map[string]string `json:"permissions"`
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil || len(body.Repositories) != 1 || body.Repositories[0] != "CallOfMommy" || len(body.Permissions) != 1 || len(strings.Split(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), ".")) != 3 {
				t.Error("token scope or JWT invalid")
				w.WriteHeader(400)
				return
			}
			permission := "issues"
			if body.Permissions["contents"] == "write" {
				permission = "contents"
				mock.contentTokens++
			} else if body.Permissions["issues"] == "write" {
				mock.issueTokens++
			} else {
				t.Error("unexpected permission")
				w.WriteHeader(400)
				return
			}
			if permission == "issues" && mock.rejectIssues {
				writeJSON(w, 422, map[string]string{"message": "secret upstream installation diagnostic"})
				return
			}
			writeJSON(w, 201, map[string]any{"token": "mock-" + permission, "expires_at": time.Now().Add(time.Hour).Format(time.RFC3339)})
			return
		}
		if r.URL.Path == "/repos/SuperL-2233/CallOfMommy/contents/README.md" {
			if r.Header.Get("Authorization") != "Bearer mock-contents" {
				t.Error("README used issue token")
			}
			writeJSON(w, 200, map[string]string{"type": "file", "path": "README.md", "encoding": "base64", "content": base64.StdEncoding.EncodeToString([]byte("# README\n")), "sha": "readme-sha"})
			return
		}
		if r.Header.Get("Authorization") != "Bearer mock-issues" || !strings.HasPrefix(r.URL.Path, "/repos/SuperL-2233/CallOfMommy/issues") {
			t.Errorf("unexpected GitHub access: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
			return
		}
		if mock.status != 0 {
			writeJSON(w, mock.status, map[string]string{"message": "secret upstream failure"})
			return
		}
		switch r.Method + " " + strings.TrimPrefix(r.URL.Path, "/repos/SuperL-2233/CallOfMommy") {
		case "GET /issues":
			if r.URL.Query().Get("per_page") != "30" || r.URL.Query().Get("sort") != "updated" || r.URL.Query().Get("direction") != "desc" {
				t.Error("missing list pagination or sorting")
			}
			if r.URL.Query().Get("page") == "2" {
				writeJSON(w, 200, []githubIssue{})
				return
			}
			issues := []githubIssue{mock.issue}
			for index := 1; index < issuePageSize; index++ {
				issues = append(issues, githubIssue{Number: 100 + index, PullRequest: json.RawMessage(`{"url":"https://api.github.com/pulls/1"}`)})
			}
			writeJSON(w, 200, issues)
		case "GET /issues/7":
			writeJSON(w, 200, mock.issue)
		case "GET /issues/8":
			writeJSON(w, 200, githubIssue{Number: 8, State: "open", PullRequest: json.RawMessage(`{"url":"https://api.github.com/pulls/8"}`)})
		case "POST /issues":
			var body struct {
				Title string `json:"title"`
				Body  string `json:"body"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			mock.creates++
			mock.lastTitle, mock.lastBody = body.Title, body.Body
			issue := mock.issue
			issue.Number = 9
			issue.Title = body.Title
			issue.Body = body.Body
			issue.User.Login = "readme-app[bot]"
			writeJSON(w, 201, issue)
		case "PATCH /issues/7":
			var body struct {
				State       string `json:"state"`
				StateReason string `json:"state_reason"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			mock.updates++
			mock.lastReason = body.StateReason
			mock.issue.State = body.State
			mock.issue.StateReason = body.StateReason
			writeJSON(w, 200, mock.issue)
		case "GET /issues/7/comments":
			count := issuePageSize
			start := int64(1)
			if r.URL.Query().Get("page") == "2" {
				count = 1
				start = 31
			}
			comments := []githubIssueComment{}
			for index := 0; index < count; index++ {
				comments = append(comments, githubIssueComment{ID: start + int64(index), Body: "评论", User: issueUser{Login: "commenter"}, CreatedAt: now(), UpdatedAt: now()})
			}
			writeJSON(w, 200, comments)
		case "POST /issues/7/comments":
			var body struct {
				Body string `json:"body"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			mock.commentCreates++
			mock.lastBody = body.Body
			writeJSON(w, 201, githubIssueComment{ID: 32, Body: body.Body, User: issueUser{Login: "readme-app[bot]"}, CreatedAt: now(), UpdatedAt: now()})
		default:
			t.Errorf("unexpected issue mutation: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(server.Close)
	cfg := environment.app.cfg
	cfg.ReadmeRepository = "SuperL-2233/CallOfMommy"
	cfg.ReadmeBranch = "main"
	cfg.ReadmeAppID = "123"
	cfg.ReadmeInstallationID = "456"
	cfg.ReadmePrivateKeyFile = keyFile
	environment.app.cfg = cfg
	environment.app.readmeGitHub = newReadmeGitHub(cfg)
	environment.app.readmeGitHub.baseURL = server.URL
	return mock
}

func TestIssuesListPaginationExcludesPullRequestsAndRequiresLogin(t *testing.T) {
	environment := newTestEnvironment(t, nil)
	mock := newIssueMock(t, environment)
	anonymous := newTestClient(t, environment.server)
	status, data := anonymous.request(t, "GET", "/api/issues", nil)
	assertStatus(t, status, 401, data)
	admin := newAdminClient(t, environment)
	status, data = admin.request(t, "GET", "/api/issues?state=all", nil)
	assertStatus(t, status, 200, data)
	result := decodeTestJSON[struct {
		Items      []githubIssue `json:"items"`
		NextPage   int           `json:"next_page"`
		Configured bool          `json:"configured"`
	}](t, data)
	if !result.Configured || len(result.Items) != 1 || result.Items[0].Number != 7 || result.NextPage != 2 {
		t.Fatalf("incorrect issue filtering/pagination: %s", data)
	}
	if result.Items[0].HTMLURL != "https://github.com/SuperL-2233/CallOfMommy/issues/7" || result.Items[0].Labels == nil {
		t.Fatal("missing safe issue URL or normalized arrays")
	}
	status, data = admin.request(t, "GET", "/api/issues?page=2", nil)
	assertStatus(t, status, 200, data)
	if decodeTestJSON[map[string]any](t, data)["next_page"] != float64(0) {
		t.Fatal("pagination did not finish")
	}
	for _, path := range []string{"/api/issues?page=0", "/api/issues?page=nope", "/api/issues?state=unexpected", "/api/issues/0", "/api/issues/abc"} {
		status, data = admin.request(t, "GET", path, nil)
		assertStatus(t, status, 422, data)
	}
	status, data = admin.request(t, "GET", "/api/issues/8", nil)
	assertStatus(t, status, 404, data)
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.issueTokens != 1 {
		t.Fatalf("token was not cached: %d", mock.issueTokens)
	}
}

func TestIssuesCreateCommentCloseReopenAndAuditRealWebsiteUser(t *testing.T) {
	environment := newTestEnvironment(t, nil)
	mock := newIssueMock(t, environment)
	admin := newAdminClient(t, environment)
	_, err := environment.db.Exec("UPDATE users SET nickname=? WHERE id=?", "张三 @someone <b>", admin.user.ID)
	if err != nil {
		t.Fatal(err)
	}
	original := "## 复现步骤\r\n\r\n\r\n| A |B|\r\n|---|---|\r\n|甲| 乙 |\r\n"
	status, data := admin.request(t, "POST", "/api/issues", map[string]any{"title": "  新问题  ", "body": original})
	assertStatus(t, status, 201, data)
	created := decodeTestJSON[githubIssue](t, data)
	if created.Number != 9 || created.Title != "新问题" || !strings.HasPrefix(created.Body, original) || !strings.Contains(created.Body, "张三 &#64;someone &#60;b&#62;") {
		t.Fatalf("source or website identity lost: %s", data)
	}
	status, data = admin.request(t, "POST", "/api/issues/7/comments", map[string]string{"body": original})
	assertStatus(t, status, 201, data)
	comment := decodeTestJSON[githubIssueComment](t, data)
	if !strings.HasPrefix(comment.Body, original) || comment.HTMLURL != "https://github.com/SuperL-2233/CallOfMommy/issues/7#issuecomment-32" {
		t.Fatal("comment content or link incorrect")
	}
	for _, state := range []string{"closed", "open"} {
		status, data = admin.request(t, "PATCH", "/api/issues/7", map[string]string{"state": state})
		assertStatus(t, status, 200, data)
		if decodeTestJSON[githubIssue](t, data).State != state {
			t.Fatalf("incorrect state: %s", data)
		}
	}
	status, data = admin.request(t, "PATCH", "/api/issues/7", map[string]string{"state": "open"})
	assertStatus(t, status, 200, data)
	for page := 1; page <= 2; page++ {
		status, data = admin.request(t, "GET", "/api/issues/7/comments?page="+strconv.Itoa(page), nil)
		assertStatus(t, status, 200, data)
		result := decodeTestJSON[struct {
			Items    []githubIssueComment `json:"items"`
			NextPage int                  `json:"next_page"`
		}](t, data)
		if page == 1 && (len(result.Items) != 30 || result.NextPage != 2) || page == 2 && (len(result.Items) != 1 || result.NextPage != 0) {
			t.Fatal("comments pagination incorrect")
		}
	}
	var auditCount int
	if err := environment.db.QueryRow("SELECT COUNT(*) FROM audit_logs WHERE actor_id=? AND entity_type='github_issue'", admin.user.ID).Scan(&auditCount); err != nil || auditCount != 4 {
		t.Fatalf("audit count %d: %v", auditCount, err)
	}
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.creates != 1 || mock.commentCreates != 1 || mock.updates != 2 || mock.lastReason != "reopened" {
		t.Fatal("unexpected issue mutations")
	}
}

func TestIssuesRejectInvalidWritesAndPullRequestMutation(t *testing.T) {
	environment := newTestEnvironment(t, nil)
	mock := newIssueMock(t, environment)
	admin := newAdminClient(t, environment)
	for _, entry := range []struct {
		method, path string
		body         any
		status       int
	}{
		{"POST", "/api/issues", map[string]string{"title": "   "}, 422},
		{"POST", "/api/issues", map[string]string{"title": "多行\n标题"}, 422},
		{"POST", "/api/issues", map[string]string{"title": "标题", "body": strings.Repeat("字", 60001)}, 422},
		{"POST", "/api/issues", map[string]string{"title": "标题", "author": "forged-user"}, 400},
		{"POST", "/api/issues/7/comments", map[string]string{"body": "  "}, 422},
		{"PATCH", "/api/issues/7", map[string]string{"state": "deleted"}, 422},
		{"PATCH", "/api/issues/8", map[string]string{"state": "closed"}, 404},
		{"POST", "/api/issues/8/comments", map[string]string{"body": "PR comment"}, 404},
	} {
		status, data := admin.request(t, entry.method, entry.path, entry.body)
		assertStatus(t, status, entry.status, data)
	}
	csrf := admin.csrf
	admin.csrf = ""
	status, data := admin.request(t, "POST", "/api/issues", map[string]string{"title": "forbidden"})
	assertStatus(t, status, 403, data)
	admin.csrf = csrf
	req, _ := http.NewRequest("POST", admin.base+"/api/issues", strings.NewReader(`{"title":"forbidden"}`))
	req.Header.Set("X-CSRF-Token", admin.csrf)
	req.Header.Set("X-User-ID", "another-user")
	response, err := admin.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 409 {
		t.Fatal("cross-account write was allowed")
	}
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.creates != 0 || mock.commentCreates != 0 || mock.updates != 0 {
		t.Fatal("invalid request mutated GitHub")
	}
}

func TestIssuesPermissionFailureDoesNotBreakReadmeAndErrorsDoNotLeak(t *testing.T) {
	environment := newTestEnvironment(t, nil)
	mock := newIssueMock(t, environment)
	admin := newAdminClient(t, environment)
	mock.mu.Lock()
	mock.rejectIssues = true
	mock.mu.Unlock()
	status, data := admin.request(t, "GET", "/api/issues", nil)
	assertStatus(t, status, 502, data)
	if !strings.Contains(string(data), "github_issues_permission") || strings.Contains(string(data), "secret upstream") {
		t.Fatalf("unsafe or unhelpful permission error: %s", data)
	}
	readme := readReadme(t, admin)
	if readme.Draft.Content != "# README\n" {
		t.Fatal("README broken by missing Issues permission")
	}
	mock.mu.Lock()
	mock.rejectIssues = false
	mock.mu.Unlock()
	status, data = admin.request(t, "GET", "/api/issues", nil)
	assertStatus(t, status, 200, data)
	for _, remoteStatus := range []int{401, 403, 404, 410, 429, 500} {
		mock.mu.Lock()
		mock.status = remoteStatus
		mock.mu.Unlock()
		status, data = admin.request(t, "GET", "/api/issues/7", nil)
		if status < 400 || strings.Contains(string(data), "secret upstream") || strings.Contains(string(data), "mock-issues") {
			t.Fatalf("upstream failure leaked or succeeded: %d %s", status, data)
		}
	}
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.contentTokens != 1 || mock.issueTokens != 3 {
		t.Fatalf("permission token caches not isolated: contents=%d issues=%d", mock.contentTokens, mock.issueTokens)
	}
}

func TestIssuesUnconfiguredInstallation(t *testing.T) {
	environment := newTestEnvironment(t, nil)
	admin := newAdminClient(t, environment)
	status, data := admin.request(t, "GET", "/api/issues", nil)
	assertStatus(t, status, 200, data)
	result := decodeTestJSON[map[string]any](t, data)
	if result["configured"] != false {
		t.Fatal("unconfigured App claimed ready")
	}
	status, data = admin.request(t, "POST", "/api/issues", map[string]string{"title": "标题", "body": "说明"})
	assertStatus(t, status, 503, data)
	status, data = admin.request(t, "POST", "/api/issues/7/comments", map[string]string{"body": "说明"})
	assertStatus(t, status, 503, data)
	status, data = admin.request(t, "GET", "/api/issues/7", nil)
	assertStatus(t, status, 503, data)
}

func TestIssuesPageCanContainSeveralFullMarkdownBodies(t *testing.T) {
	environment := newTestEnvironment(t, nil)
	mock := newIssueMock(t, environment)
	admin := newAdminClient(t, environment)
	// List responses carry entire bodies; a valid page can exceed the README
	// endpoint's single-file response cap.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer mock-issues" {
			t.Error("missing issue token")
		}
		items := []githubIssue{}
		for index := 0; index < issuePageSize; index++ {
			items = append(items, githubIssue{Number: index + 1, State: "open", Body: strings.Repeat("正文", 15000)})
		}
		writeJSON(w, 200, items)
	}))
	defer server.Close()
	status, data := admin.request(t, "GET", "/api/issues", nil)
	assertStatus(t, status, 200, data)
	client := environment.app.readmeGitHub
	client.baseURL = server.URL
	status, data = admin.request(t, "GET", "/api/issues", nil)
	assertStatus(t, status, 200, data)
	result := decodeTestJSON[struct {
		Items []githubIssue `json:"items"`
	}](t, data)
	if len(result.Items) != issuePageSize || result.Items[0].Body != strings.Repeat("正文", 15000) {
		t.Fatal("long Markdown issue bodies were truncated")
	}
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.issueTokens != 1 {
		t.Fatal("issue token was not reused")
	}
}
