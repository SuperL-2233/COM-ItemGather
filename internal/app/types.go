package app

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

type User struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	Nickname  string `json:"nickname"`
	Role      string `json:"role"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

type Session struct {
	ID         string `json:"id"`
	DeviceName string `json:"device_name"`
	Remember   bool   `json:"remember"`
	CreatedAt  string `json:"created_at"`
	LastSeenAt string `json:"last_seen_at"`
	ExpiresAt  string `json:"expires_at"`
	Current    bool   `json:"current"`
}

type Attribute struct {
	ID        string `json:"id,omitempty"`
	Key       string `json:"key"`
	Value     string `json:"value"`
	Certainty string `json:"certainty"`
}

type Attachment struct {
	ID           string `json:"id"`
	IdeaID       string `json:"idea_id,omitempty"`
	ProductID    string `json:"product_id,omitempty"`
	CompanyID    string `json:"company_id,omitempty"`
	OriginalName string `json:"original_name"`
	MimeType     string `json:"mime_type"`
	Size         int64  `json:"size"`
	URL          string `json:"url"`
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
	CreatedAt    string `json:"created_at"`
}

type Link struct {
	ItemKind    string `json:"item_kind,omitempty"`
	ID          string `json:"id"`
	IdeaID      string `json:"idea_id"`
	ProductID   string `json:"product_id"`
	Status      string `json:"status"`
	IdeaTitle   string `json:"idea_title,omitempty"`
	ProductName string `json:"product_name,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type Idea struct {
	ID                string       `json:"id"`
	ClientID          string       `json:"client_id,omitempty"`
	OriginalContent   string       `json:"original_content"`
	Content           string       `json:"content"`
	DisplayTitle      string       `json:"display_title"`
	Summary           string       `json:"summary"`
	Notes             string       `json:"notes"`
	NameCandidates    []string     `json:"name_candidates"`
	IdeaTypes         []string     `json:"idea_types"`
	ProductDirections []string     `json:"product_directions"`
	Tags              []string     `json:"tags"`
	ManualFields      []string     `json:"-"`
	Attributes        []Attribute  `json:"attributes"`
	Attachments       []Attachment `json:"attachments"`
	Links             []Link       `json:"links,omitempty"`
	AuthorID          string       `json:"author_id"`
	AuthorNickname    string       `json:"author_nickname"`
	CreatedAt         string       `json:"created_at"`
	UpdatedAt         string       `json:"updated_at"`
	Version           int          `json:"version"`
	ContentVersion    int          `json:"content_version"`
	AIStatus          string       `json:"ai_status"`
	AIError           string       `json:"ai_error,omitempty"`
	Favorite          bool         `json:"favorite"`
	ProductCount      int          `json:"product_count"`
	DeletedAt         string       `json:"deleted_at,omitempty"`
}

type Product struct {
	DevelopmentStatusLabel string       `json:"development_status_label"`
	ItemKind               string       `json:"item_kind"`
	CompanyID              string       `json:"company_id"`
	CompanyDeleted         bool         `json:"company_deleted"`
	BrandName              string       `json:"brand_name"`
	UseCategory            string       `json:"use_category"`
	ToolSettings           ToolSettings `json:"tool_settings"`
	ID                     string       `json:"id"`
	ChineseName            string       `json:"chinese_name"`
	EnglishName            string       `json:"english_name"`
	Company                string       `json:"company"`
	Category               string       `json:"category"`
	Description            string       `json:"description"`
	Attributes             []Attribute  `json:"attributes"`
	Attachments            []Attachment `json:"attachments"`
	Links                  []Link       `json:"links,omitempty"`
	DevelopmentStatus      string       `json:"development_status"`
	CreatedBy              string       `json:"created_by"`
	CreatedAt              string       `json:"created_at"`
	UpdatedAt              string       `json:"updated_at"`
	Version                int          `json:"version"`
	IdeaCount              int          `json:"idea_count"`
	DeletedAt              string       `json:"deleted_at,omitempty"`
}

type Revision struct {
	ID            string          `json:"id"`
	EntityType    string          `json:"entity_type"`
	EntityID      string          `json:"entity_id"`
	Version       int             `json:"version"`
	ActorID       string          `json:"actor_id,omitempty"`
	ActorNickname string          `json:"actor_nickname,omitempty"`
	Action        string          `json:"action"`
	Before        json.RawMessage `json:"before"`
	After         json.RawMessage `json:"after"`
	CreatedAt     string          `json:"created_at"`
}

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }
func clean(value string, max int) string {
	value = strings.TrimSpace(value)
	r := []rune(value)
	if len(r) > max {
		return string(r[:max])
	}
	return value
}
func decodeStrings(raw string) []string {
	var values []string
	if json.Unmarshal([]byte(raw), &values) != nil || values == nil {
		return []string{}
	}
	return values
}
func encodeJSON(value any) string { data, _ := json.Marshal(value); return string(data) }
func nullString(value string) sql.NullString {
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}
