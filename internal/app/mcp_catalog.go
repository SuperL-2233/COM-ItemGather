package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
)

var mcpKinds = []string{"idea", "company", "product", "tool", "other", "recipe"}

// This index contains only active business records. In particular, deleted
// company/ingredient names cannot become searchable through a live product or
// recipe. Detail hydration uses explicit DTOs rather than export sanitization.
const mcpRecordIndex = `WITH records AS (
 SELECT 'idea' kind,i.id,COALESCE(NULLIF(i.display_title,''),NULLIF(substr(i.content,1,60),''),'未命名灵感') title,
 substr(COALESCE(NULLIF(i.summary,''),i.content),1,240) summary,i.updated_at,'' category,'' status,'' company_id,
 i.original_content||' '||i.content||' '||i.display_title||' '||i.summary||' '||i.notes||' '||i.idea_types_json||' '||i.product_directions_json search_text,'' group_id
 FROM ideas i WHERE i.deleted_at IS NULL
 UNION ALL
 SELECT p.item_kind,p.id,COALESCE(NULLIF(p.chinese_name,''),NULLIF(p.english_name,''),'未命名物品')||CASE WHEN p.variant_name='' THEN '' ELSE ' · '||p.variant_name END,substr(p.description,1,240),p.updated_at,p.category,COALESCE(p.status_option_id,p.development_status),COALESCE(c.id,''),
 p.chinese_name||' '||p.english_name||' '||p.variant_name||' '||p.variant_description||' '||p.description||' '||p.category||' '||p.use_category||' '||p.tool_settings_json||' '||COALESCE(c.chinese_name||' '||c.english_name||' '||c.aliases,'')||' '||
 COALESCE((SELECT group_concat(a.key||' '||a.value,' ') FROM product_attributes a WHERE a.product_id=p.id),''),''
 FROM products p LEFT JOIN companies c ON c.id=p.company_id AND c.deleted_at IS NULL WHERE p.deleted_at IS NULL
 UNION ALL
 SELECT 'company',c.id,COALESCE(NULLIF(c.chinese_name,''),NULLIF(c.english_name,''),'未命名公司'),substr(c.description,1,240),c.updated_at,'','','',
 c.chinese_name||' '||c.english_name||' '||c.aliases||' '||c.description,'' FROM companies c WHERE c.deleted_at IS NULL
 UNION ALL
 SELECT 'recipe',r.id,COALESCE(NULLIF(r.name,''),'未命名配方'),substr(r.notes,1,240),r.updated_at,'',COALESCE(r.status_option_id,r.development_status),'',
 r.name||' '||r.notes||' '||COALESCE(p.chinese_name||' '||p.english_name||' '||p.variant_name,'')||' '||
 COALESCE((SELECT group_concat(CASE WHEN ri.product_id IS NULL THEN ri.category ELSE COALESCE(m.chinese_name||' '||m.english_name||' '||m.variant_name,'') END,' ')
 FROM recipe_ingredients ri LEFT JOIN products m ON m.id=ri.product_id AND m.deleted_at IS NULL WHERE ri.recipe_id=r.id),''),r.group_id
 FROM recipes r JOIN recipe_groups g ON g.id=r.group_id AND g.deleted_at IS NULL
 LEFT JOIN products p ON p.id=g.output_id AND p.deleted_at IS NULL WHERE r.deleted_at IS NULL
), edges AS (
 SELECT 'idea:'||l.idea_id source,p.item_kind||':'||l.product_id target,l.status relation FROM idea_product_links l JOIN products p ON p.id=l.product_id
 UNION ALL SELECT 'idea:'||idea_id,entity_type||':'||entity_id,status FROM setting_idea_links
 UNION ALL SELECT 'product:'||a.id,'product:'||b.id,'product_variant' FROM products a JOIN products b ON b.variant_group_id=a.variant_group_id AND b.id<>a.id WHERE a.variant_group_id IS NOT NULL
 UNION ALL SELECT 'company:'||company_id,item_kind||':'||id,'company_item' FROM products WHERE company_id IS NOT NULL
 UNION ALL SELECT 'recipe:'||r.id,p.item_kind||':'||p.id,'recipe_output' FROM recipes r JOIN recipe_groups g ON g.id=r.group_id JOIN products p ON p.id=g.output_id
 UNION ALL SELECT 'recipe:'||ri.recipe_id,p.item_kind||':'||p.id,'recipe_ingredient' FROM recipe_ingredients ri JOIN products p ON p.id=ri.product_id
) `

