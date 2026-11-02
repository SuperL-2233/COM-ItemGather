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

type productFields struct {
	CompanyID         string       `json:"company_id"`
	BrandName         string       `json:"brand_name"`
	UseCategory       string       `json:"use_category"`
	ToolSettings      ToolSettings `json:"tool_settings"`
	ChineseName       string       `json:"chinese_name"`
	EnglishName       string       `json:"english_name"`
	Company           string       `json:"company"`
	Category          string       `json:"category"`
	Description       string       `json:"description"`
	DevelopmentStatus string       `json:"development_status"`
	Attributes        []Attribute  `json:"attributes"`
}

const productSelect = `SELECT p.id,p.chinese_name,p.english_name,p.company,p.category,p.description,COALESCE(p.status_option_id,p.development_status),COALESCE((SELECT name FROM product_statuses WHERE id=COALESCE(p.status_option_id,p.development_status)),p.development_status),p.created_by,p.created_at,p.updated_at,p.version,COALESCE(p.deleted_at,''),p.item_kind,COALESCE(p.company_id,''),p.brand_name,p.use_category,p.tool_settings_json,EXISTS(SELECT 1 FROM companies c WHERE c.id=p.company_id AND c.deleted_at IS NOT NULL),(SELECT COUNT(*) FROM idea_product_links l JOIN ideas i ON i.id=l.idea_id WHERE l.product_id=p.id AND i.deleted_at IS NULL) FROM products p `

