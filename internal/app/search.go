package app

import (
	"context"
	"net/http"
	"slices"
	"strings"
)

const searchResultLimit = 50

// Values are bound separately; column/expression is always an internal constant.
func searchIn(query string, args []any, expression string, values []string) (string, []any) {
	if len(values) > 0 {
		query += " AND " + expression + " IN (" + productFilterPlaceholders(values) + ")"
		for _, value := range values {
			args = append(args, value)
		}
	}
	return query, args
}

func (a *App) globalSearch(w http.ResponseWriter, r *http.Request) {
	parameters := r.URL.Query()
	query := strings.TrimSpace(parameters.Get("q"))
	kinds := productFilterValues(parameters["kind"])
	types := productFilterValues(parameters["idea_type"])
	directions := productFilterValues(parameters["direction"])
	authors := productFilterValues(parameters["author"])
	valid := len([]rune(query)) <= 200
	for _, values := range [][]string{kinds, types, directions, authors} {
		valid = valid && len(values) <= 50
		for _, value := range values {
			valid = valid && len([]rune(value)) <= 100
		}
	}
	for _, kind := range kinds {
		valid = valid && slices.Contains([]string{"all", "ideas", "products", "tools", "other-items", "companies", "recipes"}, kind)
	}
	if !valid {
		writeError(w, fail(422, "validation", "搜索条件无效"))
		return
	}
	includes := func(kind string) bool {
		return len(kinds) == 0 || slices.Contains(kinds, "all") || slices.Contains(kinds, kind)
	}
	ctx := r.Context()
	creatorRows, err := a.db.QueryContext(ctx, "SELECT id,nickname FROM users WHERE status='active' ORDER BY nickname COLLATE NOCASE")
	if err != nil {
		writeError(w, err)
		return
	}
	creators := []map[string]string{}
	for creatorRows.Next() {
		var id, nickname string
		if err = creatorRows.Scan(&id, &nickname); err != nil {
			creatorRows.Close()
			writeError(w, err)
			return
		}
		creators = append(creators, map[string]string{"id": id, "nickname": nickname})
	}
	err = creatorRows.Err()
	creatorRows.Close()
	if err != nil {
		writeError(w, err)
		return
	}
	ideas := []Idea{}
	products := []Product{}
	tools := []Product{}
	otherItems := []Product{}
	companies := []Company{}
	groups := []RecipeGroup{}
	more := map[string]bool{}
	write := func() {
		writeJSON(w, 200, map[string]any{"ideas": ideas, "products": products, "tools": tools, "other_items": otherItems, "companies": companies, "recipes": groups, "creators": creators, "has_more": more, "limit": searchResultLimit})
	}
	if query == "" && len(kinds) == 0 && len(types) == 0 && len(directions) == 0 && len(authors) == 0 {
		write()
		return
	}
	like := "%" + escapeLike(query) + "%"
	if includes("ideas") {
		sql := `SELECT i.id FROM ideas i WHERE i.deleted_at IS NULL AND (?='' OR i.original_content LIKE ? ESCAPE '\' OR i.content LIKE ? ESCAPE '\' OR i.display_title LIKE ? ESCAPE '\' OR i.summary LIKE ? ESCAPE '\' OR i.notes LIKE ? ESCAPE '\' OR i.idea_types_json LIKE ? ESCAPE '\' OR i.product_directions_json LIKE ? ESCAPE '\')`
		args := []any{query, like, like, like, like, like, like, like}
		for _, filter := range []struct {
			column string
			values []string
		}{{"i.idea_types_json", types}, {"i.product_directions_json", directions}} {
			if len(filter.values) > 0 {
				sql += " AND EXISTS(SELECT 1 FROM json_each(" + filter.column + ") WHERE value IN (" + productFilterPlaceholders(filter.values) + "))"
				for _, value := range filter.values {
					args = append(args, value)
				}
			}
		}
		sql, args = searchIn(sql, args, "i.author_id", authors)
		ids, e := queryIDs(ctx, a.db, sql+" ORDER BY i.updated_at DESC,i.id LIMIT 51", args...)
		if e != nil {
			writeError(w, e)
			return
		}
		ids, more["ideas"] = limitSearchIDs(ids)
		for _, id := range ids {
			item, e := loadIdea(ctx, a.db, id, currentUser(r).ID, false, false)
			if e != nil {
				writeError(w, e)
				return
			}
			ideas = append(ideas, item)
		}
	}
	// Content type and direction describe inspirations, so these filters only
	// return inspirations, matching the original global-search behavior.
	if len(types) == 0 && len(directions) == 0 {
		for _, entity := range []struct {
			kind, itemKind string
			items          *[]Product
		}{{"products", "product", &products}, {"tools", "tool", &tools}, {"other-items", "other", &otherItems}} {
			if !includes(entity.kind) {
				continue
			}
			items, hasMore, e := a.searchItems(ctx, query, like, entity.itemKind, authors)
			if e != nil {
				writeError(w, e)
				return
			}
			*entity.items = items
			more[entity.kind] = hasMore
		}
		if includes("companies") {
			companies, err = findCompanies(ctx, a.db, query, authors, false, searchResultLimit+1)
			if len(companies) > searchResultLimit {
				more["companies"] = true
				companies = companies[:searchResultLimit]
			}
		}
		if err == nil && includes("recipes") {
			groups, err = findRecipeGroups(ctx, a.db, query, authors, "", false, searchResultLimit+1, true)
			if len(groups) > searchResultLimit {
				more["recipes"] = true
				groups = groups[:searchResultLimit]
			}
		}
		if err != nil {
			writeError(w, err)
			return
		}
	}
	write()
}