type mcpListArgs struct {
	Query     string   `json:"query,omitempty"`
	Kinds     []string `json:"kinds,omitempty"`
	Limit     int      `json:"limit,omitempty"`
	Cursor    string   `json:"cursor,omitempty"`
	Category  string   `json:"category,omitempty"`
	Status    string   `json:"status,omitempty"`
	CompanyID string   `json:"company_id,omitempty"`
	RelatedTo string   `json:"related_to,omitempty"`
}

type mcpSummary struct {
	ID        string   `json:"id"`
	Kind      string   `json:"kind"`
	Title     string   `json:"title"`
	Summary   string   `json:"summary"`
	URL       string   `json:"url"`
	UpdatedAt string   `json:"updated_at"`
	Relations []string `json:"relations,omitempty"`
	GroupID   string   `json:"-"`
	LocalID   string   `json:"-"`
}

type mcpPage struct {
	Results    []mcpSummary `json:"results"`
	NextCursor string       `json:"next_cursor,omitempty"`
	Limit      int          `json:"limit"`
}

type mcpCursor struct {
	UpdatedAt string `json:"updated_at"`
	Kind      string `json:"kind"`
	ID        string `json:"id"`
	Filter    string `json:"filter"`
}

func decodeMCPArguments(raw json.RawMessage, target any) error {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || decoder.Decode(target) != nil || decoder.Decode(new(any)) != io.EOF {
		return fail(422, "validation", "工具参数格式或字段无效")
	}
	return nil
}

func parseMCPRecordID(key string) (string, string, error) {
	kind, id, ok := strings.Cut(key, ":")
	if !ok || !slices.Contains(mcpKinds, kind) || id == "" || len(key) > 200 || strings.ContainsAny(id, ":/\\ \t\r\n") {
		return "", "", fail(422, "validation", "记录 ID 应为 kind:id，请使用搜索或列表返回的 ID")
	}
	return kind, id, nil
}

func (a *App) mcpSummaryURL(item mcpSummary) string {
	path := map[string]string{"idea": "ideas", "company": "companies", "product": "products", "tool": "tools", "other": "other-items", "recipe": "recipes"}[item.Kind]
	id := item.LocalID
	if item.Kind == "recipe" {
		id = item.GroupID
	}
	return a.oauthIssuer() + "/" + path + "/" + id
}

func (a *App) mcpListRecords(ctx context.Context, input mcpListArgs) (mcpPage, error) {
	return a.mcpQueryRecords(ctx, a.db, input)
}