func loadProduct(ctx context.Context, q querier, id string, deleted, related bool) (Product, error) {
	clause := "WHERE p.id=?"
	if !deleted {
		clause += " AND p.deleted_at IS NULL"
	}
	var item Product
	var removed, toolJSON string
	err := q.QueryRowContext(ctx, productSelect+clause, id).Scan(&item.ID, &item.ChineseName, &item.EnglishName, &item.Company, &item.Category, &item.Description, &item.DevelopmentStatus, &item.DevelopmentStatusLabel, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt, &item.Version, &removed, &item.ItemKind, &item.CompanyID, &item.BrandName, &item.UseCategory, &toolJSON, &item.CompanyDeleted, &item.IdeaCount)
	if err != nil {
		return item, ensureNoRows(err)
	}
	item.DeletedAt = removed
	_ = json.Unmarshal([]byte(toolJSON), &item.ToolSettings)
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
	return productFields{CompanyID: item.CompanyID, BrandName: item.BrandName, UseCategory: item.UseCategory, ToolSettings: item.ToolSettings, ChineseName: item.ChineseName, EnglishName: item.EnglishName, Company: item.Company, Category: item.Category, Description: item.Description, DevelopmentStatus: item.DevelopmentStatus, Attributes: item.Attributes}
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
		CompanyID         string       `json:"company_id"`
		BrandName         string       `json:"brand_name"`
		UseCategory       string       `json:"use_category"`
		ToolSettings      ToolSettings `json:"tool_settings"`
		ChineseName       string       `json:"chinese_name"`
		EnglishName       string       `json:"english_name"`
		Company           string       `json:"company"`
		Category          string       `json:"category"`
		Description       string       `json:"description"`
		DevelopmentStatus string       `json:"development_status"`
		Attributes        []Attribute  `json:"attributes"`
		IdeaID            string       `json:"idea_id"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	if body.DevelopmentStatus == "" {
		body.DevelopmentStatus = "concept"
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
	statusID, legacyStatus, _, err := resolveProductStatus(r.Context(), tx, body.DevelopmentStatus)
	if err != nil {
		writeError(w, err)
		return
	}
	body.Category, err = ensureProductCategory(r.Context(), tx, body.Category)
	if err != nil {
		writeError(w, err)
		return
	}
	companyID, companyName, err := resolveCompany(r.Context(), tx, body.CompanyID, body.Company, user.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	if err = validateToolSettings(body.ToolSettings); err != nil {
		writeError(w, err)
		return
	}
	if err = ensureUseCategory(r.Context(), tx, body.UseCategory); err != nil {
		writeError(w, err)
		return
	}
	_, err = tx.ExecContext(r.Context(), "INSERT INTO products(id,chinese_name,english_name,company,category,description,development_status,created_by,created_at,updated_at,company_id,brand_name,item_kind,use_category,tool_settings_json,status_option_id) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)", id, clean(body.ChineseName, 160), clean(body.EnglishName, 160), companyName, clean(body.Category, 100), body.Description, legacyStatus, user.ID, timestamp, timestamp, nullString(companyID), clean(body.BrandName, 160), requestItemKind(r), clean(body.UseCategory, 80), encodeJSON(body.ToolSettings), statusID)
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
	item := Product{ID: id, CompanyID: companyID, BrandName: clean(body.BrandName, 160), ItemKind: requestItemKind(r), UseCategory: clean(body.UseCategory, 80), ToolSettings: body.ToolSettings, ChineseName: clean(body.ChineseName, 160), EnglishName: clean(body.EnglishName, 160), Company: companyName, Category: clean(body.Category, 100), Description: body.Description, DevelopmentStatus: body.DevelopmentStatus, CreatedBy: user.ID, CreatedAt: timestamp, UpdatedAt: timestamp, Version: 1, Attributes: attrs}
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

func (a *App) listProductFilters(w http.ResponseWriter, r *http.Request) {
	result := map[string]any{"companies": []string{}, "categories": []string{}, "attributes": []string{}}
	queries := []struct {
		key   string
		query string
	}{
		{"companies", "SELECT DISTINCT company FROM products WHERE item_kind='product' AND deleted_at IS NULL AND company<>'' ORDER BY company COLLATE NOCASE"},
		{"categories", "SELECT DISTINCT category FROM products WHERE item_kind='product' AND deleted_at IS NULL AND category<>'' ORDER BY category COLLATE NOCASE"},
		{"attributes", "SELECT DISTINCT pa.key FROM product_attributes pa JOIN products p ON p.id=pa.product_id WHERE p.item_kind='product' AND p.deleted_at IS NULL AND pa.key<>'' ORDER BY pa.key COLLATE NOCASE"},
	}
	for _, item := range queries {
		rows, err := a.db.QueryContext(r.Context(), item.query)
		if err != nil {
			writeError(w, err)
			return
		}
		values := []string{}
		for rows.Next() {
			var value string
			if err = rows.Scan(&value); err != nil {
				rows.Close()
				writeError(w, err)
				return
			}
			values = append(values, value)
		}
		if err = rows.Err(); err != nil {
			rows.Close()
			writeError(w, err)
			return
		}
		rows.Close()
		result[item.key] = values
	}
	options, err := loadProductOptions(r.Context(), a.db, "statuses")
	if err != nil {
		writeError(w, err)
		return
	}
	result["statuses"] = options
	writeJSON(w, 200, result)
}

func productFilterValues(values []string) []string {
	result := []string{}
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func productFilterPlaceholders(values []string) string {
	return strings.TrimSuffix(strings.Repeat("?,", len(values)), ",")
}

func (a *App) listProducts(w http.ResponseWriter, r *http.Request) {
	parameters := r.URL.Query()
	query := strings.TrimSpace(parameters.Get("q"))
	companies := productFilterValues(parameters["company"])
	categories := productFilterValues(parameters["category"])
	attributes := productFilterValues(parameters["attribute"])
	statuses := productFilterValues(parameters["status"])
	limit, _ := strconv.Atoi(parameters.Get("limit"))
	offset, _ := strconv.Atoi(parameters.Get("offset"))
	if limit < 1 || limit > 100 {
		limit = 30
	}
	if offset < 0 {
		offset = 0
	}
	where := "p.deleted_at IS NULL AND p.item_kind=?"
	args := []any{requestItemKind(r)}
	if companyID := parameters.Get("company_id"); companyID != "" {
		where += " AND p.company_id=?"
		args = append(args, companyID)
	}
	if source := parameters.Get("source"); source != "" {
		where += " AND EXISTS(SELECT 1 FROM json_each(p.tool_settings_json,'$.sources') s WHERE json_extract(s.value,'$.method')=?)"
		args = append(args, source)
	}
	if use := parameters.Get("use_category"); use != "" {
		where += " AND p.use_category=? COLLATE NOCASE"
		args = append(args, use)
	}
	if len(companies) > 0 {
		where += " AND p.company IN (" + productFilterPlaceholders(companies) + ")"
		for _, company := range companies {
			args = append(args, company)
		}
	}
	if len(categories) > 0 {
		where += " AND p.category IN (" + productFilterPlaceholders(categories) + ")"
		for _, category := range categories {
			args = append(args, category)
		}
	}
	if len(attributes) > 0 {
		where += " AND EXISTS(SELECT 1 FROM product_attributes pf WHERE pf.product_id=p.id AND pf.key IN (" + productFilterPlaceholders(attributes) + "))"
		for _, attribute := range attributes {
			args = append(args, attribute)
		}
	}
	if len(statuses) > 0 {
		for _, status := range statuses {
			if _, _, _, err := resolveProductStatus(r.Context(), a.db, status); err != nil {
				writeError(w, err)
				return
			}
		}
		where += " AND COALESCE(p.status_option_id,p.development_status) IN (" + productFilterPlaceholders(statuses) + ")"
		for _, status := range statuses {
			args = append(args, status)
		}
	}
	if query != "" {
		like := "%" + escapeLike(query) + "%"
		where += ` AND (p.chinese_name LIKE ? ESCAPE '\' OR p.english_name LIKE ? ESCAPE '\' OR p.company LIKE ? ESCAPE '\' OR p.category LIKE ? ESCAPE '\' OR p.description LIKE ? ESCAPE '\' OR p.brand_name LIKE ? ESCAPE '\' OR p.use_category LIKE ? ESCAPE '\' OR p.tool_settings_json LIKE ? ESCAPE '\' OR EXISTS(SELECT 1 FROM companies c WHERE c.id=p.company_id AND (c.english_name LIKE ? ESCAPE '\' OR c.aliases LIKE ? ESCAPE '\')) OR EXISTS(SELECT 1 FROM product_attributes pa WHERE pa.product_id=p.id AND (pa.key LIKE ? ESCAPE '\' OR pa.value LIKE ? ESCAPE '\')))`
		for i := 0; i < 12; i++ {
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
		var removed, toolJSON string
		if err = rows.Scan(&item.ID, &item.ChineseName, &item.EnglishName, &item.Company, &item.Category, &item.Description, &item.DevelopmentStatus, &item.DevelopmentStatusLabel, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt, &item.Version, &removed, &item.ItemKind, &item.CompanyID, &item.BrandName, &item.UseCategory, &toolJSON, &item.CompanyDeleted, &item.IdeaCount); err != nil {
			rows.Close()
			writeError(w, err)
			return
		}
		item.Attachments = []Attachment{}
		_ = json.Unmarshal([]byte(toolJSON), &item.ToolSettings)
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
		Version           int           `json:"version"`
		CompanyID         *string       `json:"company_id"`
		BrandName         *string       `json:"brand_name"`
		UseCategory       *string       `json:"use_category"`
		ToolSettings      *ToolSettings `json:"tool_settings"`
		ChineseName       *string       `json:"chinese_name"`
		EnglishName       *string       `json:"english_name"`
		Company           *string       `json:"company"`
		Category          *string       `json:"category"`
		Description       *string       `json:"description"`
		DevelopmentStatus *string       `json:"development_status"`
		Attributes        *[]Attribute  `json:"attributes"`
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
	if body.ChineseName != nil {
		item.ChineseName = clean(*body.ChineseName, 160)
	}
	if body.EnglishName != nil {
		item.EnglishName = clean(*body.EnglishName, 160)
	}
	if body.CompanyID != nil || body.Company != nil {
		companyID, companyName := "", ""
		if body.CompanyID != nil {
			companyID = *body.CompanyID
		} else if body.Company != nil {
			companyName = *body.Company
		}
		item.CompanyID, item.Company, err = resolveCompany(r.Context(), tx, companyID, companyName, user.ID)
		if err != nil {
			writeError(w, err)
			return
		}
	}
	if body.BrandName != nil {
		item.BrandName = clean(*body.BrandName, 160)
	}
	if body.UseCategory != nil {
		if err = ensureUseCategory(r.Context(), tx, *body.UseCategory); err != nil {
			writeError(w, err)
			return
		}
		item.UseCategory = clean(*body.UseCategory, 80)
	}
	if body.ToolSettings != nil {
		if err = validateToolSettings(*body.ToolSettings); err != nil {
			writeError(w, err)
			return
		}
		item.ToolSettings = *body.ToolSettings
	}

	if body.Category != nil {
		item.Category, err = ensureProductCategory(r.Context(), tx, *body.Category)
		if err != nil {
			writeError(w, err)
			return
		}
	}
	if body.Description != nil {
		if len(*body.Description) > 100000 {
			writeError(w, fail(413, "too_large", "商品描述过长"))
			return
		}
		item.Description = *body.Description
	}
	if body.DevelopmentStatus != nil {
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
	statusID, legacyStatus, statusLabel, err := resolveProductStatus(r.Context(), tx, item.DevelopmentStatus)
	if err != nil {
		writeError(w, err)
		return
	}
	item.DevelopmentStatusLabel = statusLabel
	item.Version++
	item.UpdatedAt = now()
	result, err := tx.ExecContext(r.Context(), "UPDATE products SET chinese_name=?,english_name=?,company=?,category=?,description=?,development_status=?,company_id=?,brand_name=?,use_category=?,tool_settings_json=?,status_option_id=?,version=?,updated_at=? WHERE id=? AND version=?", item.ChineseName, item.EnglishName, item.Company, item.Category, item.Description, legacyStatus, nullString(item.CompanyID), item.BrandName, item.UseCategory, encodeJSON(item.ToolSettings), statusID, item.Version, item.UpdatedAt, id, body.Version)
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
	var references int
	if err = tx.QueryRowContext(r.Context(), "SELECT (SELECT COUNT(*) FROM recipe_groups WHERE output_id=?)+(SELECT COUNT(*) FROM recipe_ingredients WHERE product_id=?)", id, id).Scan(&references); err != nil {
		writeError(w, err)
		return
	}
	if references > 0 {
		writeError(w, fail(409, "item_in_use", "该物品仍被合成配方引用，请先移除引用；也可以保留在回收站"))
		return
	}

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
