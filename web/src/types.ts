export interface User { id: string; username: string; nickname: string; role: 'admin' | 'member'; status: string; created_at: string }
export interface Attribute { id?: string; key: string; value: string; certainty: 'asserted' | 'tentative' | 'undecided' }
export interface Attachment { id: string; idea_id?: string; product_id?: string; original_name: string; mime_type: string; size: number; url: string; thumbnail_url?: string; created_at: string }
export interface Link { id: string; idea_id: string; product_id: string; status: 'candidate' | 'adopted' | 'rejected'; idea_title?: string; product_name?: string; created_at: string; updated_at: string }
export interface Idea {
  id: string; client_id?: string; original_content: string; content: string; display_title: string; summary: string; notes: string
  name_candidates: string[]; idea_types: string[]; product_directions: string[]; tags: string[]; attributes: Attribute[]; attachments: Attachment[]; links?: Link[]
  author_id: string; author_nickname: string; created_at: string; updated_at: string; version: number; content_version: number
  ai_status: 'idle' | 'pending' | 'running' | 'succeeded' | 'failed' | 'skipped'; ai_error?: string; favorite: boolean; product_count: number; deleted_at?: string
}
export interface Product { id: string; chinese_name: string; company: string; category: string; description: string; attributes: Attribute[]; attachments: Attachment[]; links?: Link[]; development_status: 'concept' | 'design' | 'production' | 'completed'; created_by: string; created_at: string; updated_at: string; version: number; idea_count: number; deleted_at?: string }
export interface Page<T> { items: T[]; total: number; limit: number; offset: number }
export interface Session { id: string; device_name: string; remember: boolean; created_at: string; last_seen_at: string; expires_at: string; current: boolean }
export interface Revision { id: string; entity_type: string; entity_id: string; version: number; actor_id?: string; actor_nickname?: string; action: string; before: unknown; after: unknown; created_at: string }
export interface SystemStatus { ai: { configured: boolean; base_url: string; model: string; timeout_seconds: number; max_retries: number; workers: number }; backup: { directory: string; interval: string; keep: number }; upload: { max_file_bytes: number; max_attachments: number; max_storage_bytes: number } }
