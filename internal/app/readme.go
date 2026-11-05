package app

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

type readmeDraft struct {
	Content       string `json:"content"`
	BaseContent   string `json:"base_content"`
	BaseSHA       string `json:"base_sha"`
	Version       int    `json:"version"`
	UpdatedAt     string `json:"updated_at"`
	LastCommitURL string `json:"last_commit_url"`
}

func (a *App) getReadmeRemote(w http.ResponseWriter, r *http.Request) {
	remote, err := a.readmeGitHub.read(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, remote)
}

func (a *App) readmeScope() (string, string) {
	return a.cfg.ReadmeRepository, a.cfg.ReadmeBranch
}

func (a *App) loadReadmeDraft(ctx context.Context, userID string) (readmeDraft, error) {
	repository, branch := a.readmeScope()
	var draft readmeDraft
	err := a.db.QueryRowContext(ctx, `SELECT content,base_content,base_sha,version,updated_at,last_commit_url
		FROM readme_drafts WHERE user_id=? AND repository=? AND branch=?`, userID, repository, branch).
		Scan(&draft.Content, &draft.BaseContent, &draft.BaseSHA, &draft.Version, &draft.UpdatedAt, &draft.LastCommitURL)
	return draft, err
}

func (a *App) getReadme(w http.ResponseWriter, r *http.Request) {
	a.readmeMu.Lock()
	defer a.readmeMu.Unlock()
	user := currentUser(r)
	draft, err := a.loadReadmeDraft(r.Context(), user.ID)
	remoteError := ""
	if errors.Is(err, sql.ErrNoRows) {
		remote := readmeRemote{}
		if a.readmeGitHub.configured() {
			remote, err = a.readmeGitHub.read(r.Context())
			if err != nil {
				remoteError = err.Error()
			}
		}
		repository, branch := a.readmeScope()
		_, err = a.db.ExecContext(r.Context(), `INSERT INTO readme_drafts(user_id,repository,branch,content,base_content,base_sha,updated_at)
			VALUES(?,?,?,?,?,?,?) ON CONFLICT DO NOTHING`, user.ID, repository, branch, remote.Content, remote.Content, remote.SHA, now())
		if err == nil {
			draft, err = a.loadReadmeDraft(r.Context(), user.ID)
		}
	}
	if err != nil {
		writeError(w, err)
		return
	}
	authors, selected, err := a.readmeAuthors(r.Context(), user.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	repository, branch := a.readmeScope()
	writeJSON(w, 200, map[string]any{
		"draft": draft, "authors": authors, "selected_author_id": selected, "remote_error": remoteError,
		"github": map[string]any{"configured": a.readmeGitHub.configured(), "repository": repository, "branch": branch, "path": "README.md"},
	})
}

func validReadmeContent(content string) error {
	if len(content) > maxReadmeBytes || !utf8.ValidString(content) || strings.ContainsRune(content, 0) {
		return fail(422, "validation", "README 必须为不超过 1 MiB 的 UTF-8 文本")
	}
	return nil
}

func (a *App) requireReadmeVersion(ctx context.Context, userID string, version int) (readmeDraft, error) {
	draft, err := a.loadReadmeDraft(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return draft, fail(409, "readme_draft_conflict", "请先重新打开读我页面，再保存草稿")
	}
	if err == nil && draft.Version != version {
		return draft, &APIError{Status: 409, Code: "readme_draft_conflict", Message: "你的草稿已在另一个页面更新，当前编辑内容已保留", Details: map[string]any{"draft": draft}}
	}
	return draft, err
}

func (a *App) storeReadmeDraft(ctx context.Context, userID string, draft readmeDraft) (readmeDraft, error) {
	repository, branch := a.readmeScope()
	draft.Version++
	draft.UpdatedAt = now()
	result, err := a.db.ExecContext(ctx, `UPDATE readme_drafts SET content=?,base_content=?,base_sha=?,version=?,updated_at=?,last_commit_url=?
		WHERE user_id=? AND repository=? AND branch=? AND version=?`, draft.Content, draft.BaseContent, draft.BaseSHA, draft.Version, draft.UpdatedAt, draft.LastCommitURL,
		userID, repository, branch, draft.Version-1)
	if err == nil {
		rows, rowsErr := result.RowsAffected()
		if rowsErr != nil {
			err = rowsErr
		} else if rows != 1 {
			err = fail(409, "readme_draft_conflict", "草稿已更新，请重新加载后合并")
		}
	}
	return draft, err
}

func (a *App) saveReadme(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Content string `json:"content"`
		Version int    `json:"version"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	if err := validReadmeContent(body.Content); err != nil {
		writeError(w, err)
		return
	}
	a.readmeMu.Lock()
	defer a.readmeMu.Unlock()
	draft, err := a.requireReadmeVersion(r.Context(), currentUser(r).ID, body.Version)
	if err == nil {
		draft.Content = body.Content
		draft, err = a.storeReadmeDraft(r.Context(), currentUser(r).ID, draft)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, draft)
}

func (a *App) refreshReadme(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Version      int  `json:"version"`
		DiscardDraft bool `json:"discard_draft"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	a.readmeMu.Lock()
	defer a.readmeMu.Unlock()
	draft, err := a.requireReadmeVersion(r.Context(), currentUser(r).ID, body.Version)
	if err == nil && draft.Content != draft.BaseContent && !body.DiscardDraft {
		err = fail(409, "readme_unsent_changes", "草稿含有尚未推送的修改，请比较合并，或明确选择用远端替换")
	}
	if err == nil {
		var remote readmeRemote
		remote, err = a.readmeGitHub.read(r.Context())
		if err == nil {
			draft.Content, draft.BaseContent, draft.BaseSHA = remote.Content, remote.Content, remote.SHA
			draft, err = a.storeReadmeDraft(r.Context(), currentUser(r).ID, draft)
		}
	}
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, draft)
}

