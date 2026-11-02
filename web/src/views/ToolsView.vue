<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { api } from '../api'
import type { ChoiceOption, Page, Product } from '../types'
import { useEvents } from '../events'
import { useLiveSearch } from '../composables/useLiveSearch'
import FilterPopover from '../components/FilterPopover.vue'
import ItemCard from '../components/ItemCard.vue'
import ItemCreateDialog from '../components/ItemCreateDialog.vue'
import ProductCatalogItem from '../components/ProductCatalogItem.vue'
import { itemPath } from '../utils'
const router = useRouter(); const route = useRoute()
const items = ref<Product[]>([]); const query = ref(''); const categories = ref<string[]>([]); const statuses = ref<ChoiceOption[]>([]); const creating = ref(false); const total = ref(0)
const layout = ref<'list' | 'grid'>('list')
try { if (localStorage.getItem('com-itemgather:tool-layout') === 'grid') layout.value = 'grid' } catch { /* Storage may be unavailable. */ }
function changeLayout(value: 'list' | 'grid') {
  layout.value = value
  try { localStorage.setItem('com-itemgather:tool-layout', value) } catch { /* Keep the selection for this visit. */ }
}
const filters = ref<Record<string, string[]>>({ use_category: [], source: [], status: [] })
const groups = computed(() => [
  { key: 'use_category', label: '使用类别', allLabel: '全部使用类别', options: categories.value },
  { key: 'source', label: '获取来源', allLabel: '全部获取来源', options: ['购买','NPC 赠送','任务奖励','事件奖励','成就解锁','合成获得','初始持有','其他'] },
  { key: 'status', label: '制作状态', allLabel: '全部状态', options: statuses.value },
])
let append = false
const { loading: busy, error, run, schedule, refresh: refreshSearch } = useLiveSearch(async signal => {
  const parameters = new URLSearchParams({ q: query.value, limit: '30', offset: String(append ? items.value.length : 0) })
  if (typeof route.query.company_id === 'string') parameters.set('company_id', route.query.company_id)
  for (const [key, values] of Object.entries(filters.value)) for (const value of values) parameters.append(key, value)
  return api<Page<Product>>(`/tools?${parameters}`, { signal })
}, result => {
  items.value = append ? [...items.value, ...result.items.filter(item => !items.value.some(existing => existing.id === item.id))] : result.items
  total.value = result.total
})
function load() { append = false; void run() }
function more() { if (!busy.value) { append = true; void run() } }
function created(item: Product) { creating.value = false; void router.push(itemPath(item)) }
async function loadChoices() {
  try {
    const [uses, states] = await Promise.all([api<{items:string[]}>('/tool-categories'), api<{items:ChoiceOption[]}>('/product-options/statuses')])
    categories.value = uses.items; statuses.value = states.items
  } catch (cause) { error.value = (cause as Error).message }
}
function refresh() { append = false; refreshSearch(); void loadChoices() }
watch(query, () => { append = false; schedule() }, { flush: 'sync' })
watch(filters, load)
watch(() => route.query.company_id, load)
onMounted(() => { load(); void loadChoices() })
useEvents(refresh)
</script>
<template>
  <section>
    <div class="page-heading"><div><h1>道具库</h1><p>记录道具的获得途径、触发方式与使用效果。</p></div><button class="primary" @click="creating = true">创建道具</button></div>
    <p v-if="route.query.company_id" class="muted">正在查看所选公司旗下道具 · <RouterLink to="/tools">查看全部公司</RouterLink></p>
    <form class="toolbar" @submit.prevent="load()"><input v-model="query" type="search" aria-label="搜索道具" placeholder="搜索名称、效果、公司或获取来源" /><FilterPopover id="tool-filter-popover" v-model="filters" label="道具" :groups="groups" /></form>
    <div class="catalog-view-controls">
      <div class="product-view-switch" role="group" aria-label="道具展示方式">
        <button type="button" :aria-pressed="layout === 'list'" @click="changeLayout('list')"><svg viewBox="0 0 20 20" aria-hidden="true"><path d="M7 5h10M7 10h10M7 15h10M3 5h.5M3 10h.5M3 15h.5" /></svg>列表</button>
        <button type="button" :aria-pressed="layout === 'grid'" @click="changeLayout('grid')"><svg viewBox="0 0 20 20" aria-hidden="true"><rect x="3" y="3" width="5" height="5" rx="1" /><rect x="12" y="3" width="5" height="5" rx="1" /><rect x="3" y="12" width="5" height="5" rx="1" /><rect x="12" y="12" width="5" height="5" rx="1" /></svg>卡片</button>
      </div>
    </div>
    <p v-if="error" class="error-text">{{ error }} <button class="button-link" @click="load()">重试</button></p>
    <p v-if="busy && !items.length" class="muted">读取中…</p>
    <div v-else-if="items.length" class="product-catalog" :class="`is-${layout}`" :aria-busy="busy">
      <div v-if="layout === 'list'" class="product-list-header" aria-hidden="true"><span>道具名称</span><span>所属公司</span><span>使用类别</span><span>制作状态</span></div>
      <template v-for="item in items" :key="item.id">
        <ProductCatalogItem v-if="layout === 'list'" :product="item" layout="list" />
        <ItemCard v-else :item="item" />
      </template>
    </div>
    <div v-else class="empty-state">{{ query || Object.values(filters).some(values => values.length) ? '没有符合条件的道具。' : '还没有道具，先记录一个名称或效果吧。' }}</div>
    <button v-if="items.length < total" class="secondary load-more" :disabled="busy" @click="more">加载更多</button>
  </section>
  <ItemCreateDialog v-if="creating" kind="tool" @created="created" @cancel="creating = false" />
</template>

<style scoped>
.catalog-view-controls{display:flex;justify-content:flex-end;margin:1rem 0 .75rem}
.product-view-switch{display:flex;gap:.2rem;padding:.2rem;border:1px solid var(--border);border-radius:7px;background:var(--surface-soft);flex:none}
.product-view-switch button{display:flex;align-items:center;gap:.35rem;padding:.35rem .6rem;border-radius:4px;background:transparent;color:var(--muted);font-size:.78rem}
.product-view-switch button[aria-pressed=true]{background:var(--surface);color:var(--text);box-shadow:0 1px 3px #0001}
.product-view-switch button:hover{color:var(--text)}
.product-view-switch svg{width:.95rem;height:.95rem;fill:none;stroke:currentColor;stroke-width:1.6;stroke-linecap:round;stroke-linejoin:round}
.product-catalog{--catalog-columns:minmax(0,2.3fr) minmax(0,1.2fr) minmax(0,.8fr) minmax(6rem,.8fr);--catalog-gap:1.25rem}
.product-catalog.is-list{border:1px solid var(--border);border-radius:9px;background:var(--surface)}
.product-list-header{display:grid;grid-template-columns:var(--catalog-columns);gap:var(--catalog-gap);padding:.8rem 1.1rem;background:var(--surface-soft);border-radius:8px 8px 0 0;color:var(--muted);font-size:.75rem}
.product-catalog.is-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:1rem;align-items:start}
@media(max-width:900px){.product-catalog{--catalog-gap:.8rem;--catalog-columns:minmax(0,2fr) minmax(0,1.2fr) minmax(0,.8fr) minmax(5rem,.8fr)}.product-catalog.is-grid{grid-template-columns:repeat(2,minmax(0,1fr))}}
@media(max-width:600px){.product-list-header{display:none}.product-catalog.is-grid{grid-template-columns:1fr;gap:.75rem}.product-view-switch button{padding:.35rem .5rem}}
</style>
