export const formatDate = (value: string) => new Intl.DateTimeFormat('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }).format(new Date(value))
export const shortDate = (value: string) => new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }).format(new Date(value))
export const ideaTitle = (idea: { display_title?: string; content?: string; original_content?: string }) => idea.display_title || idea.content || idea.original_content || '未命名灵感'
export const productTitle = (product: { chinese_name?: string; english_name?: string }) => product.chinese_name || product.english_name || '未命名物品'
export const statusLabels: Record<string, string> = { concept: '构思中', design: '设计中', production: '制作中', completed: '已完成', candidate: '备选', adopted: '已采用', rejected: '不采用', asserted: '明确', tentative: '暂定', undecided: '待定' }
export const aiLabels: Record<string, string> = { idle: '内容已修改，可重新整理', pending: '等待整理', running: '正在整理', succeeded: '整理完成', failed: 'AI 整理失败，点击重试', skipped: '纯图片记录' }
export const linesToList = (value: string) => value.split(/[\n,，]/).map(item => item.trim()).filter(Boolean)
export const listToLines = (value: string[]) => value.join('\n')
export function toast(message: string) { window.dispatchEvent(new CustomEvent('toast', { detail: message })) }
export const fileSize = (size: number) => size > 1024 * 1024 ? `${(size / 1024 / 1024).toFixed(1)} MiB` : `${Math.ceil(size / 1024)} KiB`

export const itemPath = (item: { id: string; item_kind?: string }) => `/${item.item_kind === 'tool' ? 'tools' : 'products'}/${item.id}`
export const companyTitle = (company: { chinese_name: string; english_name: string }) => company.chinese_name || company.english_name || '未命名公司'
export const emptyToolSettings = () => ({ trigger: '', effect: '', sources: [], cooldown: '', charges: '', consumption: '', duration: '', model_status: '', function_status: '' })
export const verificationLabels: Record<string, string> = { untested: '待验证', verified: '已验证', invalid: '不可行' }

export const developmentStatusLabel = (item: { development_status: string; development_status_label?: string }) => item.development_status_label || statusLabels[item.development_status] || item.development_status
