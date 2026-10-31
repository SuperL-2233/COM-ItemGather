package app

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"com-itemgather/internal/security"
)

type productFields struct {
	InternalName      string      `json:"internal_name"`
	FinalBrandName    string      `json:"final_brand_name"`
	Category          string      `json:"category"`
	Description       string      `json:"description"`
	DevelopmentStatus string      `json:"development_status"`
	Attributes        []Attribute `json:"attributes"`
}

const productSelect = `SELECT p.id,p.internal_name,p.final_brand_name,p.category,p.description,p.development_status,p.created_by,p.created_at,p.updated_at,p.version,COALESCE(p.deleted_at,''),(SELECT COUNT(*) FROM idea_product_links l JOIN ideas i ON i.id=l.idea_id WHERE l.product_id=p.id AND i.deleted_at IS NULL) FROM products p `

func loadProduct(ctx context.Context, q querier, id string, deleted, related bool) (Product, error) {
	clause := "WHERE p.id=?"
	if !deleted {
		clause += " AND p.deleted_at IS NULL"
	}
	var item Product
	var removed string
	err := q.QueryRowContext(ctx, productSelect+clause, id).Scan(&item.ID, &item.InternalName, &item.FinalBrandName, &item.Category, &item.Description, &item.DevelopmentStatus, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt, &item.Version, &removed, &item.IdeaCount)
	if err != nil {
		return item, ensureNoRows(err)
	}
	item.DeletedAt = removed
	item.Attributes = []Attribute{}
	item.Attachments = []Attachment{}
	rows, err := q.QueryContext(ctx, "SELECT id,key,value,certainty FROM product_attributes WHERE product_id=? ORDER BY position,id", id)
	if err != nil {
		return item, err
	}
	defer rows.Close()
	for rows.Next() {
		var attr Attribute
		rows.Scan(&attr.ID, &attr.Key, &attr.Value, &attr.Certainty)
		item.Attributes = append(item.Attributes, attr)
	}
	item.Attachments, err = loadAttachments(ctx, q, "product", id, -1)
	if err != nil {
		return item, err
	}
	if related {
		item.Links = []Link{}
		links, err := q.QueryContext(ctx, `SELECT l.id,l.idea_id,l.product_id,l.status,COALESCE(NULLIF(i.display_title,''),substr(i.content,1,80)),l.created_at,l.updated_at FROM idea_product_links l JOIN ideas i ON i.id=l.idea_id WHERE l.product_id=? AND i.deleted_at IS NULL ORDER BY l.created_at DESC`, id)
		if err != nil {
			return item, err
		}
		defer links.Close()
		for links.Next() {
			var link Link
			links.Scan(&link.ID, &link.IdeaID, &link.ProductID, &link.Status, &link.IdeaTitle, &link.CreatedAt, &link.UpdatedAt)
			item.Links = append(item.Links, link)
		}
	}
	return item, nil
}
func productSnapshot(item Product) productFields {
	return productFields{InternalName: item.InternalName, FinalBrandName: item.FinalBrandName, Category: item.Category, Description: item.Description, DevelopmentStatus: item.DevelopmentStatus, Attributes: item.Attributes}
}
func validProductStatus(status string) bool {
	return status == "concept" || status == "design" || status == "production" || status == "completed"
}
func replaceProductAttributes(ctx context.Context, tx *sql.Tx, id string, attrs []Attribute) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM product_attributes WHERE product_id=?", id); err != nil {
		return err
	}
	for index, attr := range attrs {
		if _, err := tx.ExecContext(ctx, "INSERT INTO product_attributes(id,product_id,key,value,certainty,position) VALUES(?,?,?,?,?,?)", security.ID(), id, attr.Key, attr.Value, attr.Certainty, index); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) createProduct(w http.ResponseWriter, r *http.Request) {
	var body struct {
		InternalName      string      `json:"internal_name"`
		FinalBrandName    string      `json:"final_brand_name"`
		Category          string      `json:"category"`
		Description       string      `json:"description"`
		DevelopmentStatus string      `json:"development_status"`
		Attributes        []Attribute `json:"attributes"`
		IdeaID            string      `json:"idea_id"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	body.InternalName = clean(body.InternalName, 160)
	if body.InternalName == "" {
		writeError(w, fail(422, "validation", "请填写内部工作名称"))
		return
	}
	if body.DevelopmentStatus == "" {
		body.DevelopmentStatus = "concept"
	}
	if !validProductStatus(body.DevelopmentStatus) {
		writeError(w, fail(422, "validation", "制作状态无效"))
		return
	}
	attrs, err := validAttributes(body.Attributes)
	if err != nil {
		writeError(w, err)
		return
	}
	if len(body.Description) > 100000 {
		writeError(w, fail(413, "too_large", "商品描述过长"))
		return
	}
	user := currentUser(r)
	id := security.ID()
	timestamp := now()
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(r.Context(), "INSERT INTO products(id,internal_name,final_brand_name,category,description,development_status,created_by,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)", id, body.InternalName, clean(body.FinalBrandName, 160), clean(body.Category, 100), body.Description, body.DevelopmentStatus, user.ID, timestamp, timestamp)
	if err != nil {
		writeError(w, err)
		return
	}
	if err = replaceProductAttributes(r.Context(), tx, id, attrs); err != nil {
		writeError(w, err)
		return
	}
	if body.IdeaID != "" {
		var exists int
		tx.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM ideas WHERE id=? AND deleted_at IS NULL", body.IdeaID).Scan(&exists)
		if exists != 1 {
			writeError(w, fail(404, "not_found", "要关联的灵感不存在"))
			return
		}
		if _, err = tx.ExecContext(r.Context(), "INSERT INTO idea_product_links(id,idea_id,product_id,status,created_by,created_at,updated_at) VALUES(?,?,?,?,?,?,?)", security.ID(), body.IdeaID, id, "candidate", user.ID, timestamp, timestamp); err != nil {
			writeError(w, err)
			return
		}
		a.publish(tx, "link.created", "idea", body.IdeaID)
	}
	item := Product{ID: id, InternalName: body.InternalName, FinalBrandName: clean(body.FinalBrandName, 160), Category: clean(body.Category, 100), Description: body.Description, DevelopmentStatus: body.DevelopmentStatus, CreatedBy: user.ID, CreatedAt: timestamp, UpdatedAt: timestamp, Version: 1, Attributes: attrs}
	if err = addRevision(r.Context(), tx, "product", id, 1, &user.ID, "create", nil, productSnapshot(item)); err != nil {
		writeError(w, err)
		return
	}
	a.publish(tx, "product.created", "product", id)
	if err = tx.Commit(); err != nil {
		writeError(w, err)
		return
	}
	a.afterCommit()
	item, err = loadProduct(r.Context(), a.db, id, true, true)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 201, item)
}

func (a *App) listProducts(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	status := r.URL.Query().Get("status")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit < 1 || limit > 100 {
		limit = 30
	}
	if offset < 0 {
		offset = 0
	}
	where := "p.deleted_at IS NULL"
	args := []any{}
	if status != "" {
		if !validProductStatus(status) {
			writeError(w, fail(422, "validation", "制作状态无效"))
			return
		}
		where += " AND p.development_status=?"
		args = append(args, status)
	}
	if query != "" {
		like := "%" + escapeLike(query) + "%"
		where += ` AND (p.internal_name LIKE ? ESCAPE '\' OR p.final_brand_name LIKE ? ESCAPE '\' OR p.category LIKE ? ESCAPE '\' OR p.description LIKE ? ESCAPE '\' OR EXISTS(SELECT 1 FROM product_attributes pa WHERE pa.product_id=p.id AND (pa.key LIKE ? ESCAPE '\' OR pa.value LIKE ? ESCAPE '\')))`
		for i := 0; i < 6; i++ {
			args = append(args, like)
		}
	}
	var total int
	a.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM products p WHERE "+where, args...).Scan(&total)
	args = append(args, limit, offset)
	rows, err := a.db.QueryContext(r.Context(), productSelect+"WHERE "+where+" ORDER BY p.created_at DESC LIMIT ? OFFSET ?", args...)
	if err != nil {
		writeError(w, err)
		return
	}
	items := []Product{}
	for rows.Next() {
		var item Product
		var removed string
		if err = rows.Scan(&item.ID, &item.InternalName, &item.FinalBrandName, &item.Category, &item.Description, &item.DevelopmentStatus, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt, &item.Version, &removed, &item.IdeaCount); err != nil {
			rows.Close()
			writeError(w, err)
			return
		}
		item.Attachments = []Attachment{}
		items = append(items, item)
	}
	rows.Close()
	for index := range items {
		var thumbnail sql.NullString
		a.db.QueryRowContext(r.Context(), "SELECT id FROM attachments WHERE product_id=? ORDER BY created_at LIMIT 1", items[index].ID).Scan(&thumbnail)
		if thumbnail.Valid {
			items[index].Attachments = append(items[index].Attachments, Attachment{ID: thumbnail.String, URL: "/api/attachments/" + thumbnail.String, ThumbnailURL: "/api/attachments/" + thumbnail.String + "?thumbnail=1"})
		}
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": total, "limit": limit, "offset": offset})
}
func (a *App) getProduct(w http.ResponseWriter, r *http.Request) {
	item, err := loadProduct(r.Context(), a.db, r.PathValue("id"), false, true)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, item)
}

func (a *App) updateProduct(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Version           int          `json:"version"`
		InternalName      *string      `json:"internal_name"`
		FinalBrandName    *string      `json:"final_brand_name"`
		Category          *string      `json:"category"`
		Description       *string      `json:"description"`
		DevelopmentStatus *string      `json:"development_status"`
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
	item, err := loadProduct(r.Context(), tx, id, false, true)
	if err != nil {
		writeError(w, err)
		return
	}
	if item.Version != body.Version {
		writeError(w, &APIError{Status: 409, Code: "version_conflict", Message: "这个商品已被其他人修改", Details: map[string]any{"current": item, "submitted": body}})
		return
	}
	before := productSnapshot(item)
	if body.InternalName != nil {
		item.InternalName = clean(*body.InternalName, 160)
		if item.InternalName == "" {
			writeError(w, fail(422, "validation", "内部工作名称不能为空"))
			return
		}
	}
	if body.FinalBrandName != nil {
		item.FinalBrandName = clean(*body.FinalBrandName, 160)
	}
	if body.Category != nil {
		item.Category = clean(*body.Category, 100)
	}
	if body.Description != nil {
		if len(*body.Description) > 100000 {
			writeError(w, fail(413, "too_large", "商品描述过长"))
			return
		}
		item.Description = *body.Description
	}
	if body.DevelopmentStatus != nil {
		if !validProductStatus(*body.DevelopmentStatus) {
			writeError(w, fail(422, "validation", "制作状态无效"))
			return
		}
		item.DevelopmentStatus = *body.DevelopmentStatus
	}
	if body.Attributes != nil {
		attrs, attrErr := validAttributes(*body.Attributes)
		if attrErr != nil {
			writeError(w, attrErr)
			return
		}
		item.Attributes = attrs
	}
	item.Version++
	item.UpdatedAt = now()
	result, err := tx.ExecContext(r.Context(), "UPDATE products SET internal_name=?,final_brand_name=?,category=?,description=?,development_status=?,version=?,updated_at=? WHERE id=? AND version=?", item.InternalName, item.FinalBrandName, item.Category, item.Description, item.DevelopmentStatus, item.Version, item.UpdatedAt, id, body.Version)
	if err != nil {
		writeError(w, err)
		return
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		writeError(w, fail(409, "version_conflict", "商品版本已变化"))
		return
	}
	if body.Attributes != nil {
		if err = replaceProductAttributes(r.Context(), tx, id, item.Attributes); err != nil {
			writeError(w, err)
			return
		}
	}
	if err = addRevision(r.Context(), tx, "product", id, item.Version, &user.ID, "update", before, productSnapshot(item)); err != nil {
		writeError(w, err)
		return
	}
	a.publish(tx, "product.updated", "product", id)
	if err = tx.Commit(); err != nil {
		writeError(w, err)
		return
	}
	a.afterCommit()
	item, err = loadProduct(r.Context(), a.db, id, false, true)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, item)
}

