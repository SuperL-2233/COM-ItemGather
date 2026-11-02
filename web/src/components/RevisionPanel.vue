<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api'
import type { Revision } from '../types'
import { formatDate } from '../utils'
const props = defineProps<{ kind: 'idea' | 'product' | 'tool' | 'company' | 'recipe'; id: string; version: number }>()
const emit = defineEmits<{ restored: [] }>()
const items = ref<Revision[]>([]); const loading = ref(true); const error = ref('')
const actionNames: Record<string, string> = { create: '创建', remove_category: '删除分类', replace_category: '替换已删除分类', update: '编辑', update_tags: '标签维护', delete: '移入回收站', restore: '恢复', restore_revision: '恢复历史' }
async function load() { loading.value = true; try { items.value = (await api<{ items: Revision[] }>(`/${({idea:'ideas',product:'products',tool:'tools',company:'companies',recipe:'recipes'})[props.kind]}/${props.id}/revisions`)).items } catch (e) { error.value = (e as Error).message } finally { loading.value = false } }
async function restore(item: Revision) { if (!confirm(`恢复到版本 ${item.version}？当前内容会先保留为新的历史版本。`)) return; try { await api(`/${({idea:'ideas',product:'products',tool:'tools',company:'companies',recipe:'recipes'})[props.kind]}/${props.id}/revisions/${item.id}/restore`, { method: 'POST', body: JSON.stringify({ version: props.version }) }); emit('restored'); await load() } catch (e) { error.value = (e as Error).message } }
onMounted(load)
</script>
<template><section class="history-panel"><h2>修改历史</h2><p v-if="loading" class="muted">读取中…</p><p v-if="error" class="error-text">{{ error }}</p><div v-for="item in items" :key="item.id" class="history-item"><div><strong>版本 {{ item.version }} · {{ actionNames[item.action] || (item.action.startsWith('ai_') ? '自动整理' : item.action) }}</strong><small>{{ item.actor_nickname || '系统' }} · {{ formatDate(item.created_at) }}</small></div><details><summary>查看快照</summary><pre>{{ JSON.stringify(item.after, null, 2) }}</pre></details><button v-if="item.version !== version" class="secondary small" @click="restore(item)">恢复此版本</button></div></section></template>
