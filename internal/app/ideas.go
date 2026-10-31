package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"com-itemgather/internal/security"
)

type querier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

type ideaFields struct {
	OriginalContent   string      `json:"original_content"`
	Content           string      `json:"content"`
	DisplayTitle      string      `json:"display_title"`
	Summary           string      `json:"summary"`
	Notes             string      `json:"notes"`
	NameCandidates    []string    `json:"name_candidates"`
	IdeaTypes         []string    `json:"idea_types"`
	ProductDirections []string    `json:"product_directions"`
	Tags              []string    `json:"tags"`
	Attributes        []Attribute `json:"attributes"`
	ManualFields      []string    `json:"manual_fields"`
	ContentVersion    int         `json:"content_version"`
	AIStatus          string      `json:"ai_status"`
	AIError           string      `json:"ai_error,omitempty"`
}

const ideaSelect = `SELECT i.id,COALESCE(i.client_id,''),i.original_content,i.content,i.display_title,i.summary,i.notes,
i.name_candidates_json,i.idea_types_json,i.product_directions_json,i.tags_json,i.manual_fields_json,i.author_id,u.nickname,
i.created_at,i.updated_at,i.version,i.content_version,i.ai_status,i.ai_error,COALESCE(i.deleted_at,''),
EXISTS(SELECT 1 FROM favorites f WHERE f.idea_id=i.id AND f.user_id=?),
(SELECT COUNT(*) FROM idea_product_links l JOIN products p ON p.id=l.product_id WHERE l.idea_id=i.id AND p.deleted_at IS NULL)
FROM ideas i JOIN users u ON u.id=i.author_id `

func scanIdea(row *sql.Row, userID string) (Idea, error) {
	var item Idea
	var names, types, directions, tags, manual string
	var deleted string
	err := row.Scan(&item.ID, &item.ClientID, &item.OriginalContent, &item.Content, &item.DisplayTitle, &item.Summary, &item.Notes, &names, &types, &directions, &tags, &manual, &item.AuthorID, &item.AuthorNickname, &item.CreatedAt, &item.UpdatedAt, &item.Version, &item.ContentVersion, &item.AIStatus, &item.AIError, &deleted, &item.Favorite, &item.ProductCount)
	item.NameCandidates = decodeStrings(names)
	item.IdeaTypes = decodeStrings(types)
	item.ProductDirections = decodeStrings(directions)
	item.Tags = decodeStrings(tags)
	item.ManualFields = decodeStrings(manual)
	item.Attributes = []Attribute{}
	item.Attachments = []Attachment{}
	if deleted != "" {
		item.DeletedAt = deleted
	}
	return item, err
}

func loadIdea(ctx context.Context, q querier, id, userID string, deleted bool, related bool) (Idea, error) {
	clause := "WHERE i.id=?"
	if !deleted {
		clause += " AND i.deleted_at IS NULL"
	}
	item, err := scanIdea(q.QueryRowContext(ctx, ideaSelect+clause, userID, id), userID)
	if err != nil {
		return item, ensureNoRows(err)
	}
	rows, err := q.QueryContext(ctx, "SELECT id,key,value,certainty FROM idea_attributes WHERE idea_id=? ORDER BY position,id", id)
	if err != nil {
		return item, err
	}
	defer rows.Close()
	for rows.Next() {
		var attr Attribute
		rows.Scan(&attr.ID, &attr.Key, &attr.Value, &attr.Certainty)
		item.Attributes = append(item.Attributes, attr)
	}
	attachmentRows, err := q.QueryContext(ctx, "SELECT id,idea_id,original_name,mime_type,size,thumbnail_path IS NOT NULL,created_at FROM attachments WHERE idea_id=? ORDER BY created_at", id)
	if err != nil {
		return item, err
	}
	defer attachmentRows.Close()
	for attachmentRows.Next() {
		var file Attachment
		var thumb bool
		attachmentRows.Scan(&file.ID, &file.IdeaID, &file.OriginalName, &file.MimeType, &file.Size, &thumb, &file.CreatedAt)
		file.URL = "/api/attachments/" + file.ID
		if thumb {
			file.ThumbnailURL = file.URL + "?thumbnail=1"
		}
		item.Attachments = append(item.Attachments, file)
	}
	if related {
		item.Links = []Link{}
		linkRows, err := q.QueryContext(ctx, `SELECT l.id,l.idea_id,l.product_id,l.status,p.internal_name,l.created_at,l.updated_at FROM idea_product_links l JOIN products p ON p.id=l.product_id WHERE l.idea_id=? AND p.deleted_at IS NULL ORDER BY l.created_at DESC`, id)
		if err != nil {
			return item, err
		}
		defer linkRows.Close()
		for linkRows.Next() {
			var link Link
			linkRows.Scan(&link.ID, &link.IdeaID, &link.ProductID, &link.Status, &link.ProductName, &link.CreatedAt, &link.UpdatedAt)
			item.Links = append(item.Links, link)
		}
	}
	return item, nil
}

