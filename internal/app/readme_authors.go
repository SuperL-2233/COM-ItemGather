package app

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/mail"
	"strings"

	"com-itemgather/internal/security"
)

type readmeCommitAuthor struct {
	ID string `json:"id"`
	readmeAuthor
	Version   int    `json:"version"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func (a *App) readmeAuthors(ctx context.Context, userID string) ([]readmeCommitAuthor, string, error) {
	rows, err := a.db.QueryContext(ctx, "SELECT id,name,email,version,created_at,updated_at FROM readme_authors ORDER BY name COLLATE NOCASE,email,id")
	if err != nil {
		return nil, "", err
	}
	authors := []readmeCommitAuthor{}
	for rows.Next() {
		var author readmeCommitAuthor
		if err = rows.Scan(&author.ID, &author.Name, &author.Email, &author.Version, &author.CreatedAt, &author.UpdatedAt); err != nil {
			rows.Close()
			return nil, "", err
		}
		authors = append(authors, author)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, "", err
	}
	var selected string
	err = a.db.QueryRowContext(ctx, "SELECT COALESCE(author_id,'') FROM readme_author_choices WHERE user_id=?", userID).Scan(&selected)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return authors, selected, err
}

func (a *App) listReadmeAuthors(w http.ResponseWriter, r *http.Request) {
	authors, selected, err := a.readmeAuthors(r.Context(), currentUser(r).ID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": authors, "selected_author_id": selected})
}

func validateReadmeAuthor(author *readmeAuthor) error {
	author.Name, author.Email = strings.TrimSpace(author.Name), strings.TrimSpace(author.Email)
	address, err := mail.ParseAddress(author.Email)
	if author.Name == "" || len([]rune(author.Name)) > 100 || strings.ContainsAny(author.Name+author.Email, "\r\n\x00") || len(author.Email) > 254 || err != nil || address.Address != author.Email {
		return fail(422, "validation", "请填写作者姓名和完整的 GitHub 提交邮箱，不要填写昵称或带显示名称的邮箱")
	}
	return nil
}

func (a *App) requireReadmeAuthor(ctx context.Context, id string, version int) (readmeCommitAuthor, error) {
	var author readmeCommitAuthor
	if id == "" {
		return author, fail(422, "readme_author_required", "请先选择本次提交的作者；可在设置中管理作者")
	}
	err := a.db.QueryRowContext(ctx, "SELECT id,name,email,version,created_at,updated_at FROM readme_authors WHERE id=?", id).
		Scan(&author.ID, &author.Name, &author.Email, &author.Version, &author.CreatedAt, &author.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return author, fail(422, "readme_author_unavailable", "该作者已被删除，请重新选择提交作者")
	}
	if err == nil && author.Version != version {
		err = &APIError{Status: 409, Code: "readme_author_conflict", Message: "作者资料已更新，请核对姓名和邮箱后重新确认", Details: map[string]any{"author": author}}
	}
	return author, err
}

func (a *App) duplicateReadmeAuthor(ctx context.Context, author readmeAuthor, exceptID string) error {
	var count int
	err := a.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM readme_authors WHERE name=? AND email=? AND id<>?", author.Name, author.Email, exceptID).Scan(&count)
	if err == nil && count > 0 {
		return fail(409, "readme_author_duplicate", "相同姓名和邮箱的作者已存在")
	}
	return err
}

func (a *App) createReadmeAuthor(w http.ResponseWriter, r *http.Request) {
	var identity readmeAuthor
	if err := decodeBody(w, r, &identity); err != nil {
		writeError(w, err)
		return
	}
	if err := validateReadmeAuthor(&identity); err != nil {
		writeError(w, err)
		return
	}
	a.readmeMu.Lock()
	defer a.readmeMu.Unlock()
	if err := a.duplicateReadmeAuthor(r.Context(), identity, ""); err != nil {
		writeError(w, err)
		return
	}
	author := readmeCommitAuthor{ID: security.ID(), readmeAuthor: identity, Version: 1, CreatedAt: now(), UpdatedAt: now()}
	_, err := a.db.ExecContext(r.Context(), "INSERT INTO readme_authors(id,name,email,version,created_at,updated_at) VALUES(?,?,?,?,?,?)", author.ID, author.Name, author.Email, author.Version, author.CreatedAt, author.UpdatedAt)
	if err != nil {
		writeError(w, err)
		return
	}
	a.emit("created", "readme_author", author.ID)
	writeJSON(w, 201, author)
}

func (a *App) updateReadmeAuthor(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name    string `json:"name"`
		Email   string `json:"email"`
		Version int    `json:"version"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	identity := readmeAuthor{Name: body.Name, Email: body.Email}
	if err := validateReadmeAuthor(&identity); err != nil {
		writeError(w, err)
		return
	}
	a.readmeMu.Lock()
	defer a.readmeMu.Unlock()
	author, err := a.requireReadmeAuthor(r.Context(), r.PathValue("id"), body.Version)
	if err == nil {
		err = a.duplicateReadmeAuthor(r.Context(), identity, author.ID)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	author.readmeAuthor, author.Version, author.UpdatedAt = identity, author.Version+1, now()
	result, err := a.db.ExecContext(r.Context(), "UPDATE readme_authors SET name=?,email=?,version=?,updated_at=? WHERE id=? AND version=?", author.Name, author.Email, author.Version, author.UpdatedAt, author.ID, body.Version)
	if err == nil {
		if rows, rowsErr := result.RowsAffected(); rowsErr != nil {
			err = rowsErr
		} else if rows != 1 {
			err = fail(409, "readme_author_conflict", "作者资料已更新，请重新加载后修改")
		}
	}
	if err != nil {
		writeError(w, err)
		return
	}
	a.emit("updated", "readme_author", author.ID)
	writeJSON(w, 200, author)
}

func (a *App) deleteReadmeAuthor(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Version int `json:"version"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	a.readmeMu.Lock()
	defer a.readmeMu.Unlock()
	if _, err := a.requireReadmeAuthor(r.Context(), r.PathValue("id"), body.Version); err != nil {
		writeError(w, err)
		return
	}
	result, err := a.db.ExecContext(r.Context(), "DELETE FROM readme_authors WHERE id=? AND version=?", r.PathValue("id"), body.Version)
	if err == nil {
		if rows, rowsErr := result.RowsAffected(); rowsErr != nil {
			err = rowsErr
		} else if rows != 1 {
			err = fail(409, "readme_author_conflict", "作者资料已更新，请重新加载后删除")
		}
	}
	if err != nil {
		writeError(w, err)
		return
	}
	a.emit("deleted", "readme_author", r.PathValue("id"))
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (a *App) selectReadmeAuthor(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AuthorID      string `json:"author_id"`
		AuthorVersion int    `json:"author_version"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	a.readmeMu.Lock()
	defer a.readmeMu.Unlock()
	var selection any
	if body.AuthorID != "" {
		if _, err := a.requireReadmeAuthor(r.Context(), body.AuthorID, body.AuthorVersion); err != nil {
			writeError(w, err)
			return
		}
		selection = body.AuthorID
	}
	_, err := a.db.ExecContext(r.Context(), "INSERT INTO readme_author_choices(user_id,author_id) VALUES(?,?) ON CONFLICT(user_id) DO UPDATE SET author_id=excluded.author_id", currentUser(r).ID, selection)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"selected_author_id": body.AuthorID})
}
