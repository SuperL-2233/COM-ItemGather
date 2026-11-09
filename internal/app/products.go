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
	VariantName        string       `json:"variant_name"`
	VariantDescription string       `json:"variant_description"`
	VariantGroupID     string       `json:"variant_group_id"`
	CompanyID          string       `json:"company_id"`
	UseCategory        string       `json:"use_category"`
	ToolSettings       ToolSettings `json:"tool_settings"`
	ChineseName        string       `json:"chinese_name"`
	EnglishName        string       `json:"english_name"`
	Company            string       `json:"company"`
	Category           string       `json:"category"`
	Description        string       `json:"description"`
	DevelopmentStatus  string       `json:"development_status"`
	Attributes         []Attribute  `json:"attributes"`
}

func (a *App) itemHandler(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var kind string
		err := a.db.QueryRowContext(r.Context(), "SELECT item_kind FROM products WHERE id=?", r.PathValue("id")).Scan(&kind)
		if err != nil {
			writeError(w, ensureNoRows(err))
			return
		}
		if kind != requestItemKind(r) {
			writeError(w, fail(404, "not_found", "物品不存在"))
			return
		}
		next(w, r)
	}
}

const productSelect = `SELECT p.id,p.chinese_name,p.english_name,p.company,p.category,p.description,COALESCE(p.status_option_id,p.development_status),COALESCE((SELECT name FROM product_statuses WHERE id=COALESCE(p.status_option_id,p.development_status)),p.development_status),COALESCE((SELECT color FROM product_statuses WHERE id=COALESCE(p.status_option_id,p.development_status)),'#7c3aed'),p.created_by,p.created_at,p.updated_at,p.version,COALESCE(p.deleted_at,''),p.item_kind,COALESCE(p.company_id,''),p.use_category,p.tool_settings_json,EXISTS(SELECT 1 FROM companies c WHERE c.id=p.company_id AND c.deleted_at IS NOT NULL),(SELECT COUNT(*) FROM idea_product_links l JOIN ideas i ON i.id=l.idea_id WHERE l.product_id=p.id AND i.deleted_at IS NULL),COALESCE(p.variant_group_id,''),p.variant_name,p.variant_description,CASE WHEN p.variant_group_id IS NULL THEN 1 ELSE (SELECT COUNT(*) FROM products v WHERE v.variant_group_id=p.variant_group_id AND v.deleted_at IS NULL) END FROM products p `