func (a *App) mcpQueryRecords(ctx context.Context, q querier, input mcpListArgs) (mcpPage, error) {
	page := mcpPage{Results: []mcpSummary{}, Limit: input.Limit}
	if input.Limit == 0 {
		input.Limit = 20
		page.Limit = 20
	}
	input.Query = strings.TrimSpace(input.Query)
	if input.Limit < 1 || input.Limit > 50 || len(input.Kinds) > len(mcpKinds) || len([]rune(input.Query)) > 200 || len(input.Cursor) > 2048 ||
		len([]rune(input.Category)) > 100 || len(input.Status) > 100 || len(input.CompanyID) > 100 {
		return page, fail(422, "validation", "查询条件无效，每页最多 50 条")
	}
	for _, kind := range input.Kinds {
		if !slices.Contains(mcpKinds, kind) {
			return page, fail(422, "validation", "记录类型无效")
		}
	}
	if input.RelatedTo != "" {
		if _, _, err := parseMCPRecordID(input.RelatedTo); err != nil {
			return page, err
		}
	}
	filter := input
	filter.Cursor, filter.Limit = "", 0
	filter.Kinds = slices.Clone(filter.Kinds)
	slices.Sort(filter.Kinds)
	digest := sha256.Sum256([]byte(encodeJSON(filter)))
	fingerprint := hex.EncodeToString(digest[:])
	values := []any{}
	relationColumn := "''"
	if input.RelatedTo != "" {
		relationColumn = `COALESCE((SELECT group_concat(DISTINCT e.relation) FROM edges e WHERE (e.source=? AND e.target=records.kind||':'||records.id) OR (e.target=? AND e.source=records.kind||':'||records.id)),'')`
		values = append(values, input.RelatedTo, input.RelatedTo)
	}
	query := mcpRecordIndex + `SELECT kind,id,title,summary,updated_at,group_id,` + relationColumn + ` FROM records WHERE 1=1`
	if input.Query != "" {
		query += ` AND search_text LIKE ? ESCAPE '\'`
		values = append(values, "%"+escapeLike(input.Query)+"%")
	}
	query, values = searchIn(query, values, "kind", input.Kinds)
	for _, condition := range []struct{ column, value string }{{"category", input.Category}, {"status", input.Status}, {"company_id", input.CompanyID}} {
		if condition.value != "" {
			query += " AND " + condition.column + "=?"
			values = append(values, condition.value)
		}
	}
	if input.RelatedTo != "" {
		// Both endpoints must exist in the active index, including the starting
		// record, so passing a deleted ID cannot reveal its old relationships.
		query += ` AND EXISTS(SELECT 1 FROM records origin WHERE origin.kind||':'||origin.id=?)
		AND EXISTS(SELECT 1 FROM edges e WHERE (e.source=? AND e.target=records.kind||':'||records.id) OR (e.target=? AND e.source=records.kind||':'||records.id))`
		values = append(values, input.RelatedTo, input.RelatedTo, input.RelatedTo)
	}
	if input.Cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(input.Cursor)
		var cursor mcpCursor
		if err != nil || json.Unmarshal(raw, &cursor) != nil || cursor.Filter != fingerprint || cursor.UpdatedAt == "" || cursor.ID == "" || !slices.Contains(mcpKinds, cursor.Kind) {
			return page, fail(422, "validation", "分页游标无效，请保持查询条件一致或重新开始")
		}
		query += " AND (updated_at<? OR (updated_at=? AND (kind>? OR (kind=? AND id>?))))"
		values = append(values, cursor.UpdatedAt, cursor.UpdatedAt, cursor.Kind, cursor.Kind, cursor.ID)
	}
	query += " ORDER BY updated_at DESC,kind,id LIMIT ?"
	values = append(values, input.Limit+1)
	rows, err := q.QueryContext(ctx, query, values...)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var item mcpSummary
		var relations string
		if err := rows.Scan(&item.Kind, &item.LocalID, &item.Title, &item.Summary, &item.UpdatedAt, &item.GroupID, &relations); err != nil {
			return page, err
		}
		if relations != "" {
			item.Relations = strings.Split(relations, ",")
		}
		item.ID = item.Kind + ":" + item.LocalID
		item.URL = a.mcpSummaryURL(item)
		page.Results = append(page.Results, item)
	}
	if err := rows.Err(); err != nil {
		return page, err
	}
	if len(page.Results) > input.Limit {
		last := page.Results[input.Limit-1]
		page.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(encodeJSON(mcpCursor{UpdatedAt: last.UpdatedAt, Kind: last.Kind, ID: last.LocalID, Filter: fingerprint})))
		page.Results = page.Results[:input.Limit]
	}
	return page, nil
}