func readmeRemoteConflict(remote readmeRemote) *APIError {
	return &APIError{Status: 409, Code: "readme_remote_conflict", Message: "GitHub 上的 README 已变化，请比较并合并后再提交；草稿已保留", Details: map[string]any{"remote": remote}}
}

func (a *App) rebaseReadme(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Version   int    `json:"version"`
		Content   string `json:"content"`
		RemoteSHA string `json:"remote_sha"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	if err := validReadmeContent(body.Content); err != nil {
		writeError(w, err)
		return
	}
	a.readmeMu.Lock()
	defer a.readmeMu.Unlock()
	draft, err := a.requireReadmeVersion(r.Context(), currentUser(r).ID, body.Version)
	if err == nil {
		var remote readmeRemote
		remote, err = a.readmeGitHub.read(r.Context())
		if err == nil && (body.RemoteSHA == "" || remote.SHA != body.RemoteSHA) {
			err = readmeRemoteConflict(remote)
		}
		if err == nil {
			draft.Content, draft.BaseContent, draft.BaseSHA = body.Content, remote.Content, remote.SHA
			draft, err = a.storeReadmeDraft(r.Context(), currentUser(r).ID, draft)
		}
	}
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, draft)
}

func (a *App) pushReadme(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Version       int    `json:"version"`
		Message       string `json:"message"`
		AuthorID      string `json:"author_id"`
		AuthorVersion int    `json:"author_version"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	body.Message = strings.TrimSpace(body.Message)
	if body.Message == "" || len([]rune(body.Message)) > 500 || strings.ContainsRune(body.Message, 0) {
		writeError(w, fail(422, "validation", "请填写不超过 500 字的提交说明"))
		return
	}
	a.readmeMu.Lock()
	defer a.readmeMu.Unlock()
	user := currentUser(r)
	draft, err := a.requireReadmeVersion(r.Context(), user.ID, body.Version)
	if err == nil && draft.BaseSHA == "" {
		err = fail(409, "readme_no_base", "请先从 GitHub 读取 README，再合并你的草稿后提交")
	}
	if err == nil && draft.Content == draft.BaseContent {
		err = fail(422, "readme_no_changes", "README 没有需要推送的修改")
	}
	var author readmeCommitAuthor
	if err == nil {
		author, err = a.requireReadmeAuthor(r.Context(), body.AuthorID, body.AuthorVersion)
	}
	if err == nil {
		var remote readmeRemote
		remote, err = a.readmeGitHub.read(r.Context())
		if err == nil && remote.SHA != draft.BaseSHA {
			err = readmeRemoteConflict(remote)
		}
	}
	if err == nil {
		var remote readmeRemote
		var commitURL string
		remote, commitURL, err = a.readmeGitHub.push(r.Context(), draft, author.readmeAuthor, body.Message)
		if conflict, ok := err.(*APIError); ok && conflict.Code == "readme_remote_conflict" {
			if latest, readErr := a.readmeGitHub.read(r.Context()); readErr == nil {
				err = readmeRemoteConflict(latest)
			}
		}
		if err == nil {
			// Persist success even if the browser disconnects after GitHub accepted it.
			ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
			defer cancel()
			draft.BaseContent, draft.BaseSHA, draft.LastCommitURL = remote.Content, remote.SHA, commitURL
			draft, err = a.storeReadmeDraft(ctx, user.ID, draft)
			if err == nil {
				_, err = a.db.ExecContext(ctx, "INSERT INTO audit_logs(actor_id,action,entity_type,entity_id,created_at) VALUES(?,?,?,?,?)", user.ID, "push", "readme", commitURL, now())
			}
		}
	}
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, draft)
}
