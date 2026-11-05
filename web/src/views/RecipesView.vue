<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'
import type { Ingredient, RecipeGroup } from '../types'
import { useEvents } from '../events'
import { useLiveSearch } from '../composables/useLiveSearch'
import ItemPicker from '../components/ItemPicker.vue'
import RecipeCatalogItem from '../components/RecipeCatalogItem.vue'
import { useListNavigation } from '../composables/listNavigation'
type RecipeListState = { items: RecipeGroup[]; query: string; layout: 'list' | 'grid' }
const previous: RecipeListState | undefined = useListNavigation<RecipeListState>(() => ({ items: items.value, query: query.value, layout: layout.value }))
const router = useRouter(); const items = ref<RecipeGroup[]>(previous?.items || []); const query = ref(previous?.query || ''); const picking = ref(false); const busy = ref(false);
const layout = ref<'list' | 'grid'>('list')
try { if (localStorage.getItem('com-itemgather:recipe-layout') === 'grid') layout.value = 'grid' } catch { /* Storage may be unavailable. */ }
if (previous) layout.value = previous.layout
function changeLayout(value: 'list' | 'grid') {
  layout.value = value
  try { localStorage.setItem('com-itemgather:recipe-layout', value) } catch { /* Keep the selection for this visit. */ }
}
const { loading, error, run: load, schedule, refresh } = useLiveSearch(signal => api<{ items: RecipeGroup[] }>(`/recipe-groups?q=${encodeURIComponent(query.value)}`, { signal }), result => { items.value = result.items })
async function selected(ingredient: Ingredient) { if (busy.value) return; busy.value = true; error.value = ''; try { const group = await api<RecipeGroup>('/recipe-groups',{ method: 'POST', body: JSON.stringify({ output_id: ingredient.product_id }) }); picking.value = false; await router.push(`/recipes/${group.id}`) } catch (e) { error.value = (e as Error).message; picking.value = false } finally { busy.value = false } }
onMounted(load); watch(query, schedule, { flush: 'sync' }); useEvents(refresh)
</script>
<template>
  <section>
    <div class="page-heading"><div><h1>合成配方</h1><p>按产物归组，在每个产物中管理多套合成方案。</p></div><button class="primary" :disabled="busy" @click="picking = true">添加合成产物</button></div>
    <form class="toolbar" @submit.prevent="load"><input v-model="query" type="search" aria-label="搜索配方" placeholder="搜索产物、材料、分类或配方备注" /></form>
    <div class="catalog-view-controls">
      <div class="product-view-switch" role="group" aria-label="配方展示方式">
        <button type="button" :aria-pressed="layout === 'list'" @click="changeLayout('list')"><svg viewBox="0 0 20 20" aria-hidden="true"><path d="M7 5h10M7 10h10M7 15h10M3 5h.5M3 10h.5M3 15h.5" /></svg>列表</button>
        <button type="button" :aria-pressed="layout === 'grid'" @click="changeLayout('grid')"><svg viewBox="0 0 20 20" aria-hidden="true"><rect x="3" y="3" width="5" height="5" rx="1" /><rect x="12" y="3" width="5" height="5" rx="1" /><rect x="3" y="12" width="5" height="5" rx="1" /><rect x="12" y="12" width="5" height="5" rx="1" /></svg>卡片</button>
      </div>
    </div>
    <p v-if="error" class="error-text">{{ error }} <button class="button-link" @click="load">重试</button></p>
    <p v-if="loading && !items.length" class="muted">读取中…</p>
    <div v-else-if="items.length" class="product-catalog" :class="`is-${layout}`" :aria-busy="loading">
      <div v-if="layout === 'list'" class="product-list-header" aria-hidden="true"><span>产物名称</span><span>类型</span><span>配方数量</span><span>制作状态</span></div>
      <RecipeCatalogItem v-for="group in items" :key="group.id" :group="group" :layout="layout" />
    </div>
    <div v-else class="empty-state">{{ query ? '没有匹配的产物或材料。' : '选择商品、道具或其他物品作为产物，开始记录配方。' }}</div>
  </section>
  <ItemPicker v-if="picking" title="选择合成产物" @select="selected" @cancel="picking = false" />
</template>

<style scoped>
.catalog-view-controls{display:flex;justify-content:flex-end;margin:1rem 0 .75rem}
.product-view-switch{display:flex;gap:.2rem;padding:.2rem;border:1px solid var(--border);border-radius:7px;background:var(--surface-soft);flex:none}
.product-view-switch button{display:flex;align-items:center;gap:.35rem;padding:.35rem .6rem;border-radius:4px;background:transparent;color:var(--muted);font-size:.78rem}
.product-view-switch button[aria-pressed=true]{background:var(--surface);color:var(--text);box-shadow:0 1px 3px #0001}
.product-view-switch button:hover{color:var(--text)}
.product-view-switch svg{width:.95rem;height:.95rem;fill:none;stroke:currentColor;stroke-width:1.6;stroke-linecap:round;stroke-linejoin:round}
.product-catalog{--catalog-columns:minmax(0,2.3fr) minmax(4rem,.6fr) minmax(6rem,.8fr) minmax(6rem,.8fr);--catalog-gap:1.25rem}
.product-catalog.is-list{border:1px solid var(--border);border-radius:9px;background:var(--surface)}
.product-list-header{display:grid;grid-template-columns:var(--catalog-columns);gap:var(--catalog-gap);padding:.8rem 1.1rem;background:var(--surface-soft);border-radius:8px 8px 0 0;color:var(--muted);font-size:.75rem}
.product-catalog.is-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:1rem;align-items:start}
@media(max-width:900px){.product-catalog{--catalog-gap:.8rem;--catalog-columns:minmax(0,2fr) minmax(3.5rem,.6fr) minmax(5rem,.8fr) minmax(5rem,.8fr)}.product-catalog.is-grid{grid-template-columns:repeat(2,minmax(0,1fr))}}
@media(max-width:600px){.product-list-header{display:none}.product-catalog.is-grid{grid-template-columns:1fr;gap:.75rem}.product-view-switch button{padding:.35rem .5rem}}
</style>
