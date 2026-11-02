<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import ItemCreateDialog from '../components/ItemCreateDialog.vue'
import ProductCatalogItem from '../components/ProductCatalogItem.vue'
import { api } from '../api'
import type { ChoiceOption, Page, Product } from '../types'
import { useEvents } from '../events'
import FilterPopover from '../components/FilterPopover.vue'

interface ProductFilterOptions { companies: string[]; categories: string[]; attributes: string[]; statuses: ChoiceOption[] }

const route = useRoute()
const products = ref<Product[]>([]); const total = ref(0); const query = ref(''); const loading = ref(true); const loadingMore = ref(false); const error = ref(''); const showCreate = ref(false);
const layout = ref<'list' | 'grid'>('list')
try { if (localStorage.getItem('com-itemgather:product-layout') === 'grid') layout.value = 'grid' } catch { /* Storage may be unavailable. */ }
function changeLayout(value: 'list' | 'grid') {
  layout.value = value
  try { localStorage.setItem('com-itemgather:product-layout', value) } catch { /* Keep the selection for this visit. */ }
}
const sortGroups = [
  { label: '中文名称（按拼音，无中文名置后）', options: [
    { value: 'name_asc', label: '中文名称 · 正序（A → Z）' },
    { value: 'name_desc', label: '中文名称 · 倒序（Z → A）' },
  ] },
  { label: '修改时间', options: [
    { value: 'updated_desc', label: '修改时间 · 倒序（最新在前）' },
    { value: 'updated_asc', label: '修改时间 · 正序（最早在前）' },
  ] },
  { label: '创建时间', options: [
    { value: 'created_desc', label: '创建时间 · 倒序（最新在前）' },
    { value: 'created_asc', label: '创建时间 · 正序（最早在前）' },
  ] },
] as const
type ProductSort = (typeof sortGroups)[number]['options'][number]['value']
const sort = ref<ProductSort>('created_desc')
try {
  const stored = localStorage.getItem('com-itemgather:product-sort')
  if (sortGroups.some(group => group.options.some(option => option.value === stored))) sort.value = stored as ProductSort
} catch { /* Use the default when storage is unavailable. */ }
const filterOptions = ref<ProductFilterOptions>({ companies: [], categories: [], attributes: [], statuses: [{value:'concept',label:'构思中'},{value:'design',label:'设计中'},{value:'production',label:'制作中'},{value:'completed',label:'已完成'}] })
const filters = ref<Record<string, string[]>>({ company: [], category: [], attribute: [], status: [] })
const filterGroups = computed(() => [
  { key: 'company', label: '所属公司', allLabel: '全部公司', options: filterOptions.value.companies },
  { key: 'category', label: '商品分类', allLabel: '全部分类', options: filterOptions.value.categories },
  { key: 'attribute', label: '动态属性分类', allLabel: '全部属性分类', options: filterOptions.value.attributes },
  { key: 'status', label: '制作状态', allLabel: '全部状态', options: filterOptions.value.statuses },
])
const activeFilters = computed(() => filterGroups.value.flatMap(group => filters.value[group.key].map(value => ({
  key: group.key,
  value,
  label: `${group.label}：${group.options.map(option => typeof option === 'string' ? { value: option, label: option } : option).find(option => option.value === value)?.label || value}`,
}))))
const hasConditions = computed(() => !!query.value.trim() || activeFilters.value.length > 0 || !!route.query.company_id)
function removeFilter(key: string, value: string) { filters.value = { ...filters.value, [key]: filters.value[key].filter(item => item !== value) } }
const loadMoreTrigger = ref<HTMLElement>()
let observer: IntersectionObserver | undefined
let requestID = 0
let requestController: AbortController | undefined
let searchTimer: ReturnType<typeof setTimeout> | undefined