func limitSearchIDs(ids []string) ([]string, bool) {
	if len(ids) > searchResultLimit {
		return ids[:searchResultLimit], true
	}
	return ids, false
}

func (a *App) searchItems(ctx context.Context, query, like, kind string, authors []string) ([]Product, bool, error) {
	sql := `SELECT p.id FROM products p WHERE p.deleted_at IS NULL AND p.parent_product_id IS NULL AND p.item_kind=? AND (?='' OR p.chinese_name LIKE ? ESCAPE '\' OR p.english_name LIKE ? ESCAPE '\' OR p.company LIKE ? ESCAPE '\' OR p.category LIKE ? ESCAPE '\' OR p.description LIKE ? ESCAPE '\' OR p.use_category LIKE ? ESCAPE '\' OR p.variant_name LIKE ? ESCAPE '\' OR p.variant_description LIKE ? ESCAPE '\' OR p.tool_settings_json LIKE ? ESCAPE '\' OR EXISTS(SELECT 1 FROM companies c WHERE c.id=p.company_id AND (c.english_name LIKE ? ESCAPE '\' OR c.aliases LIKE ? ESCAPE '\')) OR EXISTS(SELECT 1 FROM product_attributes a WHERE a.product_id=p.id AND (a.key LIKE ? ESCAPE '\' OR a.value LIKE ? ESCAPE '\')) OR EXISTS(SELECT 1 FROM products v WHERE v.parent_product_id=p.id AND v.deleted_at IS NULL AND (v.variant_name LIKE ? ESCAPE '\' OR v.variant_description LIKE ? ESCAPE '\')))`
	args := []any{kind, query, like, like, like, like, like, like, like, like, like, like, like, like, like, like, like}
	sql, args = searchIn(sql, args, "p.created_by", authors)
	ids, err := queryIDs(ctx, a.db, sql+" ORDER BY p.updated_at DESC,p.id LIMIT 51", args...)
	if err != nil {
		return nil, false, err
	}
	ids, more := limitSearchIDs(ids)
	items := []Product{}
	for _, id := range ids {
		item, e := loadProduct(ctx, a.db, id, false, false)
		if e != nil {
			return nil, false, e
		}
		items = append(items, item)
	}
	return items, more, nil
}