func ideaSnapshot(item Idea) ideaFields {
	return ideaFields{OriginalContent: item.OriginalContent, Content: item.Content, DisplayTitle: item.DisplayTitle, Summary: item.Summary, Notes: item.Notes, NameCandidates: item.NameCandidates, IdeaTypes: item.IdeaTypes, ProductDirections: item.ProductDirections, Tags: item.Tags, Attributes: item.Attributes, ManualFields: item.ManualFields, ContentVersion: item.ContentVersion, AIStatus: item.AIStatus, AIError: item.AIError}
}
func addRevision(ctx context.Context, tx *sql.Tx, kind, id string, version int, actor *string, action string, before, after any) error {
	var beforeJSON any
	if before != nil {
		beforeJSON = encodeJSON(before)
	}
	_, err := tx.ExecContext(ctx, "INSERT INTO revisions(id,entity_type,entity_id,version,actor_id,action,before_data,after_data,created_at) VALUES(?,?,?,?,?,?,?,?,?)", security.ID(), kind, id, version, actor, action, beforeJSON, encodeJSON(after), now())
	return err
}
func validStringList(values []string, max int) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		value = clean(value, 80)
		key := strings.ToLower(value)
		if value != "" && !seen[key] {
			seen[key] = true
			result = append(result, value)
		}
		if len(result) >= max {
			break
		}
	}
	return result
}
func validAttributes(values []Attribute) ([]Attribute, error) {
	if len(values) > 100 {
		return nil, fail(422, "validation", "属性不能超过 100 项")
	}
	result := make([]Attribute, 0, len(values))
	for _, attr := range values {
		attr.ID = ""
		attr.Key = clean(attr.Key, 80)
		attr.Value = clean(attr.Value, 2000)
		if attr.Key == "" {
			continue
		}
		if attr.Certainty != "asserted" && attr.Certainty != "tentative" && attr.Certainty != "undecided" {
			return nil, fail(422, "validation", "属性确定程度无效")
		}
		result = append(result, attr)
	}
	return result, nil
}

