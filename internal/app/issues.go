package app

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

func (a *App) registerIssueRoutes(handle func(string, http.HandlerFunc, bool)) {
	handle("GET /api/issues", a.listIssues, true)
	handle("POST /api/issues", a.createIssue, true)
	handle("GET /api/issues/{number}", a.getIssue, true)
	handle("PATCH /api/issues/{number}", a.changeIssueState, true)
	handle("GET /api/issues/{number}/comments", a.listIssueComments, true)
	handle("POST /api/issues/{number}/comments", a.createIssueComment, true)
}

func issuePage(r *http.Request) (int, error) {
	value := r.URL.Query().Get("page")
	if value == "" {
		return 1, nil
	}
	page, err := strconv.Atoi(value)
	if err != nil || page < 1 || page > 10000 {
		return 0, fail(422, "validation", "分页参数不正确")
	}
	return page, nil
}

func issueNumber(r *http.Request) (int, error) {
	number, err := strconv.Atoi(r.PathValue("number"))
	if err != nil || number < 1 || number > 2147483647 {
		return 0, fail(422, "validation", "问题编号不正确")
	}
	return number, nil
}

func (a *App) listIssues(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if state == "" {
		state = "open"
	}
	if state != "open" && state != "closed" && state != "all" {
		writeError(w, fail(422, "validation", "问题状态不正确"))
		return
	}
	page, err := issuePage(r)
	if err != nil {
		writeError(w, err)
		return
	}
	items := []githubIssue{}
	nextPage := 0
	if a.readmeGitHub.configured() {
		query := url.Values{"state": {state}, "page": {strconv.Itoa(page)}, "per_page": {strconv.Itoa(issuePageSize)}, "sort": {"updated"}, "direction": {"desc"}}
		var remote []githubIssue
		if err := a.readmeGitHub.requestIssue(r.Context(), http.MethodGet, a.readmeGitHub.issuesPath()+"?"+query.Encode(), nil, &remote); err != nil {
			writeError(w, err)
			return
		}
		if len(remote) == issuePageSize {
			nextPage = page + 1
		}
		for _, issue := range remote {
			if len(issue.PullRequest) > 0 && string(issue.PullRequest) != "null" {
				continue
			}
			if issue.Number < 1 || (issue.State != "open" && issue.State != "closed") {
				writeError(w, fail(502, "github_response", "GitHub 返回了不完整的问题列表，请刷新重试"))
				return
			}
			a.readmeGitHub.cleanIssue(&issue)
			items = append(items, issue)
		}
	}
	writeJSON(w, 200, map[string]any{"items": items, "next_page": nextPage, "configured": a.readmeGitHub.configured(), "repository": a.cfg.ReadmeRepository})
}

func validIssueText(value string, required bool, max int) bool {
	return (!required || strings.TrimSpace(value) != "") && utf8.ValidString(value) && utf8.RuneCountInString(value) <= max && !strings.ContainsRune(value, 0)
}

func issueSignedBody(body string, user User) string {
	name := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, user.Nickname)
	// Escape the website identity as plain Markdown, without treating it as a
	// GitHub login or allowing a nickname to inject a mention or markup.
	var safe strings.Builder
	for _, r := range name {
		if r > 127 || unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' {
			safe.WriteRune(r)
		} else {
			fmt.Fprintf(&safe, "&#%d;", r)
		}
	}
	return body + "\n\n---\n通过游戏设定库发布 · " + safe.String()
}

func (a *App) issueActivity(r *http.Request, action string, number int) {
	if _, err := a.db.ExecContext(r.Context(), "INSERT INTO audit_logs(actor_id,action,entity_type,entity_id,created_at) VALUES(?,?,?,?,?)", currentUser(r).ID, action, "github_issue", strconv.Itoa(number), now()); err != nil {
		// The remote mutation has already succeeded. An audit failure must not
		// invite a retry that would create a duplicate issue or comment.
		a.log.Printf("GitHub issue #%d %s audit failed", number, action)
	}
	a.emit(action, "github_issue", strconv.Itoa(number))
}

