package app

// Tools share the item identity, attachments and inspiration links of products.
// Their acquisition and use rules are kept together so incomplete designs remain editable.
type AcquisitionSource struct {
	Method     string `json:"method"`
	Provider   string `json:"provider"`
	Timing     string `json:"timing"`
	Conditions string `json:"conditions"`
	Price      string `json:"price"`
}
type ToolSettings struct {
	Trigger        string              `json:"trigger"`
	Effect         string              `json:"effect"`
	Sources        []AcquisitionSource `json:"sources"`
	Cooldown       string              `json:"cooldown"`
	Charges        string              `json:"charges"`
	Consumption    string              `json:"consumption"`
	Duration       string              `json:"duration"`
	ModelStatus    string              `json:"model_status"`
	FunctionStatus string              `json:"function_status"`
}
type Company struct {
	ID          string       `json:"id"`
	ChineseName string       `json:"chinese_name"`
	EnglishName string       `json:"english_name"`
	Aliases     string       `json:"aliases"`
	Description string       `json:"description"`
	Attachments []Attachment `json:"attachments"`
	ItemCount   int          `json:"item_count"`
	Version     int          `json:"version"`
	CreatedBy   string       `json:"created_by"`
	CreatedAt   string       `json:"created_at"`
	UpdatedAt   string       `json:"updated_at"`
	DeletedAt   string       `json:"deleted_at,omitempty"`
}
type Ingredient struct {
	ProductID string   `json:"product_id"`
	Category  string   `json:"category"`
	Quantity  int      `json:"quantity"`
	Item      *Product `json:"item,omitempty"`
}
type Recipe struct {
	ID                string       `json:"id"`
	GroupID           string       `json:"group_id"`
	Name              string       `json:"name"`
	OutputQuantity    int          `json:"output_quantity"`
	Notes             string       `json:"notes"`
	Verification      string       `json:"verification"`
	DevelopmentStatus string       `json:"development_status"`
	Ingredients       []Ingredient `json:"ingredients"`
	Version           int          `json:"version"`
	CreatedBy         string       `json:"created_by"`
	CreatedAt         string       `json:"created_at"`
	UpdatedAt         string       `json:"updated_at"`
	DeletedAt         string       `json:"deleted_at,omitempty"`
}
type RecipeGroup struct {
	RecipeCount *int     `json:"recipe_count,omitempty"`
	ID          string   `json:"id"`
	OutputID    string   `json:"output_id"`
	Output      Product  `json:"output"`
	Recipes     []Recipe `json:"recipes"`
	Version     int      `json:"version"`
	CreatedBy   string   `json:"created_by"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
	DeletedAt   string   `json:"deleted_at,omitempty"`
}
