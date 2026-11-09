package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const issuePageSize = 30

type issueUser struct {
	Login string `json:"login"`
}

type issueLabel struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

type githubIssue struct {
	Number      int             `json:"number"`
	Title       string          `json:"title"`
	Body        string          `json:"body"`
	State       string          `json:"state"`
	StateReason string          `json:"state_reason"`
	HTMLURL     string          `json:"html_url"`
	User        issueUser       `json:"user"`
	Labels      []issueLabel    `json:"labels"`
	Assignees   []issueUser     `json:"assignees"`
	Comments    int             `json:"comments"`
	Locked      bool            `json:"locked"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
	PullRequest json.RawMessage `json:"pull_request,omitempty"`
}

type githubIssueComment struct {
	ID        int64     `json:"id"`
	Body      string    `json:"body"`
	HTMLURL   string    `json:"html_url"`
	User      issueUser `json:"user"`
	CreatedAt string    `json:"created_at"`
	UpdatedAt string    `json:"updated_at"`
}

func (g *readmeGitHub) issuesPath() string {
	parts := strings.Split(g.cfg.ReadmeRepository, "/")
	if len(parts) != 2 {
		return ""
	}
	return "/repos/" + url.PathEscape(parts[0]) + "/" + url.PathEscape(parts[1]) + "/issues"
}

func (g *readmeGitHub) issueError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var failure *APIError
	if !errors.As(err, &failure) {
		return err
	}
	switch failure.Code {
	case "github_not_configured":
		return fail(503, "github_not_configured", "问题功能尚未连接 GitHub，请管理员完成现有 GitHub App 配置")
	case "github_rejected":
		return fail(502, "github_issues_permission", "GitHub 拒绝了请求：请确认 App 已获得 Issues 读写权限，并在安装设置中接受新增权限；也请检查问题是否锁定或请求是否过于频繁")
	case "github_not_found":
		return fail(404, "issue_not_found", "无法访问此问题，请检查问题编号、仓库范围和 App 的 Issues 权限")
	case "github_unavailable":
		return fail(502, "github_unavailable", "暂时无法连接 GitHub；若刚执行了发布操作，请刷新或在 GitHub 核对结果后再重试，避免重复发布")
	case "readme_remote_conflict":
		return fail(409, "issue_conflict", "GitHub 无法完成当前操作，请刷新问题后重试")
	case "github_gone":
		return fail(410, "github_issues_disabled", "仓库已关闭 Issues 功能，请仓库管理员在 GitHub 设置中启用")
	case "github_rate_limit":
		return fail(429, "github_rate_limit", "GitHub 请求过于频繁，请稍后再试")
	}
	return err
}

func (g *readmeGitHub) requestIssue(ctx context.Context, method, path string, body any, result any) error {
	token, err := g.installationTokenFor(ctx, "issues")
	if err != nil {
		return g.issueError(ctx, err)
	}
	// A page contains up to 30 complete Markdown bodies, rather than a single
	// README. Keep a bounded response without applying the single-file limit.
	data, err := g.requestWithLimit(ctx, method, path, token, body, 12*1024*1024)
	if err != nil {
		var failure *APIError
		if errors.As(err, &failure) && failure.Code == "github_auth" {
			g.tokenMu.Lock()
			if g.issueToken == token {
				g.issueToken = ""
				g.issueTokenUntil = time.Time{}
			}
			g.tokenMu.Unlock()
		}
		return g.issueError(ctx, err)
	}
	if json.Unmarshal(data, result) != nil {
		return fail(502, "github_response", "GitHub 返回了无法读取的问题数据，请刷新核对后再操作")
	}
	return nil
}

func (g *readmeGitHub) cleanIssue(issue *githubIssue) {
	issue.HTMLURL = "https://github.com/" + g.cfg.ReadmeRepository + "/issues/" + strconv.Itoa(issue.Number)
	if issue.Labels == nil {
		issue.Labels = []issueLabel{}
	}
	if issue.Assignees == nil {
		issue.Assignees = []issueUser{}
	}
}

func (g *readmeGitHub) getIssue(ctx context.Context, number int) (githubIssue, error) {
	var issue githubIssue
	err := g.requestIssue(ctx, http.MethodGet, g.issuesPath()+"/"+strconv.Itoa(number), nil, &issue)
	if err == nil && (issue.Number != number || (issue.State != "open" && issue.State != "closed")) {
		err = fail(502, "github_response", "GitHub 返回了不完整的问题数据")
	}
	if err == nil && len(issue.PullRequest) != 0 && string(issue.PullRequest) != "null" {
		err = fail(404, "issue_not_found", "该编号是 Pull Request，请在 GitHub 中查看")
	}
	g.cleanIssue(&issue)
	return issue, err
}