func replaceIdeaAttributes(ctx context.Context, tx *sql.Tx, id string, attrs []Attribute) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM idea_attributes WHERE idea_id=?", id); err != nil {
		return err
	}
	for index, attr := range attrs {
		if _, err := tx.ExecContext(ctx, "INSERT INTO idea_attributes(id,idea_id,key,value,certainty,position) VALUES(?,?,?,?,?,?)", security.ID(), id, attr.Key, attr.Value, attr.Certainty, index); err != nil {
			return err
		}
	}
	return nil
}
func replaceIdeaTags(ctx context.Context, tx *sql.Tx, id string, tags []string) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM idea_tags WHERE idea_id=?", id); err != nil {
		return err
	}
	for _, name := range tags {
		var tagID string
		err := tx.QueryRowContext(ctx, "SELECT id FROM tags WHERE name=? COLLATE NOCASE", name).Scan(&tagID)
		if err == sql.ErrNoRows {
			tagID = security.ID()
			_, err = tx.ExecContext(ctx, "INSERT INTO tags(id,name,created_at) VALUES(?,?,?)", tagID, name, now())
		}
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO idea_tags(idea_id,tag_id) VALUES(?,?)", id, tagID); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) createIdea(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ClientID string `json:"client_id"`
		Content  string `json:"content"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	if len(body.Content) > 200000 {
		writeError(w, fail(413, "too_large", "灵感文字过长"))
		return
	}
	body.ClientID = clean(body.ClientID, 100)
	user := currentUser(r)
	if body.ClientID != "" {
		var existingID string
		err := a.db.QueryRowContext(r.Context(), "SELECT id FROM ideas WHERE author_id=? AND client_id=?", user.ID, body.ClientID).Scan(&existingID)
		if err == nil {
			item, loadErr := loadIdea(r.Context(), a.db, existingID, user.ID, true, true)
			if loadErr != nil {
				writeError(w, loadErr)
				return
			}
			writeJSON(w, 200, item)
			return
		} else if err != sql.ErrNoRows {
			writeError(w, err)
			return
		}
	}
	id := security.ID()
	timestamp := now()
	status := "skipped"
	if strings.TrimSpace(body.Content) != "" {
		status = "pending"
	}
	title := clean(body.Content, 80)
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(r.Context(), `INSERT INTO ideas(id,client_id,original_content,content,display_title,author_id,created_at,updated_at,ai_status) VALUES(?,?,?,?,?,?,?,?,?)`, id, nullString(body.ClientID), body.Content, body.Content, title, user.ID, timestamp, timestamp, status)
	if err != nil {
		writeError(w, fail(409, "duplicate_submission", "重复提交"))
		return
	}
	if status == "pending" {
		if err = a.enqueueAITx(r.Context(), tx, id, body.Content, 1, "normal"); err != nil {
			writeError(w, err)
			return
		}
	}
	item := Idea{ID: id, ClientID: body.ClientID, OriginalContent: body.Content, Content: body.Content, DisplayTitle: title, AuthorID: user.ID, CreatedAt: timestamp, UpdatedAt: timestamp, Version: 1, ContentVersion: 1, AIStatus: status, NameCandidates: []string{}, IdeaTypes: []string{}, ProductDirections: []string{}, Tags: []string{}, Attributes: []Attribute{}, Attachments: []Attachment{}}
	if err = addRevision(r.Context(), tx, "idea", id, 1, &user.ID, "create", nil, ideaSnapshot(item)); err != nil {
		writeError(w, err)
		return
	}
	if err = a.publish(tx, "idea.created", "idea", id); err != nil {
		writeError(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		writeError(w, err)
		return
	}
	a.afterCommit()
	a.ai.Wake()
	item, err = loadIdea(r.Context(), a.db, id, user.ID, true, true)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 201, item)
}

func (a *App) listIdeas(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	filter := r.URL.Query().Get("filter")
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit < 1 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	where := "i.deleted_at IS NULL"
	args := []any{}
	switch filter {
	case "favorites":
		where += " AND EXISTS(SELECT 1 FROM favorites f WHERE f.idea_id=i.id AND f.user_id=?)"
		args = append(args, user.ID)
	case "unlinked":
		where += " AND NOT EXISTS(SELECT 1 FROM idea_product_links l JOIN products p ON p.id=l.product_id WHERE l.idea_id=i.id AND p.deleted_at IS NULL)"
	case "mine":
		where += " AND i.author_id=?"
		args = append(args, user.ID)
	}
	if query != "" {
		where += " AND (i.original_content LIKE ? ESCAPE '\\' OR i.content LIKE ? ESCAPE '\\' OR i.display_title LIKE ? ESCAPE '\\' OR i.summary LIKE ? ESCAPE '\\' OR i.tags_json LIKE ? ESCAPE '\\')"
		like := "%" + escapeLike(query) + "%"
		for i := 0; i < 5; i++ {
			args = append(args, like)
		}
	}
	var total int
	a.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM ideas i WHERE "+where, args...).Scan(&total)
	args = append(args, user.ID, limit, offset)
	rows, err := a.db.QueryContext(r.Context(), ideaSelect+" WHERE "+where+" ORDER BY i.created_at DESC LIMIT ? OFFSET ?", reorderIdeaArgs(args)...)
	if err != nil {
		writeError(w, err)
		return
	}
	defer rows.Close()
	items := []Idea{}
	for rows.Next() {
		var item Idea
		var names, types, directions, tags, manual, deleted string
		err = rows.Scan(&item.ID, &item.ClientID, &item.OriginalContent, &item.Content, &item.DisplayTitle, &item.Summary, &item.Notes, &names, &types, &directions, &tags, &manual, &item.AuthorID, &item.AuthorNickname, &item.CreatedAt, &item.UpdatedAt, &item.Version, &item.ContentVersion, &item.AIStatus, &item.AIError, &deleted, &item.Favorite, &item.ProductCount)
		if err != nil {
			writeError(w, err)
			return
		}
		item.NameCandidates = decodeStrings(names)
		item.IdeaTypes = decodeStrings(types)
		item.ProductDirections = decodeStrings(directions)
		item.Tags = decodeStrings(tags)
		item.Attributes = []Attribute{}
		item.Attachments = []Attachment{}
		items = append(items, item)
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": total, "limit": limit, "offset": offset})
}

// ideaSelect starts with the current-user favorite placeholder; filter placeholders precede it in the final SQL.
func reorderIdeaArgs(args []any) []any {
	if len(args) < 3 {
		return args
	}
	userID := args[len(args)-3]
	paging := args[len(args)-2:]
	filters := args[:len(args)-3]
	result := []any{userID}
	result = append(result, filters...)
	return append(result, paging...)
}
func escapeLike(value string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_")
	return replacer.Replace(value)
}
func (a *App) getIdea(w http.ResponseWriter, r *http.Request) {
	item, err := loadIdea(r.Context(), a.db, r.PathValue("id"), currentUser(r).ID, true, true)
	if err != nil {
		writeError(w, err)
		return
	}
	if item.DeletedAt != "" {
		writeError(w, fail(404, "not_found", "灵感不存在或已删除"))
		return
	}
	writeJSON(w, 200, item)
}

func (a *App) updateIdea(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Version           int          `json:"version"`
		Content           *string      `json:"content"`
		DisplayTitle      *string      `json:"display_title"`
		Summary           *string      `json:"summary"`
		Notes             *string      `json:"notes"`
		NameCandidates    *[]string    `json:"name_candidates"`
		IdeaTypes         *[]string    `json:"idea_types"`
		ProductDirections *[]string    `json:"product_directions"`
		Tags              *[]string    `json:"tags"`
		Attributes        *[]Attribute `json:"attributes"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	user := currentUser(r)
	id := r.PathValue("id")
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer tx.Rollback()
	item, err := loadIdea(r.Context(), tx, id, user.ID, false, true)
	if err != nil {
		writeError(w, err)
		return
	}
	if item.Version != body.Version {
		writeError(w, &APIError{Status: 409, Code: "version_conflict", Message: "这条灵感已被其他人修改", Details: map[string]any{"current": item, "submitted": body}})
		return
	}
	before := ideaSnapshot(item)
	manual := map[string]bool{}
	for _, field := range decodeStrings(mustManualFields(r.Context(), tx, id)) {
		manual[field] = true
	}
	contentChanged := false
	if body.Content != nil {
		if len(*body.Content) > 200000 {
			writeError(w, fail(413, "too_large", "灵感文字过长"))
			return
		}
		contentChanged = item.Content != *body.Content
		item.Content = *body.Content
		manual["content"] = true
	}
	if body.DisplayTitle != nil {
		item.DisplayTitle = clean(*body.DisplayTitle, 160)
		manual["display_title"] = true
	}
	if body.Summary != nil {
		item.Summary = clean(*body.Summary, 4000)
		manual["summary"] = true
	}
	if body.Notes != nil {
		item.Notes = clean(*body.Notes, 20000)
	}
	if body.NameCandidates != nil {
		item.NameCandidates = validStringList(*body.NameCandidates, 100)
		manual["name_candidates"] = true
	}
	if body.IdeaTypes != nil {
		item.IdeaTypes = validStringList(*body.IdeaTypes, 30)
		manual["idea_types"] = true
	}
	if body.ProductDirections != nil {
		item.ProductDirections = validStringList(*body.ProductDirections, 30)
		manual["product_directions"] = true
	}
	if body.Tags != nil {
		item.Tags = validStringList(*body.Tags, 50)
		manual["tags"] = true
	}
	if body.Attributes != nil {
		attrs, attrErr := validAttributes(*body.Attributes)
		if attrErr != nil {
			writeError(w, attrErr)
			return
		}
		item.Attributes = attrs
		manual["attributes"] = true
	}
	if contentChanged {
		item.ContentVersion++
		item.AIStatus = "idle"
		item.AIError = ""
		if _, err = tx.ExecContext(r.Context(), "UPDATE ai_tasks SET status='cancelled',updated_at=? WHERE idea_id=? AND status IN ('pending','running')", now(), id); err != nil {
			writeError(w, err)
			return
		}
	}
	manualList := make([]string, 0, len(manual))
	for field := range manual {
		manualList = append(manualList, field)
	}
	item.Version++
	item.UpdatedAt = now()
	result, err := tx.ExecContext(r.Context(), `UPDATE ideas SET content=?,display_title=?,summary=?,notes=?,name_candidates_json=?,idea_types_json=?,product_directions_json=?,tags_json=?,manual_fields_json=?,content_version=?,ai_status=?,ai_error=?,version=?,updated_at=? WHERE id=? AND version=?`, item.Content, item.DisplayTitle, item.Summary, item.Notes, encodeJSON(item.NameCandidates), encodeJSON(item.IdeaTypes), encodeJSON(item.ProductDirections), encodeJSON(item.Tags), encodeJSON(manualList), item.ContentVersion, item.AIStatus, item.AIError, item.Version, item.UpdatedAt, id, body.Version)
	if err != nil {
		writeError(w, err)
		return
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		writeError(w, fail(409, "version_conflict", "这条灵感已被其他人修改"))
		return
	}
	if body.Attributes != nil {
		if err = replaceIdeaAttributes(r.Context(), tx, id, item.Attributes); err != nil {
			writeError(w, err)
			return
		}
	}
	if body.Tags != nil {
		if err = replaceIdeaTags(r.Context(), tx, id, item.Tags); err != nil {
			writeError(w, err)
			return
		}
	}
	if err = addRevision(r.Context(), tx, "idea", id, item.Version, &user.ID, "update", before, ideaSnapshot(item)); err != nil {
		writeError(w, err)
		return
	}
	if err = a.publish(tx, "idea.updated", "idea", id); err != nil {
		writeError(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		writeError(w, err)
		return
	}
	a.afterCommit()
	item, err = loadIdea(r.Context(), a.db, id, user.ID, true, true)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, item)
}
func mustManualFields(ctx context.Context, q querier, id string) string {
	var raw string
	q.QueryRowContext(ctx, "SELECT manual_fields_json FROM ideas WHERE id=?", id).Scan(&raw)
	return raw
}

