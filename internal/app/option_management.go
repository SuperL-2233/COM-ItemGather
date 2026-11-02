package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"regexp"
	"strings"
)

var optionColorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func validOptionColor(value string) bool { return optionColorPattern.MatchString(value) }

func optionRevision(items []ChoiceOption) string {
	digest := sha256.Sum256([]byte(encodeJSON(items)))
	return hex.EncodeToString(digest[:])
}

type optionState struct {
	Items    []ChoiceOption `json:"items"`
	Revision string         `json:"revision"`
}

func loadOptionState(ctx context.Context, q querier, kind string) (optionState, error) {
	items, err := loadProductOptions(ctx, q, kind)
	return optionState{Items: items, Revision: optionRevision(items)}, err
}

func optionChanged(state optionState) *APIError {
	return &APIError{Status: 409, Code: "option_changed", Message: "选项已被其他成员更改，已刷新列表，请重试", Details: state}
}

func (a *App) reorderProductOptions(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	table, key := "product_categories", "name"
	if kind == "statuses" {
		table, key = "product_statuses", "id"
	} else if kind != "categories" {
		writeError(w, fail(404, "not_found", "选项类型不存在"))
		return
	}
	var body struct {
		Order    []string `json:"order"`
		Revision string   `json:"revision"`
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
	state, err := loadOptionState(r.Context(), tx, kind)
	if err != nil {
		writeError(w, err)
		return
	}
	if body.Revision != state.Revision {
		writeError(w, optionChanged(state))
		return
	}
	valid := map[string]bool{}
	for _, item := range state.Items {
		valid[item.Value] = true
	}
	if len(body.Order) != len(state.Items) {
		writeError(w, fail(422, "validation", "请提交全部现有选项的顺序"))
		return
	}
	for _, value := range body.Order {
		if !valid[value] {
			writeError(w, fail(422, "validation", "排序中有重复或不可用的选项"))
			return
		}
		delete(valid, value)
	}
	for index, value := range body.Order {
		_, err = tx.ExecContext(r.Context(), "UPDATE "+table+" SET position=?,version=version+1 WHERE "+key+"=? AND position<>?", index, value, index)
		if err != nil {
			writeError(w, err)
			return
		}
	}
	if err = a.publish(tx, "product_option.reordered", kind, ""); err == nil {
		err = tx.Commit()
	}
	if err != nil {
		writeError(w, err)
		return
	}
	a.afterCommit()
	state, err = loadOptionState(r.Context(), a.db, kind)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, state)
}

func (a *App) updateProductStatusColor(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Color   string `json:"color"`
		Version int    `json:"version"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	if !validOptionColor(body.Color) || body.Version < 1 {
		writeError(w, fail(422, "validation", "请选择有效颜色并刷新选项后重试"))
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer tx.Rollback()
	state, err := loadOptionState(r.Context(), tx, "statuses")
	if err != nil {
		writeError(w, err)
		return
	}
	found := false
	for _, item := range state.Items {
		if item.Value != r.PathValue("id") {
			continue
		}
		found = true
		if item.Version != body.Version {
			writeError(w, optionChanged(state))
			return
		}
	}
	if !found {
		writeError(w, fail(404, "status_deleted", "该制作状态不存在或已被删除"))
		return
	}
	_, err = tx.ExecContext(r.Context(), "UPDATE product_statuses SET color=?,version=version+1 WHERE id=?", strings.ToLower(body.Color), r.PathValue("id"))
	if err == nil {
		err = a.publish(tx, "product_option.changed", "statuses", r.PathValue("id"))
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		writeError(w, err)
		return
	}
	a.afterCommit()
	state, err = loadOptionState(r.Context(), a.db, "statuses")
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, state)
}
