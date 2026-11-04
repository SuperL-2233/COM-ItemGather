<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import ItemCreateDialog from '../components/ItemCreateDialog.vue'
import ProductCatalogItem from '../components/ProductCatalogItem.vue'
import ProductBatchEditDialog from '../components/ProductBatchEditDialog.vue'
import { api } from '../api'
import type { ChoiceOption, Page, Product } from '../types'
import { useEvents } from '../events'
import FilterPopover from '../components/FilterPopover.vue'
import { toast } from '../utils'
import { loadListPages, useListNavigation } from '../composables/listNavigation'

interface ProductFilterOptions { companies: string[]; categories: string[]; attributes: string[]; statuses: ChoiceOption[] }

const route = useRoute()
type ProductListState = { items: Product[]; total: number; query: string; filters: Record<string, string[]>; layout: 'list' | 'grid'; sort: ProductSort }
const previous: ProductListState | undefined = useListNavigation<ProductListState>(() => ({ items: products.value, total: total.value, query: query.value, filters: filters.value, layout: layout.value, sort: sort.value }))
const products = ref<Product[]>(previous?.items || []); const total = ref(previous?.total || 0); const query = ref(previous?.query || ''); const loading = ref(true); const loadingMore = ref(false); const error = ref(''); const showCreate = ref(false);
const searchPending = ref(false)
const selectionLocked = computed(() => loading.value || searchPending.value)
const selecting = ref(false)
const selected = ref(new Map<string, Product>())
const batchProducts = ref<Product[]>()
const selectionTrigger = ref<HTMLButtonElement>()
const catalog = ref<HTMLElement>()
const allDisplayedSelected = computed(() => products.value.length > 0 && products.value.every(item => selected.value.has(item.id)))
function startSelection() { closeSort(); selecting.value = true; void nextTick(() => catalog.value?.querySelector<HTMLInputElement>('.catalog-select input')?.focus()) }
function finishSelection() {
  selecting.value = false
  selected.value = new Map()
  batchProducts.value = undefined
  void nextTick(() => selectionTrigger.value?.focus())
}
function clearSelection() { selected.value = new Map(); batchProducts.value = undefined }
function toggleProduct(product: Product) {
  const next = new Map(selected.value)
  if (next.has(product.id)) next.delete(product.id)
  else {
    if (next.size >= 500) { toast('每次最多选择 500 件商品'); return }
    next.set(product.id, product)
  }
  selected.value = next
}
function toggleDisplayed() {
  const next = new Map(selected.value)
  if (allDisplayedSelected.value) for (const item of products.value) next.delete(item.id)
  else {
    if (new Set([...next.keys(), ...products.value.map(item => item.id)]).size > 500) { toast('每次最多选择 500 件商品，请减少选择后重试'); return }
    for (const item of products.value) if (!next.has(item.id)) next.set(item.id, item)
  }
  selected.value = next
}
function batchSaved(count: number) { finishSelection(); refresh(); toast(`已更新 ${count} 件商品`) }
function reselectProducts() { clearSelection(); refresh() }
const layout = ref<'list' | 'grid'>('list')
try { if (localStorage.getItem('com-itemgather:product-layout') === 'grid') layout.value = 'grid' } catch { /* Storage may be unavailable. */ }
if (previous) layout.value = previous.layout
function changeLayout(value: 'list' | 'grid') {
  layout.value = value
  try { localStorage.setItem('com-itemgather:product-layout', value) } catch { /* Keep the selection for this visit. */ }
}
const sortGroups = [
  { label: '名称', options: [
    { value: 'name_asc', label: '正序' },
    { value: 'name_desc', label: '倒序' },
  ] },
  { label: '修改时间', options: [
    { value: 'updated_desc', label: '倒序' },
    { value: 'updated_asc', label: '正序' },
  ] },
  { label: '创建时间', options: [
    { value: 'created_desc', label: '倒序' },
    { value: 'created_asc', label: '正序' },
  ] },
] as const
type ProductSort = (typeof sortGroups)[number]['options'][number]['value']
const sortOptions = sortGroups.flatMap(group => group.options.map(option => ({ ...option, group: group.label })))
const sort = ref<ProductSort>('created_desc')
const sortOpen = ref(false)
const sortControl = ref<HTMLElement>()
const sortTrigger = ref<HTMLButtonElement>()
const activeSort = ref(0)
const selectedSort = computed(() => sortOptions.find(option => option.value === sort.value)!)
try {
  const stored = localStorage.getItem('com-itemgather:product-sort')
  if (sortGroups.some(group => group.options.some(option => option.value === stored))) sort.value = stored as ProductSort
} catch { /* Use the default when storage is unavailable. */ }
if (previous) sort.value = previous.sort
function focusActiveSort() {
  document.getElementById(`product-sort-${sortOptions[activeSort.value].value}`)?.focus()
}
async function openSort() {
  activeSort.value = Math.max(0, sortOptions.findIndex(option => option.value === sort.value))
  sortOpen.value = true
  await nextTick()
  focusActiveSort()
}
function closeSort(restoreFocus = false) {
  sortOpen.value = false
  if (restoreFocus) void nextTick(() => sortTrigger.value?.focus())
}
function chooseSort(value: ProductSort) {
  sort.value = value
  closeSort(true)
}
function moveSort(delta: number) {
  activeSort.value = (activeSort.value + delta + sortOptions.length) % sortOptions.length
  void nextTick(focusActiveSort)
}
function sortKeydown(event: KeyboardEvent) {
  if (event.isComposing || event.keyCode === 229) return
  if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
    event.preventDefault()
    event.stopPropagation()
    if (sortOpen.value) moveSort(event.key === 'ArrowDown' ? 1 : -1)
    else void openSort()
  } else if (sortOpen.value && (event.key === 'Home' || event.key === 'End')) {
    event.preventDefault()
    activeSort.value = event.key === 'Home' ? 0 : sortOptions.length - 1
    void nextTick(focusActiveSort)
  } else if (sortOpen.value && (event.key === 'Enter' || event.key === ' ') && event.target !== sortTrigger.value) {
    event.preventDefault()
    chooseSort(sortOptions[activeSort.value].value)
  } else if (sortOpen.value && event.key === 'Escape') {
    event.preventDefault()
    event.stopPropagation()
    closeSort(true)
  } else if (sortOpen.value && event.key === 'Tab') closeSort()
}
function sortOutside(event: MouseEvent) {
  if (sortOpen.value && sortControl.value && !event.composedPath().includes(sortControl.value)) closeSort()
}
function sortFocusOut(event: FocusEvent) {
  if (sortOpen.value && event.relatedTarget && !sortControl.value?.contains(event.relatedTarget as Node)) closeSort()
}
const filterOptions = ref<ProductFilterOptions>({ companies: [], categories: [], attributes: [], statuses: [{value:'concept',label:'构思中'},{value:'design',label:'设计中'},{value:'production',label:'制作中'},{value:'completed',label:'已完成'}] })
const filters = ref<Record<string, string[]>>(previous?.filters || { company: [], category: [], attribute: [], status: [] })
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