func loadProduct(ctx context.Context, q querier, id string, deleted, related bool) (Product, error) {
	clause := "WHERE p.id=?"
	if !deleted {
		clause += " AND p.deleted_at IS NULL"
	}
	var item Product
	var removed, toolJSON string
	err := q.QueryRowContext(ctx, productSelect+clause, id).Scan(&item.ID, &item.ChineseName, &item.EnglishName, &item.Company, &item.Category, &item.Description, &item.DevelopmentStatus, &item.DevelopmentStatusLabel, &item.DevelopmentStatusColor, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt, &item.Version, &removed, &item.ItemKind, &item.CompanyID, &item.UseCategory, &toolJSON, &item.CompanyDeleted, &item.IdeaCount, &item.VariantGroupID, &item.VariantName, &item.VariantDescription, &item.VariantCount)
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
	if related && item.VariantGroupID != "" && item.VariantCount > 1 {
		item.Variants, err = loadVariantMembers(ctx, q, item.VariantGroupID)
		if err != nil {
			return item, err
		}
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
	return productFields{VariantGroupID: item.VariantGroupID, VariantName: item.VariantName, VariantDescription: item.VariantDescription, CompanyID: item.CompanyID, UseCategory: item.UseCategory, ToolSettings: item.ToolSettings, ChineseName: item.ChineseName, EnglishName: item.EnglishName, Company: item.Company, Category: item.Category, Description: item.Description, DevelopmentStatus: item.DevelopmentStatus, Attributes: item.Attributes}
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
		CompanyID          string         `json:"company_id"`
		UseCategory        string         `json:"use_category"`
		ToolSettings       ToolSettings   `json:"tool_settings"`
		ChineseName        string         `json:"chinese_name"`
		EnglishName        string         `json:"english_name"`
		Company            string         `json:"company"`
		Category           string         `json:"category"`
		Description        string         `json:"description"`
		DevelopmentStatus  string         `json:"development_status"`
		Attributes         []Attribute    `json:"attributes"`
		IdeaID             string         `json:"idea_id"`
		VariantName        string         `json:"variant_name"`
		VariantDescription string         `json:"variant_description"`
		Variants           []variantInput `json:"variants"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	if len(body.Variants) > 100 || (requestItemKind(r) != "product" && len(body.Variants) > 0) {
		writeError(w, fail(422, "validation", "仅商品支持变体，每个商品最多 100 款"))
		return
	}
	if len(body.Variants) > 0 {
		body.VariantName, body.VariantDescription = body.Variants[0].Name, body.Variants[0].Description
		if strings.TrimSpace(body.VariantName) == "" {
			writeError(w, fail(422, "validation", "请填写口味／样式名称"))
			return
		}
		if body.Variants[0].Status != "" {
			body.DevelopmentStatus = body.Variants[0].Status
		}
	}
	attrs, err := validAttributes(body.Attributes)
	if err != nil {
		writeError(w, err)
		return
	}
	if len(body.Description) > 100000 {
		writeError(w, fail(413, "too_large", "物品描述过长"))
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
	body.DevelopmentStatus = statusID
	body.Category, err = ensureItemCategory(r.Context(), tx, body.Category, requestItemKind(r))
	if err != nil {
		writeError(w, err)
		return
	}
	companyID, companyName, err := resolveItemCompany(r.Context(), tx, requestItemKind(r), body.CompanyID, body.Company, user.ID)
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
	variant := Product{ID: id, ItemKind: requestItemKind(r), VariantName: strings.TrimSpace(body.VariantName), VariantDescription: body.VariantDescription}
	if variant.ItemKind == "product" {
		variant.VariantGroupID = id
		if _, err = tx.ExecContext(r.Context(), "INSERT INTO product_variant_groups(id) VALUES(?)", id); err != nil {
			writeError(w, err)
			return
		}
	}
	if err = validateVariant(r.Context(), tx, variant); err != nil {
		writeError(w, err)
		return
	}
	_, err = tx.ExecContext(r.Context(), "INSERT INTO products(id,chinese_name,english_name,company,category,description,development_status,created_by,created_at,updated_at,company_id,item_kind,use_category,tool_settings_json,status_option_id,variant_group_id,variant_name,variant_description) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)", id, clean(body.ChineseName, 160), clean(body.EnglishName, 160), companyName, clean(body.Category, 100), body.Description, legacyStatus, user.ID, timestamp, timestamp, nullString(companyID), requestItemKind(r), clean(body.UseCategory, 80), encodeJSON(body.ToolSettings), statusID, nullString(variant.VariantGroupID), variant.VariantName, variant.VariantDescription)
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
	item := Product{VariantGroupID: variant.VariantGroupID, VariantName: variant.VariantName, VariantDescription: variant.VariantDescription, ID: id, CompanyID: companyID, ItemKind: requestItemKind(r), UseCategory: clean(body.UseCategory, 80), ToolSettings: body.ToolSettings, ChineseName: clean(body.ChineseName, 160), EnglishName: clean(body.EnglishName, 160), Company: companyName, Category: clean(body.Category, 100), Description: body.Description, DevelopmentStatus: body.DevelopmentStatus, CreatedBy: user.ID, CreatedAt: timestamp, UpdatedAt: timestamp, Version: 1, Attributes: attrs}
	if err = addRevision(r.Context(), tx, "product", id, 1, &user.ID, "create", nil, productSnapshot(item)); err != nil {
		writeError(w, err)
		return
	}
	for _, input := range body.Variants[min(1, len(body.Variants)):] {
		if _, err = a.insertProductVariant(r.Context(), tx, item, input, user.ID); err != nil {
			writeError(w, err)
			return
		}
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
	kind := requestItemKind(r)
	table := categoryTable(itemCategoryKind(kind))
	result := map[string]any{"companies": []string{}, "categories": []string{}, "attributes": []string{}}
	queries := []struct {
		key   string
		query string
	}{
		{"companies", "SELECT DISTINCT company FROM products WHERE item_kind=? AND deleted_at IS NULL AND company<>'' ORDER BY company COLLATE NOCASE"},
		{"categories", "SELECT c.name FROM " + table + " c WHERE c.deleted_at IS NULL AND EXISTS(SELECT 1 FROM products p WHERE p.item_kind=? AND p.deleted_at IS NULL AND p.category=c.name COLLATE NOCASE) ORDER BY c.position,c.name COLLATE NOCASE"},
		{"attributes", "SELECT DISTINCT pa.key FROM product_attributes pa JOIN products p ON p.id=pa.product_id WHERE p.item_kind=? AND p.deleted_at IS NULL AND pa.key<>'' ORDER BY pa.key COLLATE NOCASE"},
	}
	for _, item := range queries {
		rows, err := a.db.QueryContext(r.Context(), item.query, kind)
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

func productOrder(value string) (string, error) {
	// UTC RFC3339Nano strings omit trailing fractional zeros. Removing the final
	// Z makes shorter fractions (and whole seconds) compare chronologically.
	switch value {
	case "", "created_desc":
		return "rtrim(p.created_at,'Z') DESC,p.id DESC", nil
	case "created_asc":
		return "rtrim(p.created_at,'Z') ASC,p.id ASC", nil
	case "updated_desc":
		return "rtrim(p.updated_at,'Z') DESC,p.id DESC", nil
	case "updated_asc":
		return "rtrim(p.updated_at,'Z') ASC,p.id ASC", nil
	case "name_asc":
		return "(p.chinese_name='') ASC,(p.chinese_name='' AND p.english_name='') ASC,p.chinese_name COLLATE ZH_PINYIN ASC,p.english_name COLLATE ZH_PINYIN ASC,p.id ASC", nil
	case "name_desc":
		return "(p.chinese_name='') ASC,(p.chinese_name='' AND p.english_name='') ASC,p.chinese_name COLLATE ZH_PINYIN DESC,p.english_name COLLATE ZH_PINYIN DESC,p.id DESC", nil
	default:
		return "", fail(400, "invalid_sort", "请选择有效的物品排序方式")
	}
}

func (a *App) listProducts(w http.ResponseWriter, r *http.Request) {
	parameters := r.URL.Query()
	order, err := productOrder(parameters.Get("sort"))
	if err != nil {
		writeError(w, err)
		return
	}
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
	if sources := productFilterValues(parameters["source"]); len(sources) > 0 {
		where += " AND EXISTS(SELECT 1 FROM json_each(p.tool_settings_json,'$.sources') s WHERE json_extract(s.value,'$.method') IN (" + productFilterPlaceholders(sources) + "))"
		for _, source := range sources {
			args = append(args, source)
		}
	}
	if uses := productFilterValues(parameters["use_category"]); len(uses) > 0 {
		where += " AND p.use_category COLLATE NOCASE IN (" + productFilterPlaceholders(uses) + ")"
		for _, use := range uses {
			args = append(args, use)
		}
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
		where += ` AND (p.chinese_name LIKE ? ESCAPE '\' OR p.english_name LIKE ? ESCAPE '\' OR p.company LIKE ? ESCAPE '\' OR p.category LIKE ? ESCAPE '\' OR p.description LIKE ? ESCAPE '\' OR p.use_category LIKE ? ESCAPE '\' OR p.variant_name LIKE ? ESCAPE '\' OR p.variant_description LIKE ? ESCAPE '\' OR p.tool_settings_json LIKE ? ESCAPE '\' OR EXISTS(SELECT 1 FROM companies c WHERE c.id=p.company_id AND (c.english_name LIKE ? ESCAPE '\' OR c.aliases LIKE ? ESCAPE '\')) OR EXISTS(SELECT 1 FROM product_attributes pa WHERE pa.product_id=p.id AND (pa.key LIKE ? ESCAPE '\' OR pa.value LIKE ? ESCAPE '\')))`
		for i := 0; i < 13; i++ {
			args = append(args, like)
		}
	}
	grouped := requestItemKind(r) == "product" && parameters.Get("grouped") == "1"
	countExpression := "COUNT(*)"
	if grouped {
		countExpression = "COUNT(DISTINCT COALESCE(p.variant_group_id,p.id))"
	}
	var total int
	if err = a.db.QueryRowContext(r.Context(), "SELECT "+countExpression+" FROM products p WHERE "+where, args...).Scan(&total); err != nil {
		writeError(w, err)
		return
	}
	prefix := ""
	if grouped {
		order = strings.ReplaceAll(order, "rtrim(p.updated_at,'Z')", "(SELECT MAX(rtrim(v.updated_at,'Z')) FROM products v WHERE v.deleted_at IS NULL AND COALESCE(v.variant_group_id,v.id)=COALESCE(p.variant_group_id,p.id))")
		prefix = "WITH matched AS (SELECT p.id,ROW_NUMBER() OVER(PARTITION BY COALESCE(p.variant_group_id,p.id) ORDER BY p.created_at,p.id) AS member FROM products p WHERE " + where + ") "
		where = "p.id IN (SELECT id FROM matched WHERE member=1)"
	}
	args = append(args, limit, offset)
	rows, err := a.db.QueryContext(r.Context(), prefix+productSelect+"WHERE "+where+" ORDER BY "+order+" LIMIT ? OFFSET ?", args...)
	if err != nil {
		writeError(w, err)
		return
	}
	items := []Product{}
	for rows.Next() {
		var item Product
		var removed, toolJSON string
		if err = rows.Scan(&item.ID, &item.ChineseName, &item.EnglishName, &item.Company, &item.Category, &item.Description, &item.DevelopmentStatus, &item.DevelopmentStatusLabel, &item.DevelopmentStatusColor, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt, &item.Version, &removed, &item.ItemKind, &item.CompanyID, &item.UseCategory, &toolJSON, &item.CompanyDeleted, &item.IdeaCount, &item.VariantGroupID, &item.VariantName, &item.VariantDescription, &item.VariantCount); err != nil {
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
		if grouped && items[index].VariantCount > 1 {
			items[index].Variants, err = loadVariantMembers(r.Context(), a.db, items[index].VariantGroupID)
			if err != nil {
				writeError(w, err)
				return
			}
		}
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
		Version            int           `json:"version"`
		VariantName        *string       `json:"variant_name"`
		VariantDescription *string       `json:"variant_description"`
		CompanyID          *string       `json:"company_id"`
		UseCategory        *string       `json:"use_category"`
		ToolSettings       *ToolSettings `json:"tool_settings"`
		ChineseName        *string       `json:"chinese_name"`
		EnglishName        *string       `json:"english_name"`
		Company            *string       `json:"company"`
		Category           *string       `json:"category"`
		Description        *string       `json:"description"`
		DevelopmentStatus  *string       `json:"development_status"`
		Attributes         *[]Attribute  `json:"attributes"`
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
		writeError(w, &APIError{Status: 409, Code: "version_conflict", Message: "这个物品已被其他人修改", Details: map[string]any{"current": item, "submitted": body}})
		return
	}
	before := productSnapshot(item)
	if body.VariantName != nil {
		item.VariantName = strings.TrimSpace(*body.VariantName)
	}
	if body.VariantDescription != nil {
		item.VariantDescription = *body.VariantDescription
	}
	if err = validateVariant(r.Context(), tx, item); err != nil {
		writeError(w, err)
		return
	}
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
		item.CompanyID, item.Company, err = resolveItemCompany(r.Context(), tx, item.ItemKind, companyID, companyName, user.ID)
		if err != nil {
			writeError(w, err)
			return
		}
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
		item.Category, err = ensureItemCategory(r.Context(), tx, *body.Category, item.ItemKind)
		if err != nil {
			writeError(w, err)
			return
		}
	}
	if body.Description != nil {
		if len(*body.Description) > 100000 {
			writeError(w, fail(413, "too_large", "物品描述过长"))
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
	result, err := tx.ExecContext(r.Context(), "UPDATE products SET chinese_name=?,english_name=?,company=?,category=?,description=?,development_status=?,company_id=?,use_category=?,tool_settings_json=?,status_option_id=?,variant_name=?,variant_description=?,version=?,updated_at=? WHERE id=? AND version=?", item.ChineseName, item.EnglishName, item.Company, item.Category, item.Description, legacyStatus, nullString(item.CompanyID), item.UseCategory, encodeJSON(item.ToolSettings), statusID, item.VariantName, item.VariantDescription, item.Version, item.UpdatedAt, id, body.Version)
	if err != nil {
		writeError(w, err)
		return
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		writeError(w, fail(409, "version_conflict", "物品版本已变化"))
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
	if err = a.syncProductShared(r.Context(), tx, before, item, user.ID, nil); err != nil {
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
		if err = validateVariant(r.Context(), tx, item); err != nil {
			writeError(w, err)
			return
		}
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
		writeError(w, fail(400, "not_deleted", "请先将物品移入回收站"))
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