func mcpAttachments(items []Attachment) []map[string]any {
	result := []map[string]any{}
	for _, item := range items {
		result = append(result, map[string]any{"id": item.ID, "name": item.OriginalName, "mime_type": item.MimeType, "size": item.Size, "created_at": item.CreatedAt})
	}
	return result
}

func mcpAttributes(items []Attribute) []map[string]string {
	result := []map[string]string{}
	for _, item := range items {
		result = append(result, map[string]string{"key": item.Key, "value": item.Value, "certainty": item.Certainty})
	}
	return result
}

func mcpProductFields(item Product) map[string]any {
	data := map[string]any{"variant_group_id": item.VariantGroupID, "variant_name": item.VariantName, "variant_description": item.VariantDescription, "variant_count": item.VariantCount, "chinese_name": item.ChineseName, "english_name": item.EnglishName, "category": item.Category, "description": item.Description,
		"attributes": mcpAttributes(item.Attributes), "attachments": mcpAttachments(item.Attachments), "development_status": item.DevelopmentStatus,
		"development_status_label": item.DevelopmentStatusLabel, "created_at": item.CreatedAt, "updated_at": item.UpdatedAt, "version": item.Version}
	if item.CompanyID != "" && !item.CompanyDeleted {
		data["company_id"], data["company_name"] = item.CompanyID, item.Company
	}
	if item.ItemKind == "tool" {
		data["use_category"], data["tool_settings"] = item.UseCategory, item.ToolSettings
	}
	return data
}

func (a *App) mcpFetch(ctx context.Context, key string) (map[string]any, error) {
	kind, id, err := parseMCPRecordID(key)
	if err != nil {
		return nil, err
	}
	tx, err := a.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var summary mcpSummary
	summary.Kind, summary.LocalID = kind, id
	err = tx.QueryRowContext(ctx, mcpRecordIndex+"SELECT title,group_id FROM records WHERE kind=? AND id=?", kind, id).Scan(&summary.Title, &summary.GroupID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fail(404, "not_found", "记录不存在或已删除")
	}
	if err != nil {
		return nil, err
	}
	var fields map[string]any
	switch kind {
	case "idea":
		item, e := loadIdea(ctx, tx, id, "", false, false)
		if e != nil {
			return nil, e
		}
		fields = map[string]any{"original_content": item.OriginalContent, "content": item.Content, "display_title": item.DisplayTitle, "summary": item.Summary,
			"notes": item.Notes, "idea_types": item.IdeaTypes, "product_directions": item.ProductDirections,
			"attachments": mcpAttachments(item.Attachments), "created_at": item.CreatedAt, "updated_at": item.UpdatedAt, "version": item.Version}
	case "product", "tool", "other":
		item, e := loadProduct(ctx, tx, id, false, false)
		if e != nil {
			return nil, e
		}
		fields = mcpProductFields(item)
	case "company":
		item, e := loadCompany(ctx, tx, id, false)
		if e != nil {
			return nil, e
		}
		fields = map[string]any{"chinese_name": item.ChineseName, "english_name": item.EnglishName, "aliases": item.Aliases, "description": item.Description,
			"attachments": mcpAttachments(item.Attachments), "created_at": item.CreatedAt, "updated_at": item.UpdatedAt, "version": item.Version}
	case "recipe":
		item, e := loadRecipe(ctx, tx, id, false)
		if e != nil {
			return nil, e
		}
		var outputID string
		if err := tx.QueryRowContext(ctx, "SELECT output_id FROM recipe_groups WHERE id=?", item.GroupID).Scan(&outputID); err != nil {
			return nil, err
		}
		output, e := loadProduct(ctx, tx, outputID, true, false)
		if e != nil {
			return nil, e
		}
		outputFields := map[string]any{"id": output.ItemKind + ":" + outputID}
		if output.DeletedAt == "" {
			outputFields["chinese_name"], outputFields["english_name"] = output.ChineseName, output.EnglishName
		}
		ingredients := []map[string]any{}
		for _, ingredient := range item.Ingredients {
			entry := map[string]any{"quantity": ingredient.Quantity}
			if ingredient.ProductID == "" {
				entry["category"] = ingredient.Category
			} else {
				entry["product_id"] = ingredient.ProductID
				if ingredient.Item != nil && ingredient.Item.DeletedAt == "" {
					entry["id"] = ingredient.Item.ItemKind + ":" + ingredient.ProductID
					entry["chinese_name"], entry["english_name"] = ingredient.Item.ChineseName, ingredient.Item.EnglishName
				}
			}
			ingredients = append(ingredients, entry)
		}
		fields = map[string]any{"name": item.Name, "group_id": item.GroupID, "output": outputFields, "output_quantity": item.OutputQuantity, "ingredients": ingredients,
			"notes": item.Notes, "verification": item.Verification, "development_status": item.DevelopmentStatus, "development_status_label": item.DevelopmentStatusLabel,
			"created_at": item.CreatedAt, "updated_at": item.UpdatedAt, "version": item.Version}
	}
	related, err := a.mcpQueryRecords(ctx, tx, mcpListArgs{RelatedTo: key, Limit: 50})
	if err != nil {
		return nil, err
	}
	return map[string]any{"id": key, "kind": kind, "title": summary.Title, "text": encodeJSON(fields), "url": a.mcpSummaryURL(summary), "record": fields, "related": related}, nil
}