async function load(reset = true) {
  if (!reset && (searchTimer || loading.value || loadingMore.value || products.value.length >= total.value)) return
  if (reset) {
    clearTimeout(searchTimer)
    searchTimer = undefined
    requestID++
    requestController?.abort()
    requestController = new AbortController()
    loading.value = true
    loadingMore.value = false
  } else loadingMore.value = true
  const id = requestID
  error.value = ''
  try {
    const offset = reset ? 0 : products.value.length
    const parameters = new URLSearchParams({ q: query.value, sort: sort.value, limit: '30', offset: String(offset) })
    if (typeof route.query.company_id === 'string') parameters.set('company_id',route.query.company_id)
    for (const [key, values] of Object.entries(filters.value)) for (const value of values) parameters.append(key, value)
    const result = await api<Page<Product>>(`/products?${parameters}`, { signal: requestController?.signal })
    if (id !== requestID) return
    products.value = reset ? result.items : [...products.value, ...result.items.filter(item => !products.value.some(existing => existing.id === item.id))]
    total.value = result.total
  } catch (e) {
    if ((e as Error).name === 'AbortError') return
    if (id === requestID) error.value = (e as Error).message
  } finally {
    if (id === requestID) loading.value = false
    if (!reset && id === requestID) loadingMore.value = false
  }
  if (id === requestID && !error.value) {
    await nextTick()
    const trigger = loadMoreTrigger.value
    if (trigger && trigger.getBoundingClientRect().top <= window.innerHeight + 600) void load(false)
  }
}
function searchNow() {
  clearTimeout(searchTimer)
  searchTimer = undefined
  void load()
}
function scheduleSearch() {
  clearTimeout(searchTimer)
  requestID++
  requestController?.abort()
  loading.value = false
  loadingMore.value = false
  searchTimer = setTimeout(searchNow, 250)
}
async function loadFilters() {
  try { filterOptions.value = await api<ProductFilterOptions>('/products/filters') }
  catch (e) { error.value = (e as Error).message }
}
function refresh() { if (!searchTimer) void load(); void loadFilters() }
watch(query, scheduleSearch, { flush: 'sync' })
watch(sort, value => {
  try { localStorage.setItem('com-itemgather:product-sort', value) } catch { /* Keep the selection for this visit. */ }
  searchNow()
}, { flush: 'sync' })
watch(filters, searchNow)
watch(() => route.query.company_id, refresh)
watch(loadMoreTrigger, (element, previous) => {
  if (previous) observer?.unobserve(previous)
  if (element) observer?.observe(element)
})
onMounted(() => {
  observer = new IntersectionObserver(entries => {
    if (entries.some(entry => entry.isIntersecting)) void load(false)
  }, { rootMargin: '600px 0px' })
  if (loadMoreTrigger.value) observer.observe(loadMoreTrigger.value)
  refresh()
})
onUnmounted(() => {
  requestID++
  clearTimeout(searchTimer)
  requestController?.abort()
  observer?.disconnect()
})
useEvents(refresh)
</script>
<template>
  <section class="products-page">
    <div class="page-heading">
      <div><h1>商品库</h1><p>查看商品设定与制作进度。</p></div>
      <button class="primary" @click="showCreate = true"><svg viewBox="0 0 20 20" aria-hidden="true"><path d="M10 4v12M4 10h12" /></svg>创建商品</button>
    </div>
    <p v-if="route.query.company_id" class="muted">正在查看所选公司旗下商品 · <RouterLink to="/products">查看全部公司</RouterLink></p>
    <form class="toolbar product-toolbar" @submit.prevent="searchNow">
      <div class="product-search">
        <svg viewBox="0 0 20 20" aria-hidden="true"><circle cx="8.5" cy="8.5" r="5.5" /><path d="m13 13 4 4" /></svg>
        <input v-model="query" type="search" aria-label="搜索商品" placeholder="搜索名称、公司、分类或描述" />
      </div>
      <FilterPopover id="product-filter-popover" v-model="filters" label="商品" :groups="filterGroups" />
    </form>
    <div v-if="activeFilters.length" class="product-active-filters" aria-label="已选筛选条件">
      <button v-for="filter in activeFilters" :key="`${filter.key}:${filter.value}`" type="button" :aria-label="`移除${filter.label}筛选`" @click="removeFilter(filter.key, filter.value)">{{ filter.label }}<span aria-hidden="true">×</span></button>
    </div>
    <div class="product-results-bar">
      <p class="product-result-count" role="status">{{ loading ? '正在读取…' : `${hasConditions ? '找到' : '共'} ${total} 件商品` }}<span v-if="!loading && products.length < total"> · 已显示 {{ products.length }} 件</span></p>
      <div class="product-browse-controls">
        <label class="product-sort">
          <span>排序</span>
          <select v-model="sort" aria-label="商品排序">
            <optgroup v-for="group in sortGroups" :key="group.label" :label="group.label">
              <option v-for="option in group.options" :key="option.value" :value="option.value">{{ option.label }}</option>
            </optgroup>
          </select>
        </label>
        <div class="product-view-switch" role="group" aria-label="商品展示方式">
          <button type="button" :aria-pressed="layout === 'list'" @click="changeLayout('list')"><svg viewBox="0 0 20 20" aria-hidden="true"><path d="M7 5h10M7 10h10M7 15h10M3 5h.5M3 10h.5M3 15h.5" /></svg>列表</button>
          <button type="button" :aria-pressed="layout === 'grid'" @click="changeLayout('grid')"><svg viewBox="0 0 20 20" aria-hidden="true"><rect x="3" y="3" width="5" height="5" rx="1" /><rect x="12" y="3" width="5" height="5" rx="1" /><rect x="3" y="12" width="5" height="5" rx="1" /><rect x="12" y="12" width="5" height="5" rx="1" /></svg>卡片</button>
        </div>
      </div>
    </div>
    <p v-if="error" class="error-text" role="alert">{{ error }} <button class="button-link" @click="load(products.length === 0)">重试</button></p>
    <p v-if="loading && !products.length" class="muted">读取中…</p>
    <div v-else-if="products.length" class="product-catalog" :class="`is-${layout}`" :aria-busy="loading">
      <div v-if="layout === 'list'" class="product-list-header" aria-hidden="true"><span>商品名称</span><span>所属公司</span><span>商品分类</span><span>制作状态</span></div>
      <ProductCatalogItem v-for="product in products" :key="product.id" :product="product" :layout="layout" />
    </div>
    <div v-else class="empty-state">{{ hasConditions ? '没有找到符合条件的商品，试试其他关键词或筛选条件。' : '还没有商品。可以在这里创建，也可以从一条灵感主动创建。' }}</div>
    <div v-if="products.length < total" ref="loadMoreTrigger" class="load-more-status muted" role="status">{{ loadingMore ? '正在读取…' : '' }}</div>
  </section>
  <ItemCreateDialog v-if="showCreate" kind="product" @created="showCreate = false; refresh()" @cancel="showCreate = false" />
