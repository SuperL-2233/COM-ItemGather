export interface User { id: string; username: string; nickname: string; role: 'admin' | 'member'; status: string; created_at: string }
export interface Attribute { id?: string; key: string; value: string; certainty: 'asserted' | 'tentative' | 'undecided' }
export interface Attachment { id: string; idea_id?: string; product_id?: string; company_id?: string; original_name: string; mime_type: string; size: number; url: string; thumbnail_url?: string; created_at: string }
export interface Link { item_kind?: 'product' | 'tool'; id: string; idea_id: string; product_id: string; status: 'candidate' | 'adopted' | 'rejected'; idea_title?: string; product_name?: string; created_at: string; updated_at: string }
export interface Idea {
  id: string; client_id?: string; original_content: string; content: string; display_title: string; summary: string; notes: string
  name_candidates: string[]; idea_types: string[]; product_directions: string[]; tags: string[]; attributes: Attribute[]; attachments: Attachment[]; links?: Link[]
  author_id: string; author_nickname: string; created_at: string; updated_at: string; version: number; content_version: number
  ai_status: 'idle' | 'pending' | 'running' | 'succeeded' | 'failed' | 'skipped'; ai_error?: string; favorite: boolean; product_count: number; deleted_at?: string
}
export interface Product { item_kind: 'product' | 'tool'; company_id: string; company_deleted: boolean; brand_name: string; use_category: string; tool_settings: ToolSettings; id: string; chinese_name: string; english_name: string; company: string; category: string; description: string; attributes: Attribute[]; attachments: Attachment[]; links?: Link[]; development_status: string; development_status_label: string; created_by: string; created_at: string; updated_at: string; version: number; idea_count: number; deleted_at?: string }
export interface Page<T> { items: T[]; total: number; limit: number; offset: number }
export interface Session { id: string; device_name: string; remember: boolean; created_at: string; last_seen_at: string; expires_at: string; current: boolean }
export interface Revision { id: string; entity_type: string; entity_id: string; version: number; actor_id?: string; actor_nickname?: string; action: string; before: unknown; after: unknown; created_at: string }
export interface SystemStatus { ai: { configured: boolean; base_url: string; model: string; timeout_seconds: number; max_retries: number; workers: number }; backup: { directory: string; interval: string; keep: number }; upload: { max_file_bytes: number; max_attachments: number; max_storage_bytes: number } }

export interface AcquisitionSource { method: string; provider: string; timing: string; conditions: string; price: string }
export interface ToolSettings { trigger: string; effect: string; sources: AcquisitionSource[]; cooldown: string; charges: string; consumption: string; duration: string; model_status: string; function_status: string }
export interface Company { id: string; chinese_name: string; english_name: string; aliases: string; description: string; attachments: Attachment[]; item_count: number; version: number; created_by: string; created_at: string; updated_at: string; deleted_at?: string }
export interface Ingredient { product_id: string; category: string; quantity: number; item?: Product }
export interface Recipe { id: string; group_id: string; name: string; output_quantity: number; notes: string; verification: 'untested' | 'verified' | 'invalid'; development_status: Product['development_status']; ingredients: Ingredient[]; version: number; created_by: string; created_at: string; updated_at: string; deleted_at?: string }
export interface RecipeGroup { id: string; output_id: string; output: Product; recipes: Recipe[]; version: number; created_by: string; created_at: string; updated_at: string; deleted_at?: string }
export interface SettingLink { id: string; idea_id: string; entity_type: 'company' | 'recipe'; entity_id: string; status: string; title: string; path: string }

export interface ChoiceOption { value: string; label: string }

export interface SearchOption extends ChoiceOption { searchText?: string; description?: string; image?: string }