func (a *App) mcpLibraryInfo(ctx context.Context) (map[string]any, error) {
	counts := map[string]int{}
	for _, kind := range mcpKinds {
		counts[kind] = 0
	}
	rows, err := a.db.QueryContext(ctx, mcpRecordIndex+"SELECT kind,COUNT(*) FROM records GROUP BY kind")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var kind string
		var count int
		if err := rows.Scan(&kind, &count); err != nil {
			rows.Close()
			return nil, err
		}
		counts[kind] = count
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	categories, err := loadProductOptions(ctx, a.db, "categories")
	if err != nil {
		return nil, err
	}
	otherCategories, err := loadProductOptions(ctx, a.db, "other-categories")
	if err != nil {
		return nil, err
	}
	statuses, err := loadProductOptions(ctx, a.db, "statuses")
	if err != nil {
		return nil, err
	}
	useCategories, err := queryIDs(ctx, a.db, "SELECT name FROM tool_use_categories ORDER BY name LIMIT 500")
	if err != nil {
		return nil, err
	}
	return map[string]any{"name": "Call of Mommy 创意库", "counts": counts, "kinds": mcpKinds, "categories": categories, "other_categories": otherCategories, "statuses": statuses, "use_categories": useCategories,
		"scope": mcpReadScope, "default_page_size": 20, "max_page_size": 50,
		"fields": map[string]string{"variant_group_id": "同一商品各款共享的分组 ID；具体款仍使用原物品 ID", "variant_name": "口味／样式名称，可留空；variant_description 为当前款差异说明", "id": "kind:id；fetch 使用完整 ID，company_id 筛选使用不带前缀的公司 ID", "attributes": "商品、道具与其他物品的动态设定属性；certainty 为 asserted/tentative/undecided", "category": "商品与道具使用 categories；其他物品使用独立的 other_categories，且没有所属公司", "idea_types": "灵感主要讨论的方面", "product_directions": "灵感中有明确依据的商品品类",
			"related": "关联摘要；list_records 的 related_to 可分页读取所有关联", "verification": "配方验证状态 untested/verified/invalid", "attachments": "仅文件名、类型、大小、时间等元信息",
			"relations":          "candidate/adopted/rejected 为灵感关联状态；company_item/recipe_output/recipe_ingredient/product_variant 为公司物品/配方产物/配方材料/同商品其他款",
			"deleted_references": "已删除材料和产物仅保留引用 ID 与数量；详情不可读取"}}, nil
}