func (a *App) deleteProduct(w http.ResponseWriter, r *http.Request) {
	a.changeProductDeleted(w, r, true)
}
func (a *App) restoreProduct(w http.ResponseWriter, r *http.Request) {
	a.changeProductDeleted(w, r, false)
}
func (a *App) changeProductDeleted(w http.ResponseWriter, r *http.Request, remove bool) {
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
	item, err := loadProduct(r.Context(), tx, id, true, false)
	if err != nil {
		writeError(w, err)
		return
	}
	if item.Version != body.Version {
		writeError(w, fail(409, "version_conflict", "记录版本已变化"))
		return
	}
	before := productSnapshot(item)
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
	result, err := tx.ExecContext(r.Context(), "UPDATE products SET deleted_at=?,version=?,updated_at=? WHERE id=? AND version=?", deleted, item.Version, item.UpdatedAt, id, body.Version)
	if err != nil {
		writeError(w, err)
		return
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		writeError(w, fail(409, "version_conflict", "记录版本已变化"))
		return
	}
	if err = addRevision(r.Context(), tx, "product", id, item.Version, &user.ID, action, before, productSnapshot(item)); err != nil {
		writeError(w, err)
		return
	}
	a.publish(tx, "product."+action, "product", id)
	if err = tx.Commit(); err != nil {
		writeError(w, err)
		return
	}
	a.afterCommit()
	writeJSON(w, 200, map[string]any{"ok": true, "version": item.Version})
}
func (a *App) permanentDeleteProduct(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Version int `json:"version"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	unlock, err := a.lockAttachments(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	defer unlock()
	id := r.PathValue("id")
	item, err := loadProduct(r.Context(), a.db, id, true, false)
	if err != nil {
		writeError(w, err)
		return
	}
	if item.DeletedAt == "" {
		writeError(w, fail(400, "not_deleted", "请先将商品移入回收站"))
		return
	}
	if item.Version != body.Version {
		writeError(w, fail(409, "version_conflict", "记录版本已变化"))
		return
	}
	paths := a.attachmentPaths(r.Context(), a.db, "product", id)
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(r.Context(), "DELETE FROM products WHERE id=? AND version=? AND deleted_at IS NOT NULL", id, body.Version)
	if err == nil {
		count, _ := result.RowsAffected()
		if count != 1 {
			writeError(w, fail(409, "version_conflict", "记录版本已变化"))
			return
		}
		err = a.publish(tx, "product.deleted_permanently", "product", id)
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
