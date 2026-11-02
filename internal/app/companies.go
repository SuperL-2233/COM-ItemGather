package app

import (
	"context"
	"database/sql"
	"net/http"
	"strings"

	"com-itemgather/internal/security"
)

func requestItemKind(r *http.Request) string {
	if strings.HasPrefix(r.URL.Path, "/api/tools") {
		return "tool"
	}
	return "product"
}
func companyName(c Company) string {
	if c.ChineseName != "" {
		return c.ChineseName
	}
	return c.EnglishName
}
func resolveCompany(ctx context.Context, tx *sql.Tx, id, name, user string) (string, string, error) {
	if id != "" {
		var c Company
		err := tx.QueryRowContext(ctx, "SELECT id,chinese_name,english_name FROM companies WHERE id=? AND deleted_at IS NULL", id).Scan(&c.ID, &c.ChineseName, &c.EnglishName)
		return c.ID, companyName(c), ensureNoRows(err)
	}
	name = clean(name, 160)
	if name == "" {
		return "", "", nil
	}
	var existing Company
	err := tx.QueryRowContext(ctx, "SELECT id,chinese_name,english_name FROM companies WHERE deleted_at IS NULL AND (chinese_name=? COLLATE NOCASE OR english_name=? COLLATE NOCASE) ORDER BY created_at LIMIT 1", name, name).Scan(&existing.ID, &existing.ChineseName, &existing.EnglishName)
	if err == nil {
		return existing.ID, companyName(existing), nil
	}
	if err != sql.ErrNoRows {
		return "", "", err
	}
	id = security.ID()
	stamp := now()
	_, err = tx.ExecContext(ctx, "INSERT INTO companies(id,chinese_name,created_by,created_at,updated_at) VALUES(?,?,?,?,?)", id, name, user, stamp, stamp)
	if err == nil {
		c := Company{ID: id, ChineseName: name, CreatedBy: user, CreatedAt: stamp, UpdatedAt: stamp, Version: 1}
		err = addRevision(ctx, tx, "company", id, 1, &user, "create", nil, c)
	}
	return id, name, err
}
func ensureUseCategory(ctx context.Context, tx *sql.Tx, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	if len([]rune(name)) > 80 || name == "+" || name == "＋" {
		return fail(422, "validation", "类别名称应为 1 至 80 个字符")
	}
	_, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO tool_use_categories(name,created_at) VALUES(?,?)", name, now())
	return err
}
func validateToolSettings(s ToolSettings) error {
	if len(s.Sources) > 100 {
		return fail(422, "validation", "获得来源过多")
	}
	for _, v := range []string{s.Trigger, s.Effect, s.Cooldown, s.Charges, s.Consumption, s.Duration} {
		if len([]rune(v)) > 20000 {
			return fail(422, "validation", "道具说明过长")
		}
	}
	for _, source := range s.Sources {
		for _, v := range []string{source.Method, source.Provider, source.Timing, source.Conditions, source.Price} {
			if len([]rune(v)) > 4000 {
				return fail(422, "validation", "获得来源说明过长")
			}
		}
	}
	for _, status := range []string{s.ModelStatus, s.FunctionStatus} {
		if status != "" && !validProductStatus(status) {
			return fail(422, "validation", "道具制作进度无效")
		}
	}
	return nil
}
func (a *App) toolCategories(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		var body struct {
			Name string `json:"name"`
		}
		if err := decodeBody(w, r, &body); err != nil {
			writeError(w, err)
			return
		}
		name := strings.TrimSpace(body.Name)
		if name == "" {
			writeError(w, fail(422, "validation", "请输入类别名称"))
			return
		}
		tx, err := a.db.BeginTx(r.Context(), nil)
		if err != nil {
			writeError(w, err)
			return
		}
		defer tx.Rollback()
		if err = ensureUseCategory(r.Context(), tx, name); err == nil {
			err = a.publish(tx, "category.changed", "tool_category", name)
		}
		if err == nil {
			err = tx.Commit()
		}
		if err != nil {
			writeError(w, err)
			return
		}
		a.afterCommit()
	}
	rows, err := a.db.QueryContext(r.Context(), "SELECT name FROM tool_use_categories ORDER BY CASE name WHEN '主动' THEN 0 WHEN '被动' THEN 1 ELSE 2 END,created_at,name")
	if err != nil {
		writeError(w, err)
		return
	}
	defer rows.Close()
	items := []string{}
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			writeError(w, err)
			return
		}
		items = append(items, name)
	}
	writeJSON(w, 200, map[string]any{"items": items})
}
func loadCompany(ctx context.Context, q querier, id string, deleted bool) (Company, error) {
	var c Company
	clause := ""
	if !deleted {
		clause = " AND c.deleted_at IS NULL"
	}
	err := q.QueryRowContext(ctx, `SELECT c.id,c.chinese_name,c.english_name,c.aliases,c.description,COALESCE(c.created_by,''),c.created_at,c.updated_at,c.version,COALESCE(c.deleted_at,''),(SELECT COUNT(*) FROM products p WHERE p.company_id=c.id AND p.deleted_at IS NULL) FROM companies c WHERE c.id=?`+clause, id).Scan(&c.ID, &c.ChineseName, &c.EnglishName, &c.Aliases, &c.Description, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt, &c.Version, &c.DeletedAt, &c.ItemCount)
	if err != nil {
		return c, ensureNoRows(err)
	}
	c.Attachments, err = loadAttachments(ctx, q, "company", id, -1)
	return c, err
}
func queryIDs(ctx context.Context, q querier, query string, args ...any) ([]string, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
func listCompaniesData(ctx context.Context, q querier, search, author string, deleted bool) ([]Company, error) {
	where := "c.deleted_at IS NULL"
	if deleted {
		where = "1=1"
	}
	like := "%" + escapeLike(search) + "%"
	ids, err := queryIDs(ctx, q, `SELECT c.id FROM companies c WHERE `+where+` AND (?='' OR c.created_by=?) AND (c.chinese_name LIKE ? ESCAPE '\' OR c.english_name LIKE ? ESCAPE '\' OR c.aliases LIKE ? ESCAPE '\' OR c.description LIKE ? ESCAPE '\') ORDER BY c.updated_at DESC,c.id`, author, author, like, like, like, like)
	if err != nil {
		return nil, err
	}
	items := []Company{}
	for _, id := range ids {
		c, e := loadCompany(ctx, q, id, true)
		if e != nil {
			return nil, e
		}
		items = append(items, c)
	}
	return items, nil
}
func (a *App) listCompanies(w http.ResponseWriter, r *http.Request) {
	items, err := listCompaniesData(r.Context(), a.db, r.URL.Query().Get("q"), "", false)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": len(items)})
}
func (a *App) getCompany(w http.ResponseWriter, r *http.Request) {
	c, err := loadCompany(r.Context(), a.db, r.PathValue("id"), false)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, c)
}
func (a *App) saveCompany(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Version     int    `json:"version"`
		ChineseName string `json:"chinese_name"`
		EnglishName string `json:"english_name"`
		Aliases     string `json:"aliases"`
		Description string `json:"description"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	body.ChineseName = clean(body.ChineseName, 160)
	body.EnglishName = clean(body.EnglishName, 160)
	if body.ChineseName == "" && body.EnglishName == "" {
		writeError(w, fail(422, "validation", "请至少填写一个公司名称"))
		return
	}
	if len(body.Description) > 100000 || len([]rune(body.Aliases)) > 2000 {
		writeError(w, fail(422, "validation", "公司设定或别名过长"))
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer tx.Rollback()
	user := currentUser(r).ID
	id := r.PathValue("id")
	stamp := now()
	var before any
	c := Company{ID: id, ChineseName: body.ChineseName, EnglishName: body.EnglishName, Aliases: body.Aliases, Description: body.Description, Version: 1, CreatedBy: user, CreatedAt: stamp, UpdatedAt: stamp}
	action := "create"
	code := 201
	if r.Method == "PATCH" {
		previous, e := loadCompany(r.Context(), tx, id, false)
		if e != nil {
			writeError(w, e)
			return
		}
		if previous.Version != body.Version {
			writeError(w, &APIError{Status: 409, Code: "version_conflict", Message: "公司已被其他人修改", Details: map[string]any{"current": previous, "submitted": body}})
			return
		}
		before = previous
		c.Version = previous.Version + 1
		c.CreatedBy = previous.CreatedBy
		c.CreatedAt = previous.CreatedAt
		action = "update"
		if strings.Contains(r.URL.Path, "/revisions/") {
			action = "restore_revision"
		}
		code = 200
		_, err = tx.ExecContext(r.Context(), "UPDATE companies SET chinese_name=?,english_name=?,aliases=?,description=?,version=?,updated_at=? WHERE id=? AND version=?", c.ChineseName, c.EnglishName, c.Aliases, c.Description, c.Version, stamp, id, body.Version)
	} else {
		c.ID = security.ID()
		_, err = tx.ExecContext(r.Context(), "INSERT INTO companies(id,chinese_name,english_name,aliases,description,version,created_by,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)", c.ID, c.ChineseName, c.EnglishName, c.Aliases, c.Description, c.Version, user, stamp, stamp)
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), "UPDATE products SET company=? WHERE company_id=?", companyName(c), c.ID)
	}
	if err == nil {
		err = addRevision(r.Context(), tx, "company", c.ID, c.Version, &user, action, before, c)
	}
	if err == nil {
		err = a.publish(tx, "company."+action, "company", c.ID)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		writeError(w, err)
		return
	}
	a.afterCommit()
	c, err = loadCompany(r.Context(), a.db, c.ID, false)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, code, c)
}
func (a *App) mergeCompany(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Version       int    `json:"version"`
		TargetID      string `json:"target_id"`
		TargetVersion int    `json:"target_version"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer tx.Rollback()
	source, err := loadCompany(r.Context(), tx, r.PathValue("id"), false)
	if err != nil {
		writeError(w, err)
		return
	}
	target, err := loadCompany(r.Context(), tx, body.TargetID, false)
	if err != nil {
		writeError(w, err)
		return
	}
	if source.ID == target.ID {
		writeError(w, fail(422, "validation", "请选择另一家公司"))
		return
	}
	if source.Version != body.Version || target.Version != body.TargetVersion {
		writeError(w, fail(409, "version_conflict", "公司资料已变化，请刷新后重试"))
		return
	}
	user := currentUser(r).ID
	ids, err := queryIDs(r.Context(), tx, "SELECT id FROM products WHERE company_id=?", source.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	for _, id := range ids {
		p, e := loadProduct(r.Context(), tx, id, true, false)
		if e != nil {
			err = e
			break
		}
		before := productSnapshot(p)
		p.CompanyID = target.ID
		p.Company = companyName(target)
		p.Version++
		p.UpdatedAt = now()
		_, err = tx.ExecContext(r.Context(), "UPDATE products SET company_id=?,company=?,version=?,updated_at=? WHERE id=?", p.CompanyID, p.Company, p.Version, p.UpdatedAt, id)
		if err == nil {
			err = addRevision(r.Context(), tx, "product", id, p.Version, &user, "update", before, productSnapshot(p))
		}
		if err != nil {
			break
		}
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), "INSERT OR IGNORE INTO setting_idea_links(id,idea_id,entity_type,entity_id,status,created_at) SELECT lower(hex(randomblob(16))),idea_id,entity_type,?,status,created_at FROM setting_idea_links WHERE entity_type='company' AND entity_id=?", target.ID, source.ID)
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), "UPDATE companies SET deleted_at=?,updated_at=?,version=version+1 WHERE id=?", now(), now(), source.ID)
	}
	if err == nil {
		after := source
		after.Version++
		after.DeletedAt = now()
		err = addRevision(r.Context(), tx, "company", source.ID, after.Version, &user, "merge", source, after)
	}
	if err == nil {
		err = a.publish(tx, "company.merged", "company", source.ID)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		writeError(w, err)
		return
	}
	a.afterCommit()
	writeJSON(w, 200, map[string]any{"ok": true, "target_id": target.ID})
}