func (a *App) favoriteIdea(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Favorite bool `json:"favorite"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	user := currentUser(r)
	id := r.PathValue("id")
	var err error
	if body.Favorite {
		_, err = a.db.ExecContext(r.Context(), "INSERT OR IGNORE INTO favorites(user_id,idea_id,created_at) SELECT ?,id,? FROM ideas WHERE id=? AND deleted_at IS NULL", user.ID, now(), id)
	} else {
		_, err = a.db.ExecContext(r.Context(), "DELETE FROM favorites WHERE user_id=? AND idea_id=?", user.ID, id)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"favorite": body.Favorite})
}

func (a *App) deleteIdea(w http.ResponseWriter, r *http.Request)  { a.changeIdeaDeleted(w, r, true) }
func (a *App) restoreIdea(w http.ResponseWriter, r *http.Request) { a.changeIdeaDeleted(w, r, false) }
func (a *App) changeIdeaDeleted(w http.ResponseWriter, r *http.Request, remove bool) {
	var body struct {
		Version int `json:"version"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	user := currentUser(r)
	id := r.PathValue("id")
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer tx.Rollback()
	item, err := loadIdea(r.Context(), tx, id, user.ID, true, false)
	if err != nil {
		writeError(w, err)
		return
	}
	if item.Version != body.Version {
		writeError(w, fail(409, "version_conflict", "记录版本已变化，请刷新后重试"))
		return
	}
	before := ideaSnapshot(item)
	item.Version++
	item.UpdatedAt = now()
	action := "restore"
	var deleted any
	if remove {
		deleted = now()
		item.DeletedAt = deleted.(string)
		action = "delete"
	} else {
		item.DeletedAt = ""
	}
	result, err := tx.ExecContext(r.Context(), "UPDATE ideas SET deleted_at=?,version=?,updated_at=? WHERE id=? AND version=?", deleted, item.Version, item.UpdatedAt, id, body.Version)
	if err != nil {
		writeError(w, err)
		return
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		writeError(w, fail(409, "version_conflict", "记录版本已变化"))
		return
	}
	if err = addRevision(r.Context(), tx, "idea", id, item.Version, &user.ID, action, before, ideaSnapshot(item)); err != nil {
		writeError(w, err)
		return
	}
	a.publish(tx, "idea."+action, "idea", id)
	if err = tx.Commit(); err != nil {
		writeError(w, err)
		return
	}
	a.afterCommit()
	writeJSON(w, 200, map[string]any{"ok": true, "version": item.Version})
}
func (a *App) permanentDeleteIdea(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Version int `json:"version"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	user := currentUser(r)
	id := r.PathValue("id")
	item, err := loadIdea(r.Context(), a.db, id, user.ID, true, false)
	if err != nil {
		writeError(w, err)
		return
	}
	if item.DeletedAt == "" {
		writeError(w, fail(400, "not_deleted", "请先将灵感移入回收站"))
		return
	}
	if item.Version != body.Version {
		writeError(w, fail(409, "version_conflict", "记录版本已变化"))
		return
	}
	paths := a.attachmentPaths(r.Context(), a.db, "idea", id)
	tx, _ := a.db.BeginTx(r.Context(), nil)
	if _, err = tx.ExecContext(r.Context(), "DELETE FROM ideas WHERE id=? AND version=?", id, body.Version); err == nil {
		err = a.publish(tx, "idea.deleted_permanently", "idea", id)
	}
	if err != nil {
		tx.Rollback()
		writeError(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		writeError(w, err)
		return
	}
	removeFiles(paths)
	a.afterCommit()
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *App) retryAI(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Mode string `json:"mode"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	if body.Mode == "" {
		body.Mode = "normal"
	}
	if body.Mode != "normal" && body.Mode != "full" {
		writeError(w, fail(422, "validation", "整理模式无效"))
		return
	}
	user := currentUser(r)
	id := r.PathValue("id")
	item, err := loadIdea(r.Context(), a.db, id, user.ID, false, false)
	if err != nil {
		writeError(w, err)
		return
	}
	if strings.TrimSpace(item.Content) == "" {
		writeError(w, fail(400, "empty_content", "请先补充文字再整理"))
		return
	}
	tx, _ := a.db.BeginTx(r.Context(), nil)
	defer tx.Rollback()
	var active int
	tx.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM ai_tasks WHERE idea_id=? AND status IN ('pending','running')", id).Scan(&active)
	if active > 0 {
		writeError(w, fail(409, "ai_running", "已有整理任务正在执行"))
		return
	}
	if _, err = tx.ExecContext(r.Context(), "UPDATE ideas SET ai_status='pending',ai_error='',updated_at=? WHERE id=?", now(), id); err != nil {
		writeError(w, err)
		return
	}
	if err = a.enqueueAITx(r.Context(), tx, id, item.Content, item.ContentVersion, body.Mode); err != nil {
		writeError(w, err)
		return
	}
	a.publish(tx, "idea.ai_pending", "idea", id)
	if err = tx.Commit(); err != nil {
		writeError(w, err)
		return
	}
	a.afterCommit()
	a.ai.Wake()
	writeJSON(w, 202, map[string]any{"ok": true})
}

