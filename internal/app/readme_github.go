package app

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"com-itemgather/internal/config"
)

const maxReadmeBytes = 1024 * 1024

type readmeRemote struct {
	Content string `json:"content"`
	SHA     string `json:"sha"`
}

type readmeAuthor struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type readmeGitHub struct {
	cfg        config.Config
	http       *http.Client
	baseURL    string
	tokenMu    sync.Mutex
	token      string
	tokenUntil time.Time
}

func newReadmeGitHub(cfg config.Config) *readmeGitHub {
	return &readmeGitHub{cfg: cfg, baseURL: "https://api.github.com", http: &http.Client{
		Timeout: 25 * time.Second,
		// Never forward a credential to a redirect target.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

func (g *readmeGitHub) configured() bool {
	return g.cfg.ReadmeRepository != "" && g.cfg.ReadmeAppID != "" && g.cfg.ReadmeInstallationID != "" && g.cfg.ReadmePrivateKeyFile != ""
}

func (g *readmeGitHub) request(ctx context.Context, method, path, token string, body any) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, g.baseURL+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	req.Header.Set("User-Agent", "COM-ItemGather-Readme")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := g.http.Do(req)
	if err != nil {
		return nil, fail(502, "github_unavailable", "暂时无法连接 GitHub，草稿已保留，请稍后重试")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 2*maxReadmeBytes+65537))
	if err != nil || len(data) > 2*maxReadmeBytes+65536 {
		return nil, fail(502, "github_response", "GitHub 返回内容过大或不完整")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		switch response.StatusCode {
		case 409:
			return nil, fail(409, "readme_remote_conflict", "GitHub 上的 README 已变化，请比较并合并后重试")
		case 401:
			return nil, fail(502, "github_auth", "GitHub 凭据无效，请管理员检查 App 安装和私钥配置")
		case 403, 422:
			return nil, fail(502, "github_rejected", "GitHub 拒绝了请求，请检查 App 的 Contents 写权限、请求限额及目标分支的保护规则；草稿已保留")
		case 404:
			return nil, fail(502, "github_not_found", "无法访问目标仓库、分支或 README.md，请检查 App 安装范围和仓库配置")
		default:
			return nil, fail(502, "github_unavailable", "GitHub 暂时未能完成请求，草稿已保留，请稍后重试")
		}
	}
	return data, nil
}

func (g *readmeGitHub) installationToken(ctx context.Context) (string, error) {
	if !g.configured() {
		return "", fail(503, "github_not_configured", "尚未连接游戏仓库，请管理员配置 GitHub App；仍可编辑和保存草稿")
	}
	g.tokenMu.Lock()
	defer g.tokenMu.Unlock()
	if g.token != "" && time.Until(g.tokenUntil) > 2*time.Minute {
		return g.token, nil
	}
	keyPEM, err := os.ReadFile(g.cfg.ReadmePrivateKeyFile)
	if err != nil || len(keyPEM) > 65536 {
		return "", fail(503, "github_private_key", "无法读取 GitHub App 私钥，请管理员检查私钥文件及服务账号的读取权限")
	}
	appJWT, err := signReadmeJWT(keyPEM, g.cfg.ReadmeAppID, time.Now())
	if err != nil {
		return "", fail(503, "github_private_key", "GitHub App 私钥格式无效，需要 RSA PEM 私钥")
	}
	parts := strings.Split(g.cfg.ReadmeRepository, "/")
	if len(parts) != 2 {
		return "", fail(503, "github_repository", "游戏仓库配置必须为 owner/repo")
	}
	data, err := g.request(ctx, http.MethodPost, "/app/installations/"+url.PathEscape(g.cfg.ReadmeInstallationID)+"/access_tokens", appJWT, map[string]any{
		"repositories": []string{parts[1]}, "permissions": map[string]string{"contents": "write"},
	})
	if err != nil {
		return "", err
	}
	var result struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if json.Unmarshal(data, &result) != nil || result.Token == "" || !result.ExpiresAt.After(time.Now()) {
		return "", fail(502, "github_response", "GitHub 未返回有效的安装令牌")
	}
	g.token, g.tokenUntil = result.Token, result.ExpiresAt
	return g.token, nil
}

func signReadmeJWT(keyPEM []byte, appID string, timestamp time.Time) (string, error) {
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return "", fmt.Errorf("invalid PEM")
	}
	var key *rsa.PrivateKey
	if parsed, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		key = parsed
	} else if parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		key, _ = parsed.(*rsa.PrivateKey)
	}
	if key == nil {
		return "", fmt.Errorf("not an RSA private key")
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims, err := json.Marshal(map[string]any{"iat": timestamp.Add(-time.Minute).Unix(), "exp": timestamp.Add(9 * time.Minute).Unix(), "iss": appID})
	if err != nil {
		return "", err
	}
	unsigned := header + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (g *readmeGitHub) contentsPath() string {
	parts := strings.Split(g.cfg.ReadmeRepository, "/")
	return "/repos/" + url.PathEscape(parts[0]) + "/" + url.PathEscape(parts[1]) + "/contents/README.md"
}

func (g *readmeGitHub) forgetRejectedToken(token string, err error) {
	if failure, ok := err.(*APIError); ok && failure.Code == "github_auth" {
		g.tokenMu.Lock()
		defer g.tokenMu.Unlock()
		if g.token == token {
			g.token = ""
			g.tokenUntil = time.Time{}
		}
	}
}

func (g *readmeGitHub) read(ctx context.Context) (readmeRemote, error) {
	token, err := g.installationToken(ctx)
	if err != nil {
		return readmeRemote{}, err
	}
	query := url.Values{"ref": {g.cfg.ReadmeBranch}}
	data, err := g.request(ctx, http.MethodGet, g.contentsPath()+"?"+query.Encode(), token, nil)
	if err != nil {
		g.forgetRejectedToken(token, err)
		return readmeRemote{}, err
	}
	var file struct {
		Type     string `json:"type"`
		Path     string `json:"path"`
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
		SHA      string `json:"sha"`
	}
	if json.Unmarshal(data, &file) != nil || file.Type != "file" || file.Path != "README.md" || file.Encoding != "base64" || file.SHA == "" {
		return readmeRemote{}, fail(502, "github_response", "GitHub 未返回可编辑的 README.md 文件")
	}
	content, err := base64.StdEncoding.DecodeString(file.Content)
	if err != nil || len(content) > maxReadmeBytes || !utf8.Valid(content) {
		return readmeRemote{}, fail(502, "github_response", "README 必须为不超过 1 MiB 的 UTF-8 文本")
	}
	return readmeRemote{Content: string(content), SHA: file.SHA}, nil
}

func (g *readmeGitHub) push(ctx context.Context, draft readmeDraft, author readmeAuthor, message string) (readmeRemote, string, error) {
	token, err := g.installationToken(ctx)
	if err != nil {
		return readmeRemote{}, "", err
	}
	data, err := g.request(ctx, http.MethodPut, g.contentsPath(), token, map[string]any{
		"content": base64.StdEncoding.EncodeToString([]byte(draft.Content)), "sha": draft.BaseSHA,
		"branch": g.cfg.ReadmeBranch, "message": message, "author": author,
	})
	if err != nil {
		g.forgetRejectedToken(token, err)
		return readmeRemote{}, "", err
	}
	var result struct {
		Content struct {
			SHA string `json:"sha"`
		} `json:"content"`
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	if json.Unmarshal(data, &result) != nil || result.Content.SHA == "" || result.Commit.SHA == "" {
		return readmeRemote{}, "", fail(502, "github_response", "GitHub 返回了不完整的提交结果，请先核对远端再重试")
	}
	return readmeRemote{Content: draft.Content, SHA: result.Content.SHA}, "https://github.com/" + g.cfg.ReadmeRepository + "/commit/" + url.PathEscape(result.Commit.SHA), nil
}
