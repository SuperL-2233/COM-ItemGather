<script setup lang="ts">
import { ref, watch } from 'vue'
import { api } from '../api'
import { useLiveSearch } from '../composables/useLiveSearch'
import type { Idea, Link, Page, Product } from '../types'
import { ideaTitle, productTitle, itemPath, statusLabels } from '../utils'
const props = defineProps<{ ownerType: 'idea' | 'product'; ownerId: string; links: Link[] }>()
const emit = defineEmits<{ changed: [] }>()
const targetKind = ref('products'); const query = ref(''); const results = ref<(Idea | Product)[]>([]);
const { loading: busy, error, run: search, schedule, cancel } = useLiveSearch(async signal => {
  if (!query.value.trim()) return [] as (Idea | Product)[]
  const path = props.ownerType === 'idea' ? `/${targetKind.value}` : '/ideas'
  return (await api<Page<Idea | Product>>(`${path}?q=${encodeURIComponent(query.value)}&limit=10`, { signal })).items
}, items => { results.value = items })
watch(query, () => { if (query.value.trim()) schedule(); else { cancel(); results.value = [] } }, { flush: 'sync' })
watch(targetKind, () => { results.value = []; void search() })

function linked(id: string) { return props.links.some(link => props.ownerType === 'idea' ? link.product_id === id : link.idea_id === id) }
async function add(id: string) { error.value = ''; try { await api('/links', { method: 'POST', body: JSON.stringify(props.ownerType === 'idea' ? { idea_id: props.ownerId, product_id: id, status: 'candidate' } : { idea_id: id, product_id: props.ownerId, status: 'candidate' }) }); query.value = ''; results.value = []; emit('changed') } catch (e) { error.value = (e as Error).message } }
async function status(link: Link) { try { await api(`/links/${link.id}`, { method: 'PATCH', body: JSON.stringify({ status: link.status }) }); emit('changed') } catch (e) { error.value = (e as Error).message } }
async function remove(id: string) { if (!confirm('解除这条关联？灵感和商品本身都会保留。')) return; await api(`/links/${id}`, { method: 'DELETE' }); emit('changed') }
const title = (item: Idea | Product) => 'chinese_name' in item ? productTitle(item) : ideaTitle(item)
const id = (item: Idea | Product) => item.id
</script>
<template><div><div v-if="links.length" class="link-list"><div v-for="link in links" :key="link.id" class="link-item"><RouterLink :to="ownerType === 'idea' ? itemPath({ id: link.product_id, item_kind: link.item_kind }) : `/ideas/${link.idea_id}`">{{ ownerType === 'idea' ? link.product_name : link.idea_title }}</RouterLink><select v-model="link.status" aria-label="关联采用状态" @change="status(link)"><option v-for="key in ['candidate','adopted','rejected']" :key="key" :value="key">{{ statusLabels[key] }}</option></select><button class="text-button danger-text" @click="remove(link.id)">解除</button></div></div><p v-else class="muted">还没有关联。</p><form class="link-add" @submit.prevent="search"><select v-if="ownerType === 'idea'" v-model="targetKind" aria-label="关联物品类型"><option value="products">商品</option><option value="tools">道具</option></select><input v-model="query" type="search" :aria-label="ownerType === 'idea' ? '搜索关联物品' : '搜索关联灵感'" :placeholder="ownerType === 'idea' ? '搜索要关联的商品' : '搜索要关联的灵感'" /><span v-if="busy" class="muted" role="status">搜索中…</span></form><p v-if="error" class="error-text">{{ error }}</p><div class="search-results"><div v-for="item in results" :key="id(item)" class="search-result"><span>{{ title(item) }}</span><button class="secondary small" :disabled="linked(id(item))" @click="add(id(item))">{{ linked(id(item)) ? '已关联' : '关联' }}</button></div></div></div></template>
