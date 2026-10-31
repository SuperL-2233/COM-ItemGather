<script setup lang="ts">
import { ref } from 'vue'
import { api } from '../api'
import type { Idea, Link, Page, Product } from '../types'
import { ideaTitle, statusLabels } from '../utils'
const props = defineProps<{ ownerType: 'idea' | 'product'; ownerId: string; links: Link[] }>()
const emit = defineEmits<{ changed: [] }>()
const query = ref(''); const results = ref<(Idea | Product)[]>([]); const busy = ref(false); const error = ref('')
async function search() { if (!query.value.trim()) { results.value = []; return }; busy.value = true; try { if (props.ownerType === 'idea') results.value = (await api<Page<Product>>(`/products?q=${encodeURIComponent(query.value)}&limit=10`)).items; else results.value = (await api<Page<Idea>>(`/ideas?q=${encodeURIComponent(query.value)}&limit=10`)).items } catch (e) { error.value = (e as Error).message } finally { busy.value = false } }
function linked(id: string) { return props.links.some(link => props.ownerType === 'idea' ? link.product_id === id : link.idea_id === id) }
async function add(id: string) { error.value = ''; try { await api('/links', { method: 'POST', body: JSON.stringify(props.ownerType === 'idea' ? { idea_id: props.ownerId, product_id: id, status: 'candidate' } : { idea_id: id, product_id: props.ownerId, status: 'candidate' }) }); query.value = ''; results.value = []; emit('changed') } catch (e) { error.value = (e as Error).message } }
async function status(link: Link) { try { await api(`/links/${link.id}`, { method: 'PATCH', body: JSON.stringify({ status: link.status }) }); emit('changed') } catch (e) { error.value = (e as Error).message } }
async function remove(id: string) { if (!confirm('解除这条关联？灵感和商品本身都会保留。')) return; await api(`/links/${id}`, { method: 'DELETE' }); emit('changed') }
const title = (item: Idea | Product) => 'internal_name' in item ? item.internal_name : ideaTitle(item)
const id = (item: Idea | Product) => item.id
</script>
<template><div><div v-if="links.length" class="link-list"><div v-for="link in links" :key="link.id" class="link-item"><RouterLink :to="ownerType === 'idea' ? `/products/${link.product_id}` : `/ideas/${link.idea_id}`">{{ ownerType === 'idea' ? link.product_name : link.idea_title }}</RouterLink><select v-model="link.status" aria-label="关联采用状态" @change="status(link)"><option v-for="key in ['candidate','adopted','rejected']" :key="key" :value="key">{{ statusLabels[key] }}</option></select><button class="text-button danger-text" @click="remove(link.id)">解除</button></div></div><p v-else class="muted">还没有关联。</p><form class="link-add" @submit.prevent="search"><input v-model="query" :placeholder="ownerType === 'idea' ? '搜索要关联的商品' : '搜索要关联的灵感'" /><button class="secondary" :disabled="busy">搜索</button></form><p v-if="error" class="error-text">{{ error }}</p><div class="search-results"><div v-for="item in results" :key="id(item)" class="search-result"><span>{{ title(item) }}</span><button class="secondary small" :disabled="linked(id(item))" @click="add(id(item))">{{ linked(id(item)) ? '已关联' : '关联' }}</button></div></div></div></template>