func (a *App) listRevisions(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := a.db.QueryContext(r.Context(), `SELECT v.id,v.entity_type,v.entity_id,v.version,COALESCE(v.actor_id,''),COALESCE(u.nickname,''),v.action,COALESCE(v.before_data,'null'),v.after_data,v.created_at FROM revisions v LEFT JOIN users u ON u.id=v.actor_id WHERE v.entity_type=? AND v.entity_id=? ORDER BY v.version DESC LIMIT 100`, kind, r.PathValue("id"))
		if err != nil {
			writeError(w, err)
			return
		}
		defer rows.Close()
		items := []Revision{}
		for rows.Next() {
			var item Revision
			var before, after string
			rows.Scan(&item.ID, &item.EntityType, &item.EntityID, &item.Version, &item.ActorID, &item.ActorNickname, &item.Action, &before, &after, &item.CreatedAt)
			item.Before = json.RawMessage(before)
			item.After = json.RawMessage(after)
			items = append(items, item)
		}
		writeJSON(w, 200, map[string]any{"items": items})
	}
}
func (a *App) restoreRevision(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Version int `json:"version"`
		}
		if err := decodeBody(w, r, &body); err != nil {
			writeError(w, err)
			return
		}
		var raw string
		err := a.db.QueryRowContext(r.Context(), "SELECT after_data FROM revisions WHERE id=? AND entity_type=? AND entity_id=?", r.PathValue("revision"), kind, r.PathValue("id")).Scan(&raw)
		if err != nil {
			writeError(w, ensureNoRows(err))
			return
		}
		if kind == "idea" {
			var data ideaFields
			if json.Unmarshal([]byte(raw), &data) != nil {
				writeError(w, fail(500, "invalid_revision", "历史版本数据无效"))
				return
			}
			a.restoreIdeaRevision(w, r, body.Version, data)
		} else {
			var data productFields
			if json.Unmarshal([]byte(raw), &data) != nil {
				writeError(w, fail(500, "invalid_revision", "历史版本数据无效"))
				return
			}
			a.restoreProductRevision(w, r, body.Version, data)
		}
	}
}