</template>

<style scoped>
.page-heading .primary{gap:.4rem}
svg{width:1.1rem;height:1.1rem;fill:none;stroke:currentColor;stroke-width:1.6;stroke-linecap:round;stroke-linejoin:round;flex:none}
.product-toolbar{flex-wrap:nowrap;gap:.65rem;margin:1.5rem 0 0}
.product-search{position:relative;flex:1;min-width:0}
.product-search>svg{position:absolute;left:.85rem;top:50%;transform:translateY(-50%);color:var(--muted);pointer-events:none}
.product-search>input{max-width:none;padding-left:2.5rem;min-height:2.75rem}
.product-toolbar :deep(.filter-control){margin-left:0;flex:none}
.product-toolbar :deep(.filter-button){min-height:2.75rem}
.product-active-filters{display:flex;gap:.4rem;flex-wrap:wrap;margin-top:.75rem}
.product-active-filters button{display:inline-flex;align-items:center;gap:.55rem;max-width:100%;padding:.35rem .6rem;border:1px solid var(--border);border-radius:6px;background:var(--surface);color:var(--muted);font-size:.78rem;text-align:left;overflow-wrap:anywhere}
.product-active-filters button:hover{color:var(--accent);border-color:var(--accent)}
.product-active-filters button>span{font-size:1rem;flex:none}
.product-results-bar{display:flex;align-items:center;justify-content:space-between;gap:.7rem;flex-wrap:wrap;margin:1rem 0 .75rem}
.product-result-count{margin:0;color:var(--muted);font-size:.82rem;line-height:1.5}
.product-result-count>span{font-size:.75rem}
.product-browse-controls{display:flex;align-items:center;gap:.75rem;margin-left:auto;min-width:0}
.product-sort{display:flex;align-items:center;gap:.5rem;color:var(--muted);font-size:.78rem;min-width:0}
.product-sort>span{flex:none}
.product-sort>select{min-width:0;max-width:100%;padding:.45rem .55rem;font-size:.78rem;min-height:2.2rem;color:var(--text)}
.product-view-switch{display:flex;gap:.2rem;padding:.2rem;border:1px solid var(--border);border-radius:7px;background:var(--surface-soft);flex:none}
.product-view-switch button{display:flex;align-items:center;gap:.35rem;padding:.35rem .6rem;border-radius:4px;background:transparent;color:var(--muted);font-size:.78rem}
.product-view-switch button[aria-pressed=true]{background:var(--surface);color:var(--text);box-shadow:0 1px 3px #0001}
.product-view-switch button:hover{color:var(--text)}
.product-view-switch svg{width:.95rem;height:.95rem}
.product-catalog{--catalog-columns:minmax(0,2.3fr) minmax(0,1.2fr) minmax(0,.8fr) minmax(6rem,.8fr);--catalog-gap:1.25rem}
.product-catalog.is-list{border:1px solid var(--border);border-radius:9px;background:var(--surface)}
.product-list-header{display:grid;grid-template-columns:var(--catalog-columns);gap:var(--catalog-gap);padding:.8rem 1.1rem;background:var(--surface-soft);border-radius:8px 8px 0 0;color:var(--muted);font-size:.75rem}
.product-catalog.is-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:1rem;align-items:start}
@media(max-width:900px){.product-catalog{--catalog-gap:.8rem;--catalog-columns:minmax(0,2fr) minmax(0,1.2fr) minmax(0,.8fr) minmax(5rem,.8fr)}.product-catalog.is-grid{grid-template-columns:repeat(2,minmax(0,1fr))}}
@media(max-width:600px){.product-list-header{display:none}.product-catalog.is-grid{grid-template-columns:1fr;gap:.75rem}.product-result-count>span{display:block}.product-view-switch button{padding:.35rem .5rem}.product-toolbar{margin-top:1rem}.product-results-bar{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:.75rem}.product-result-count{grid-column:1;grid-row:1}.product-browse-controls{display:contents}.product-view-switch{grid-column:2;grid-row:1}.product-sort{grid-column:1/-1;grid-row:2}.product-sort>select{flex:1;width:100%;min-height:2.65rem}}
</style>
