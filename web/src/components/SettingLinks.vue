<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { api } from '../api'
import type { Company, Idea, Page, RecipeGroup, SettingLink } from '../types'
import { companyTitle, ideaTitle, productTitle, statusLabels } from '../utils'
import { useEvents } from '../events'
import { useLiveSearch } from '../composables/useLiveSearch'
const props = defineProps<{ kind: 'company' | 'recipe' | 'idea'; id: string }>()
const links = ref<SettingLink[]>([]); const query = ref(''); const target = ref('company'); const results = ref<{ id: string; title: string }[]>([]);
async function load() { try { const p: Record<string,string> = props.kind === 'idea' ? { idea_id: props.id } : { entity_type: props.kind, entity_id: props.id }; links.value = (await api<{ items: SettingLink[] }>(`/setting-links?${new URLSearchParams(p)}`)).items } catch (e) { error.value = (e as Error).message } }
const { loading: busy, error, run: search, schedule, cancel } = useLiveSearch(async signal => {
  if (!query.value.trim()) return [] as { id: string; title: string }[]
  if (props.kind !== 'idea') return (await api<Page<Idea>>(`/ideas?q=${encodeURIComponent(query.value)}&limit=20`, { signal })).items.map(item => ({ id: item.id, title: ideaTitle(item) }))
  if (target.value === 'company') return (await api<{ items: Company[] }>(`/companies?q=${encodeURIComponent(query.value)}`, { signal })).items.map(item => ({ id: item.id, title: companyTitle(item) }))
  return (await api<{ items: RecipeGroup[] }>(`/recipe-groups?q=${encodeURIComponent(query.value)}`, { signal })).items.flatMap(group => group.recipes.map((item, index) => ({ id: item.id, title: `${productTitle(group.output)} · ${item.name || `配方 ${index+1}`}` })))
}, items => { results.value = items })
watch(query, () => { if (query.value.trim()) schedule(); else { cancel(); results.value = [] } }, { flush: 'sync' })
watch(target, () => { results.value = []; void search() })

async function add(id: string) { error.value = ''; try { await api('/setting-links',{ method: 'POST', body: JSON.stringify(props.kind === 'idea' ? { idea_id: props.id, entity_type: target.value, entity_id: id } : { idea_id: id, entity_type: props.kind, entity_id: props.id }) }); results.value = []; query.value = ''; await load() } catch (e) { error.value = (e as Error).message } }
async function status(link: SettingLink) { try { await api(`/setting-links/${link.id}`,{ method: 'PATCH', body: JSON.stringify({ status: link.status }) }) } catch (e) { error.value = (e as Error).message; await load() } }
async function remove(link: SettingLink) { if (!confirm('解除这条关联？')) return; try { await api(`/setting-links/${link.id}`,{ method: 'DELETE' }); await load() } catch (e) { error.value = (e as Error).message } }
onMounted(load); useEvents(load)
</script>
<template><div><div v-for="link in links" :key="link.id" class="link-item"><RouterLink :to="link.path">{{ link.title }}</RouterLink><select v-model="link.status" aria-label="关联采用状态" @change="status(link)"><option v-for="key in ['candidate','adopted','rejected']" :key="key" :value="key">{{ statusLabels[key] }}</option></select><button type="button" class="text-button danger-text" @click="remove(link)">解除</button></div><p v-if="!links.length" class="muted">还没有关联。</p><form class="link-add" @submit.prevent="search"><select v-if="kind === 'idea'" v-model="target" aria-label="关联设定类型"><option value="company">公司</option><option value="recipe">配方</option></select><input v-model="query" type="search" :aria-label="kind === 'idea' ? '搜索设定' : '搜索关联灵感'" :placeholder="kind === 'idea' ? '搜索公司或配方' : '搜索要关联的灵感'" /><span v-if="busy" class="muted" role="status">搜索中…</span></form><div v-for="item in results" :key="item.id" class="search-result"><span>{{ item.title }}</span><button type="button" class="secondary small" :disabled="links.some(link => kind === 'idea' ? link.entity_id === item.id : link.idea_id === item.id)" @click="add(item.id)">关联</button></div><p v-if="error" class="error-text" role="alert">{{ error }}</p></div></template>
