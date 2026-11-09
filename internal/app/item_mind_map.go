package app

import (
	"context"
	"net/http"
)

type mindMapCompany struct {
	ID          string `json:"id"`
	ChineseName string `json:"chinese_name"`
	EnglishName string `json:"english_name"`
	Aliases     string `json:"aliases"`
}

type mindMapItem struct {
	VariantGroupID         string `json:"variant_group_id"`
	VariantName            string `json:"variant_name"`
	ID                     string `json:"id"`
	ChineseName            string `json:"chinese_name"`
	EnglishName            string `json:"english_name"`
	ItemKind               string `json:"item_kind"`
	CompanyID              string `json:"company_id"`
	CompanyDeleted         bool   `json:"company_deleted"`
	Category               string `json:"category"`
	DevelopmentStatus      string `json:"development_status"`
	DevelopmentStatusLabel string `json:"development_status_label"`
	DevelopmentStatusColor string `json:"development_status_color"`
}

type itemMindMap struct {
	Companies []mindMapCompany `json:"companies"`
	Items     []mindMapItem    `json:"items"`
	Statuses  []ChoiceOption   `json:"statuses"`
}

func loadMindMapCompanies(ctx context.Context, q querier) ([]mindMapCompany, error) {
	rows, err := q.QueryContext(ctx, `SELECT id,chinese_name,english_name,aliases FROM companies
		WHERE deleted_at IS NULL ORDER BY COALESCE(NULLIF(chinese_name,''),english_name) COLLATE ZH_PINYIN,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []mindMapCompany{}
	for rows.Next() {
		var item mindMapCompany
		if err := rows.Scan(&item.ID, &item.ChineseName, &item.EnglishName, &item.Aliases); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func loadMindMapItems(ctx context.Context, q querier) ([]mindMapItem, error) {
	rows, err := q.QueryContext(ctx, `SELECT p.id,p.chinese_name,p.english_name,p.item_kind,COALESCE(p.company_id,''),
		c.deleted_at IS NOT NULL,p.category,COALESCE(p.status_option_id,p.development_status),
		COALESCE(s.name,p.development_status),COALESCE(s.color,'#7c3aed'),COALESCE(p.variant_group_id,''),p.variant_name
		FROM products p LEFT JOIN companies c ON c.id=p.company_id
		LEFT JOIN product_statuses s ON s.id=COALESCE(p.status_option_id,p.development_status)
		WHERE p.item_kind='product' AND p.deleted_at IS NULL
		ORDER BY COALESCE(NULLIF(p.chinese_name,''),p.english_name) COLLATE ZH_PINYIN,p.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []mindMapItem{}
	for rows.Next() {
		var item mindMapItem
		if err := rows.Scan(&item.ID, &item.ChineseName, &item.EnglishName, &item.ItemKind, &item.CompanyID,
			&item.CompanyDeleted, &item.Category, &item.DevelopmentStatus, &item.DevelopmentStatusLabel, &item.DevelopmentStatusColor, &item.VariantGroupID, &item.VariantName); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (a *App) getItemMindMap(w http.ResponseWriter, r *http.Request) {
	// Read the whole graph in one snapshot. List endpoints are paginated and
	// loading each company separately could lose items during a concurrent move.
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer tx.Rollback()
	var result itemMindMap
	result.Companies, err = loadMindMapCompanies(r.Context(), tx)
	if err == nil {
		result.Items, err = loadMindMapItems(r.Context(), tx)
	}
	if err == nil {
		result.Statuses, err = loadProductOptions(r.Context(), tx, "statuses")
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