func (a *App) createIssue(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	body.Title = strings.TrimSpace(body.Title)
	if !validIssueText(body.Title, true, 256) || strings.ContainsAny(body.Title, "\r\n") || !validIssueText(body.Body, false, 60000) {
		writeError(w, fail(422, "validation", "标题须为 1–256 字，说明须不超过 60000 字"))
		return
	}
	var issue githubIssue
	if err := a.readmeGitHub.requestIssue(r.Context(), http.MethodPost, a.readmeGitHub.issuesPath(), map[string]any{"title": body.Title, "body": issueSignedBody(body.Body, currentUser(r))}, &issue); err != nil {
		writeError(w, err)
		return
	}
	if issue.Number < 1 {
		writeError(w, fail(502, "github_response", "GitHub 未返回问题编号，请先在 GitHub 核对创建结果，避免重复发布"))
		return
	}
	a.readmeGitHub.cleanIssue(&issue)
	a.issueActivity(r, "created", issue.Number)
	writeJSON(w, 201, issue)
}

func (a *App) getIssue(w http.ResponseWriter, r *http.Request) {
	issue, err := a.existingIssue(r)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"issue": issue, "repository": a.cfg.ReadmeRepository})
}

func (a *App) existingIssue(r *http.Request) (githubIssue, error) {
	number, err := issueNumber(r)
	if err != nil {
		return githubIssue{}, err
	}
	if !a.readmeGitHub.configured() {
		return githubIssue{}, fail(503, "github_not_configured", "问题功能尚未连接 GitHub，请管理员完成现有 GitHub App 配置")
	}
	return a.readmeGitHub.getIssue(r.Context(), number)
}

func (a *App) changeIssueState(w http.ResponseWriter, r *http.Request) {
	var body struct {
		State string `json:"state"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	if body.State != "open" && body.State != "closed" {
		writeError(w, fail(422, "validation", "只能关闭或重新打开问题"))
		return
	}
	issue, err := a.existingIssue(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if issue.State != body.State {
		number := issue.Number
		reason := "completed"
		if body.State == "open" {
			reason = "reopened"
		}
		if err := a.readmeGitHub.requestIssue(r.Context(), http.MethodPatch, a.readmeGitHub.issuesPath()+"/"+strconv.Itoa(issue.Number), map[string]string{"state": body.State, "state_reason": reason}, &issue); err != nil {
			writeError(w, err)
			return
		}
		if issue.Number != number || issue.State != body.State {
			writeError(w, fail(502, "github_response", "GitHub 返回了不完整的状态更新结果，请刷新核对后再操作"))
			return
		}
		a.readmeGitHub.cleanIssue(&issue)
		a.issueActivity(r, "updated", issue.Number)
	}
	writeJSON(w, 200, issue)
}

func (a *App) listIssueComments(w http.ResponseWriter, r *http.Request) {
	page, err := issuePage(r)
	if err != nil {
		writeError(w, err)
		return
	}
	issue, err := a.existingIssue(r)
	if err != nil {
		writeError(w, err)
		return
	}
	query := url.Values{"page": {strconv.Itoa(page)}, "per_page": {strconv.Itoa(issuePageSize)}}
	items := []githubIssueComment{}
	if err := a.readmeGitHub.requestIssue(r.Context(), http.MethodGet, a.readmeGitHub.issuesPath()+"/"+strconv.Itoa(issue.Number)+"/comments?"+query.Encode(), nil, &items); err != nil {
		writeError(w, err)
		return
	}
	nextPage := 0
	if len(items) == issuePageSize {
		nextPage = page + 1
	}
	for index := range items {
		items[index].HTMLURL = issue.HTMLURL + "#issuecomment-" + strconv.FormatInt(items[index].ID, 10)
	}
	if items == nil {
		items = []githubIssueComment{}
	}
	writeJSON(w, 200, map[string]any{"items": items, "next_page": nextPage})
}

func (a *App) createIssueComment(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Body string `json:"body"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	if !validIssueText(body.Body, true, 60000) {
		writeError(w, fail(422, "validation", "请填写不超过 60000 字的评论"))
		return
	}
	issue, err := a.existingIssue(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var comment githubIssueComment
	if err := a.readmeGitHub.requestIssue(r.Context(), http.MethodPost, a.readmeGitHub.issuesPath()+"/"+strconv.Itoa(issue.Number)+"/comments", map[string]string{"body": issueSignedBody(body.Body, currentUser(r))}, &comment); err != nil {
		writeError(w, err)
		return
	}
	if comment.ID < 1 {
		writeError(w, fail(502, "github_response", "GitHub 未返回评论编号，请刷新核对发送结果，避免重复评论"))
		return
	}
	comment.HTMLURL = issue.HTMLURL + "#issuecomment-" + strconv.FormatInt(comment.ID, 10)
	a.issueActivity(r, "commented", issue.Number)
	writeJSON(w, 201, comment)
}
