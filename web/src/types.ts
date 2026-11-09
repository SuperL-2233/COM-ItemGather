export interface User { id: string; username: string; nickname: string; role: 'admin' | 'member'; status: string; created_at: string }
export interface ReadmeAuthor { id: string; name: string; email: string; version: number; created_at: string; updated_at: string }
export interface ReadmeAuthors { items: ReadmeAuthor[]; selected_author_id: string }
export interface Attribute { id?: string; key: string; value: string; certainty: 'asserted' | 'tentative' | 'undecided' }
export interface Attachment { id: string; idea_id?: string; product_id?: string; company_id?: string; original_name: string; mime_type: string; size: number; url: string; thumbnail_url?: string; created_at: string }
export type ItemKind = 'product' | 'tool' | 'other'
export interface Link { item_kind?: ItemKind; id: string; idea_id: string; product_id: string; status: 'candidate' | 'adopted' | 'rejected'; idea_title?: string; product_name?: string; created_at: string; updated_at: string }
export interface Idea {
  id: string; client_id?: string; original_content: string; content: string; display_title: string; summary: string; notes: string
  idea_types: string[]; product_directions: string[]; attachments: Attachment[]; links?: Link[]
  author_id: string; author_nickname: string; created_at: string; updated_at: string; version: number; content_version: number
  ai_status: 'idle' | 'pending' | 'running' | 'succeeded' | 'failed' | 'skipped'; ai_error?: string; favorite: boolean; product_count: number; deleted_at?: string
}
export interface VariantDraft { variant_name: string; variant_description: string; development_status: string }
export interface Product { variant_group_id: string; variant_name: string; variant_description: string; variant_count: number; variants?: Product[]; item_kind: ItemKind; company_id: string; company_deleted: boolean; use_category: string; tool_settings: ToolSettings; id: string; chinese_name: string; english_name: string; company: string; category: string; description: string; attributes: Attribute[]; attachments: Attachment[]; links?: Link[]; development_status: string; development_status_label: string; development_status_color: string; created_by: string; created_at: string; updated_at: string; version: number; idea_count: number; deleted_at?: string }
export interface Page<T> { items: T[]; total: number; limit: number; offset: number }
export interface Session { id: string; device_name: string; remember: boolean; created_at: string; last_seen_at: string; expires_at: string; current: boolean }
export interface MCPConnection { id: string; client_name: string; client_id: string; scope: string; created_at: string; expires_at: string; last_used_at?: string; revoked_at?: string }
export interface Revision { id: string; entity_type: string; entity_id: string; version: number; actor_id?: string; actor_nickname?: string; action: string; before: unknown; after: unknown; created_at: string }
export interface SystemStatus { ai: { configured: boolean; base_url: string; model: string; timeout_seconds: number; max_retries: number; workers: number }; backup: { directory: string; interval: string; keep: number }; upload: { max_file_bytes: number; max_attachments: number; max_storage_bytes: number } }

export interface AcquisitionSource { method: string; provider: string; timing: string; conditions: string; price: string }
export interface ToolSettings { trigger: string; effect: string; sources: AcquisitionSource[]; cooldown: string; charges: string; consumption: string; duration: string; model_status: string; function_status: string }
export interface Company { id: string; chinese_name: string; english_name: string; aliases: string; description: string; attachments: Attachment[]; item_count: number; version: number; created_by: string; created_at: string; updated_at: string; deleted_at?: string }
export interface Ingredient { product_id: string; category: string; quantity: number; item?: Product }
export interface Recipe { id: string; group_id: string; name: string; output_quantity: number; notes: string; verification: 'untested' | 'verified' | 'invalid'; development_status: Product['development_status']; development_status_label: string; development_status_color: string; ingredients: Ingredient[]; version: number; created_by: string; created_at: string; updated_at: string; deleted_at?: string }
export interface RecipeGroup { recipe_count?: number; id: string; output_id: string; output: Product; recipes: Recipe[]; version: number; created_by: string; created_at: string; updated_at: string; deleted_at?: string }
export interface SettingLink { id: string; idea_id: string; entity_type: 'company' | 'recipe'; entity_id: string; status: string; title: string; path: string }

export interface ChoiceOption { value: string; label: string; color?: string; version?: number }
export interface OptionState { items: ChoiceOption[]; revision: string }
export interface FilterDefinition { key: string; label: string; allLabel: string; options: (string | ChoiceOption)[]; multiple?: boolean }

export interface SearchOption extends ChoiceOption { searchText?: string; description?: string; image?: string }

export interface CategoryUsage { name: string; item_count: number; recipe_count: number; token: string }
export interface CategoryDeletion { name: string; replacement: string; affected_items: number; affected_recipes: number }
export interface StatusUsage { id: string; label: string; item_count: number; recipe_count: number; token: string }
export interface StatusDeletion { id: string; label: string; replacement: string; replacement_label: string; affected_items: number; affected_recipes: number }

export type MindMapCompany = Pick<Company, 'id' | 'chinese_name' | 'english_name' | 'aliases'>
export type MindMapItem = Pick<Product, 'id' | 'chinese_name' | 'english_name' | 'item_kind' | 'company_id' | 'company_deleted' | 'category' | 'development_status' | 'development_status_label' | 'development_status_color' | 'variant_group_id' | 'variant_name'> & { variant_count?: number }
export interface ItemMindMap { companies: MindMapCompany[]; items: MindMapItem[]; statuses: ChoiceOption[] }
