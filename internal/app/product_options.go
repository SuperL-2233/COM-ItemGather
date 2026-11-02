package app

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"unicode"

	"com-itemgather/internal/security"
)

type ChoiceOption struct {
	Value   string `json:"value"`
	Label   string `json:"label"`
	Color   string `json:"color,omitempty"`
	Version int    `json:"version"`
}

func validOptionName(name string, max int) bool {
	if name == "" || name == "+" || name == "＋" || len([]rune(name)) > max {
		return false
	}
	return !strings.ContainsFunc(name, unicode.IsControl)
}

func ensureProductCategory(ctx context.Context, tx *sql.Tx, name string) (string, error) {
	name = clean(name, 100)
	if name == "" {
		return "", nil
	}
	if !validOptionName(name, 100) {
		return "", fail(422, "validation", "商品分类名称无效")
	}
	if _, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO product_categories(name,created_at,position) VALUES(?,?,(SELECT COALESCE(MAX(position),-1)+1 FROM product_categories WHERE deleted_at IS NULL))", name, now()); err != nil {
		return "", err
	}
	var canonical string
	err := tx.QueryRowContext(ctx, "SELECT name FROM product_categories WHERE name=? COLLATE NOCASE AND deleted_at IS NULL", name).Scan(&canonical)
	if err == sql.ErrNoRows {
		return "", fail(409, "category_deleted", "该分类已被删除，请重新选择；需要恢复时可显式新建同名分类")
	}
	return canonical, err
}

func resolveProductStatus(ctx context.Context, q querier, value string) (string, string, string, error) {
	var id, name string
	query := "SELECT id,name FROM product_statuses WHERE deleted_at IS NULL AND (?='' OR id=?) ORDER BY (id='concept') DESC,position,created_at,id LIMIT 1"
	if err := q.QueryRowContext(ctx, query, value, value).Scan(&id, &name); err != nil {
		if err == sql.ErrNoRows {
			var deleted int
			if e := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM product_statuses WHERE id=? AND deleted_at IS NOT NULL", value).Scan(&deleted); e != nil {
				return "", "", "", e
			}
			if deleted > 0 {
				return "", "", "", fail(409, "status_deleted", "该制作状态已被删除，请重新选择；需要恢复时可显式新建同名状态")
			}
			return "", "", "", fail(422, "validation", "制作状态无效，请选择已有状态或先创建新状态")
		}
		return "", "", "", err
	}
	legacy := id
	if !validProductStatus(legacy) {
		legacy = "concept"
	}
	return id, legacy, name, nil
}

func loadProductOptions(ctx context.Context, q querier, kind string) ([]ChoiceOption, error) {
	query := "SELECT name,name,'',version FROM product_categories WHERE deleted_at IS NULL ORDER BY position,name COLLATE NOCASE"
	if kind == "statuses" {
		query = "SELECT id,name,color,version FROM product_statuses WHERE deleted_at IS NULL ORDER BY position,created_at,id"
	}
	rows, err := q.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ChoiceOption{}
	for rows.Next() {
		var item ChoiceOption
		if err := rows.Scan(&item.Value, &item.Label, &item.Color, &item.Version); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (a *App) productOptions(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	if kind != "categories" && kind != "statuses" {
		writeError(w, fail(404, "not_found", "选项类型不存在"))
		return
	}
	var selected *ChoiceOption
	if r.Method == http.MethodPost {
		var body struct {
			Name  string  `json:"name"`
			Color *string `json:"color"`
		}
		if err := decodeBody(w, r, &body); err != nil {
			writeError(w, err)
			return
		}
		name := strings.TrimSpace(body.Name)
		limit := 100
		if kind == "statuses" {
			limit = 80
		}
		if !validOptionName(name, limit) {
			writeError(w, fail(422, "validation", "名称不能为空、不能包含控制字符，且不能超过长度限制"))
			return
		}
		if body.Color != nil && (kind != "statuses" || !validOptionColor(*body.Color)) {
			writeError(w, fail(422, "validation", "请选择有效的制作状态颜色"))
			return
		}
		tx, err := a.db.BeginTx(r.Context(), nil)
		if err != nil {
			writeError(w, err)
			return
		}
		defer tx.Rollback()
		item := ChoiceOption{}
		if kind == "categories" {
			_, err = tx.ExecContext(r.Context(), "UPDATE product_categories SET deleted_at=NULL,version=version+1,position=(SELECT COALESCE(MAX(position),-1)+1 FROM product_categories WHERE deleted_at IS NULL) WHERE name=? COLLATE NOCASE AND deleted_at IS NOT NULL", name)
			if err == nil {
				item.Value, err = ensureProductCategory(r.Context(), tx, name)
			}
			item.Label = item.Value
			if err == nil {
				err = tx.QueryRowContext(r.Context(), "SELECT version FROM product_categories WHERE name=? COLLATE NOCASE", item.Value).Scan(&item.Version)
			}
		} else {
			color := "#7c3aed"
			var restoredColor any
			if body.Color != nil {
				color = strings.ToLower(*body.Color)
				restoredColor = color
			}
			_, err = tx.ExecContext(r.Context(), "UPDATE product_statuses SET deleted_at=NULL,version=version+1,color=COALESCE(?,color),position=(SELECT COALESCE(MAX(position),-1)+1 FROM product_statuses WHERE deleted_at IS NULL) WHERE name=? COLLATE NOCASE AND deleted_at IS NOT NULL", restoredColor, name)
			if err == nil {
				_, err = tx.ExecContext(r.Context(), "INSERT OR IGNORE INTO product_statuses(id,name,created_at,color,position) VALUES(?,?,?,?,(SELECT COALESCE(MAX(position),-1)+1 FROM product_statuses WHERE deleted_at IS NULL))", security.ID(), name, now(), color)
			}
			if err == nil {
				err = tx.QueryRowContext(r.Context(), "SELECT id,name,color,version FROM product_statuses WHERE name=? COLLATE NOCASE", name).Scan(&item.Value, &item.Label, &item.Color, &item.Version)
			}
		}
		if err == nil {
			err = a.publish(tx, "product_option.changed", kind, item.Value)
		}
		if err == nil {
			err = tx.Commit()
		}
		if err != nil {
			writeError(w, err)
			return
		}
		a.afterCommit()
		selected = &item
	}
	items, err := loadProductOptions(r.Context(), a.db, kind)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items, "option": selected, "revision": optionRevision(items)})
}