async function load(reset = true, preserveDepth = false) {
  if (!reset && (searchTimer || loading.value || loadingMore.value || products.value.length >= total.value)) return
  if (reset) {
    clearTimeout(searchTimer)
    searchTimer = undefined
    searchPending.value = false
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
    const signal = requestController?.signal
    const fetchPage = (offset: number, limit: number) => {
      parameters.set('offset', String(offset)); parameters.set('limit', String(limit))
      return api<Page<Product>>(`/products?${parameters}`, { signal })
    }
    const result = reset && preserveDepth
      ? await loadListPages(Math.max(30, products.value.length), fetchPage)
      : await fetchPage(offset, 30)
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
  searchPending.value = false
  void load()
}
function scheduleSearch() {
  clearTimeout(searchTimer)
  searchPending.value = true
  requestID++
  requestController?.abort()
  loading.value = false
  loadingMore.value = false
  searchTimer = setTimeout(searchNow, 250)
}
async function loadFilters() {
  try {
    filterOptions.value = await api<ProductFilterOptions>('/products/filters')
    const active = filters.value.status.filter(value => filterOptions.value.statuses.some(option => option.value === value))
    if (active.length !== filters.value.status.length) filters.value = { ...filters.value, status: active }
  }
  catch (e) { error.value = (e as Error).message }
}
function refresh() { if (!searchTimer) void load(true, true); void loadFilters() }
watch(query, () => { clearSelection(); scheduleSearch() }, { flush: 'sync' })
watch(sort, value => {
  try { localStorage.setItem('com-itemgather:product-sort', value) } catch { /* Keep the selection for this visit. */ }
  searchNow()
}, { flush: 'sync' })
watch(filters, () => { clearSelection(); searchNow() })
watch(() => route.query.company_id, () => { clearSelection(); refresh() })
watch(loadMoreTrigger, (element, previous) => {
  if (previous) observer?.unobserve(previous)
  if (element) observer?.observe(element)
})
onMounted(() => {
  observer = new IntersectionObserver(entries => {
    if (entries.some(entry => entry.isIntersecting)) void load(false)
  }, { rootMargin: '600px 0px' })
  if (loadMoreTrigger.value) observer.observe(loadMoreTrigger.value)
  document.addEventListener('click', sortOutside)
  refresh()
})
onUnmounted(() => {
  requestID++
  clearTimeout(searchTimer)
  requestController?.abort()
  observer?.disconnect()
  document.removeEventListener('click', sortOutside)
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
    <div v-if="selecting" class="product-selection-bar" role="region" aria-label="商品多选操作">
      <p role="status">已选 <strong>{{ selected.size }}</strong> 件商品</p>
      <button type="button" class="secondary small select-displayed" :disabled="selectionLocked || !products.length" @click="toggleDisplayed">{{ allDisplayedSelected ? '取消全选' : '全选已显示' }}</button>
      <button type="button" class="primary small edit-selected" :disabled="!selected.size || selectionLocked" @click="batchProducts = [...selected.values()]">批量修改</button>
      <button type="button" class="text-button cancel-selection" @click="finishSelection">取消多选</button>
    </div>
    <div v-else class="product-results-bar">
      <div class="product-result-summary"><p class="product-result-count" role="status">{{ loading ? '正在读取…' : `${hasConditions ? '找到' : '共'} ${total} 件商品` }}<span v-if="!loading && products.length < total"> · 已显示 {{ products.length }} 件</span></p><button v-if="products.length" ref="selectionTrigger" type="button" class="product-select-toggle" :disabled="selectionLocked" @click="startSelection"><svg viewBox="0 0 20 20" aria-hidden="true"><rect x="3.5" y="3.5" width="13" height="13" rx="2" /><path d="m6.5 10 2.5 2.5 4.5-5" /></svg>多选</button></div>
      <div class="product-browse-controls">
        <div ref="sortControl" class="product-sort" @keydown="sortKeydown" @focusout="sortFocusOut">
          <span>排序</span>
          <button ref="sortTrigger" type="button" class="product-sort-trigger" role="combobox" aria-label="商品排序" aria-haspopup="listbox" aria-controls="product-sort-options" :aria-expanded="sortOpen" :data-value="sort" @click="sortOpen ? closeSort() : openSort()">
            <span>{{ selectedSort.group }} · {{ selectedSort.label }}</span>
            <svg viewBox="0 0 20 20" aria-hidden="true"><path d="m6 8 4 4 4-4" /></svg>
          </button>
          <div v-if="sortOpen" id="product-sort-options" class="product-sort-menu" role="listbox" aria-label="商品排序方式">
            <div v-for="group in sortGroups" :key="group.label" role="group" :aria-label="group.label">
              <span class="product-sort-group" aria-hidden="true">{{ group.label }}</span>
              <button v-for="option in group.options" :id="`product-sort-${option.value}`" :key="option.value" type="button" role="option" tabindex="-1" class="product-sort-option" :class="{ 'is-active': sortOptions[activeSort].value === option.value }" :aria-label="`${group.label} · ${option.label}`" :aria-selected="sort === option.value" @click="chooseSort(option.value)" @mouseenter="activeSort = sortOptions.findIndex(item => item.value === option.value)">
                <span>{{ option.label }}</span>
                <svg v-if="sort === option.value" viewBox="0 0 20 20" aria-hidden="true"><path d="m5 10 3 3 7-7" /></svg>
              </button>
            </div>
          </div>
        </div>
        <div class="product-view-switch" role="group" aria-label="商品展示方式">
          <button type="button" :aria-pressed="layout === 'list'" @click="changeLayout('list')"><svg viewBox="0 0 20 20" aria-hidden="true"><path d="M7 5h10M7 10h10M7 15h10M3 5h.5M3 10h.5M3 15h.5" /></svg>列表</button>
          <button type="button" :aria-pressed="layout === 'grid'" @click="changeLayout('grid')"><svg viewBox="0 0 20 20" aria-hidden="true"><rect x="3" y="3" width="5" height="5" rx="1" /><rect x="12" y="3" width="5" height="5" rx="1" /><rect x="3" y="12" width="5" height="5" rx="1" /><rect x="12" y="12" width="5" height="5" rx="1" /></svg>卡片</button>
        </div>
      </div>
    </div>
    <p v-if="error" class="error-text" role="alert">{{ error }} <button class="button-link" @click="load(products.length === 0)">重试</button></p>
    <p v-if="loading && !products.length" class="muted">读取中…</p>
    <div v-else-if="products.length" ref="catalog" class="product-catalog" :class="`is-${layout}`" :aria-busy="loading">
      <div v-if="layout === 'list'" class="product-list-header" aria-hidden="true"><span>商品名称</span><span>所属公司</span><span>商品分类</span><span>制作状态</span></div>
      <ProductCatalogItem v-for="product in products" :key="product.id" :product="product" :layout="layout" :selectable="selecting" :selected="selected.has(product.id)" :selection-disabled="selectionLocked" @toggle="toggleProduct(product)" />
    </div>
    <div v-else class="empty-state">{{ hasConditions ? '没有找到符合条件的商品，试试其他关键词或筛选条件。' : '还没有商品。可以在这里创建，也可以从一条灵感主动创建。' }}</div>
    <div v-if="products.length < total" ref="loadMoreTrigger" class="load-more-status muted" role="status">{{ loadingMore ? '正在读取…' : '' }}</div>
  </section>
  <ItemCreateDialog v-if="showCreate" kind="product" @created="showCreate = false; refresh()" @cancel="showCreate = false" />
  <ProductBatchEditDialog v-if="batchProducts" :products="batchProducts" @saved="batchSaved" @cancel="batchProducts = undefined" @reselect="reselectProducts" />
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
.product-result-summary{display:flex;align-items:center;gap:.65rem;min-width:0}
.product-select-toggle{display:inline-flex;align-items:center;gap:.3rem;flex:none;min-height:2.2rem;padding:.35rem .45rem;border-radius:5px;background:transparent;color:var(--muted);font-size:.78rem}
.product-select-toggle:hover{background:var(--surface-soft);color:var(--accent)}
.product-select-toggle svg{width:1rem;height:1rem}
.product-selection-bar{display:flex;align-items:center;gap:.6rem;margin:1rem 0 .75rem;padding:.65rem .8rem;background:color-mix(in srgb,var(--accent) 5%,var(--surface));border:1px solid var(--focus);border-radius:8px}
.product-selection-bar p{margin:0 auto 0 0;font-size:.85rem;color:var(--muted)}
.product-selection-bar strong{color:var(--text)}
.product-selection-bar button{font-size:.8rem}
.cancel-selection{white-space:nowrap}
.product-result-count{margin:0;color:var(--muted);font-size:.82rem;line-height:1.5}
.product-result-count>span{font-size:.75rem}
.product-browse-controls{display:flex;align-items:center;gap:.75rem;margin-left:auto;min-width:0}
.product-sort{position:relative;display:flex;align-items:center;gap:.5rem;color:var(--muted);font-size:.78rem;min-width:0}
.product-sort>span{flex:none}
.product-sort-trigger{display:flex;align-items:center;justify-content:space-between;gap:.75rem;min-width:0;min-height:2.2rem;padding:.45rem .6rem;border:1px solid var(--select-border);border-radius:7px;background:var(--select-bg);color:var(--text);font-size:.78rem;white-space:nowrap}
.product-sort-trigger:hover{border-color:var(--menu-indicator);background:var(--select-hover)}
.product-sort-trigger:focus-visible,.product-sort-trigger[aria-expanded=true]{border-color:var(--menu-indicator);outline:2px solid var(--menu-indicator);outline-offset:2px}
.product-sort-trigger>svg{width:1rem;height:1rem;stroke:var(--menu-indicator);stroke-width:2;transition:transform .15s}
.product-sort-trigger[aria-expanded=true]>svg{transform:rotate(180deg)}
.product-sort-menu{position:absolute;top:calc(100% + .35rem);right:0;z-index:40;min-width:100%;padding:.35rem;border:1px solid var(--menu-border);border-radius:9px;background:var(--menu-bg);color:var(--menu-text);box-shadow:var(--menu-shadow)}
.product-sort-menu>[role=group]+[role=group]{border-top:1px solid var(--menu-divider);margin-top:.25rem}
.product-sort-group{display:block;padding:.45rem .55rem .25rem;color:var(--menu-muted);font-size:.72rem}
.product-sort-option{display:flex;align-items:center;justify-content:space-between;gap:1rem;width:100%;min-width:9rem;padding:.55rem .65rem;border-radius:5px;background:transparent;color:var(--menu-text);font-size:.82rem;text-align:left}
.product-sort-option:hover,.product-sort-option.is-active{background:var(--menu-hover);box-shadow:inset 3px 0 var(--menu-indicator)}
.product-sort-option[aria-selected=true]{background:var(--menu-selected);color:var(--menu-selected-text);box-shadow:inset 3px 0 var(--menu-indicator);font-weight:600}
.product-sort-option[aria-selected=true]:hover,.product-sort-option[aria-selected=true].is-active{background:var(--menu-selected-hover)}
.product-sort-option>svg{width:.95rem;height:.95rem}
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
@media(max-width:600px){.product-list-header{display:none}.product-catalog.is-grid{grid-template-columns:1fr;gap:.75rem}.product-result-count>span{display:block}.product-view-switch button{padding:.35rem .5rem}.product-toolbar{margin-top:1rem}.product-results-bar{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:.75rem}.product-result-count{grid-column:1;grid-row:1}.product-browse-controls{display:contents}.product-view-switch{grid-column:2;grid-row:1}.product-sort{grid-column:1/-1;grid-row:2;width:100%}.product-sort-trigger{flex:1;width:100%;min-height:2.65rem}.product-sort-menu{left:0;right:0}.product-sort-option{min-width:0;min-height:2.65rem}}
@media(max-width:600px){.product-result-summary{grid-column:1;grid-row:1;flex-wrap:wrap;gap:.15rem .4rem}.product-selection-bar{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:.6rem}.product-selection-bar p{grid-column:1;grid-row:1}.cancel-selection{grid-column:2;grid-row:1;justify-self:end}.select-displayed{grid-column:1;grid-row:2}.edit-selected{grid-column:2;grid-row:2;min-width:6rem}}
</style>
