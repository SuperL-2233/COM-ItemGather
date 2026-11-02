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
	Value string `json:"value"`
	Label string `json:"label"`
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
	if _, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO product_categories(name,created_at) VALUES(?,?)", name, now()); err != nil {
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
	if err := q.QueryRowContext(ctx, "SELECT id,name FROM product_statuses WHERE id=?", value).Scan(&id, &name); err != nil {
		if err == sql.ErrNoRows {
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
	query := "SELECT name,name FROM product_categories WHERE deleted_at IS NULL ORDER BY name COLLATE NOCASE"
	if kind == "statuses" {
		query = "SELECT id,name FROM product_statuses ORDER BY position,created_at,id"
	}
	rows, err := q.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ChoiceOption{}
	for rows.Next() {
		var item ChoiceOption
		if err := rows.Scan(&item.Value, &item.Label); err != nil {
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
			Name string `json:"name"`
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
		tx, err := a.db.BeginTx(r.Context(), nil)
		if err != nil {
			writeError(w, err)
			return
		}
		defer tx.Rollback()
		item := ChoiceOption{}
		if kind == "categories" {
			_, err = tx.ExecContext(r.Context(), "UPDATE product_categories SET deleted_at=NULL,version=version+1 WHERE name=? COLLATE NOCASE AND deleted_at IS NOT NULL", name)
			if err == nil {
				item.Value, err = ensureProductCategory(r.Context(), tx, name)
			}
			item.Label = item.Value
		} else {
			_, err = tx.ExecContext(r.Context(), "INSERT OR IGNORE INTO product_statuses(id,name,created_at) VALUES(?,?,?)", security.ID(), name, now())
			if err == nil {
				err = tx.QueryRowContext(r.Context(), "SELECT id,name FROM product_statuses WHERE name=? COLLATE NOCASE", name).Scan(&item.Value, &item.Label)
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
	writeJSON(w, 200, map[string]any{"items": items, "option": selected})
}
