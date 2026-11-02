<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import ItemCreateDialog from '../components/ItemCreateDialog.vue'
import { api } from '../api'
import type { Page, Product } from '../types'
import { productTitle, statusLabels } from '../utils'
import { useEvents } from '../events'
import ProductFilterGroup from '../components/ProductFilterGroup.vue'

interface ProductFilterOptions { companies: string[]; categories: string[]; attributes: string[] }

const route = useRoute()
const products = ref<Product[]>([]); const total = ref(0); const query = ref(''); const company = ref<string[]>([]); const category = ref<string[]>([]); const attribute = ref<string[]>([]); const status = ref<string[]>([]); const loading = ref(true); const loadingMore = ref(false); const error = ref(''); const showCreate = ref(false);
const filterOptions = ref<ProductFilterOptions>({ companies: [], categories: [], attributes: [] })
const showFilters = ref(false); const activeFilterCategory = ref('company'); const filterControl = ref<HTMLElement>(); const filterPopover = ref<HTMLElement>(); const filterButton = ref<HTMLButtonElement>()
const activeFilterCount = computed(() => [company.value, category.value, attribute.value, status.value].filter(values => values.length > 0).length)
const statusOptions = ['concept', 'design', 'production', 'completed'].map(value => ({ value, label: statusLabels[value] }))
const loadMoreTrigger = ref<HTMLElement>()
let observer: IntersectionObserver | undefined
let requestID = 0
let requestController: AbortController | undefined
let searchTimer: ReturnType<typeof setTimeout> | undefined

async function load(reset = true) {
  if (!reset && (loading.value || loadingMore.value || products.value.length >= total.value)) return
  if (reset) {
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
    const parameters = new URLSearchParams({ q: query.value, limit: '30', offset: String(offset) })
    if (typeof route.query.company_id === 'string') parameters.set('company_id',route.query.company_id)
    const selectedFilters = { company: company.value, category: category.value, attribute: attribute.value, status: status.value }
    for (const [key, values] of Object.entries(selectedFilters)) for (const value of values) parameters.append(key, value)
    const result = await api<Page<Product>>(`/products?${parameters}`, { signal: requestController?.signal })
    if (id !== requestID) return
    products.value = reset ? result.items : [...products.value, ...result.items.filter(item => !products.value.some(existing => existing.id === item.id))]
    total.value = result.total
  } catch (e) {
    if ((e as Error).name === 'AbortError') return
    if (id === requestID) error.value = (e as Error).message
  } finally {
    if (id === requestID) loading.value = false
    if (!reset) loadingMore.value = false
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
  searchTimer = setTimeout(searchNow, 250)
}
async function loadFilters() {
  try { filterOptions.value = await api<ProductFilterOptions>('/products/filters') }
  catch (e) { error.value = (e as Error).message }
}
function refresh() { void load(); void loadFilters() }
async function toggleFilters() {
  showFilters.value = !showFilters.value
  if (showFilters.value) {
    await nextTick()
    filterPopover.value?.querySelector('input')?.focus()
  }
}
function clearFilters() { company.value = []; category.value = []; attribute.value = []; status.value = []; void load() }
function closeOnOutside(event: MouseEvent) { if (showFilters.value && !filterControl.value?.contains(event.target as Node)) showFilters.value = false }
function closeOnEscape(event: KeyboardEvent) { if (event.key === 'Escape' && showFilters.value) { showFilters.value = false; filterButton.value?.focus() } }

watch(query, scheduleSearch)
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
  document.addEventListener('click', closeOnOutside)
  document.addEventListener('keydown', closeOnEscape)
  refresh()
})
onUnmounted(() => {
  requestID++
  clearTimeout(searchTimer)
  requestController?.abort()
  observer?.disconnect()
  document.removeEventListener('click', closeOnOutside)
  document.removeEventListener('keydown', closeOnEscape)
})
useEvents(refresh)
</script>
<template><section><div class="page-heading"><div><h1>商品库</h1><p>团队已经开始考虑实际制作的具体游戏内商品。</p></div><button class="primary" @click="showCreate = true">创建商品</button></div><p v-if="route.query.company_id" class="muted">正在查看所选公司旗下商品 · <RouterLink to="/products">查看全部公司</RouterLink></p><form class="toolbar" @submit.prevent="searchNow"><input v-model="query" type="search" aria-label="搜索商品" placeholder="搜索名称、公司、分类或描述" /><div ref="filterControl" class="filter-control"><button ref="filterButton" type="button" class="secondary filter-button" :class="{ active: activeFilterCount }" aria-label="筛选商品" :aria-expanded="showFilters" aria-controls="product-filter-popover" @click="toggleFilters"><svg class="filter-icon" viewBox="0 0 24 24" aria-hidden="true"><path fill="currentColor" d="M3 4h18l-7 8v6l-4 2v-8L3 4Z" /></svg><span v-if="activeFilterCount" class="filter-count">{{ activeFilterCount }}</span></button><div v-if="showFilters" id="product-filter-popover" ref="filterPopover" class="filter-popover" role="dialog" aria-label="商品筛选"><div class="filter-popover-layout"><nav class="filter-category-list" aria-label="筛选分类"><button type="button" class="filter-category" :class="{ active: activeFilterCategory === 'company' }" :aria-pressed="activeFilterCategory === 'company'" @click="activeFilterCategory = 'company'"><span>所属公司</span><span v-if="company.length" class="filter-category-dot" aria-hidden="true"></span></button><button type="button" class="filter-category" :class="{ active: activeFilterCategory === 'category' }" :aria-pressed="activeFilterCategory === 'category'" @click="activeFilterCategory = 'category'"><span>商品分类</span><span v-if="category.length" class="filter-category-dot" aria-hidden="true"></span></button><button type="button" class="filter-category" :class="{ active: activeFilterCategory === 'attribute' }" :aria-pressed="activeFilterCategory === 'attribute'" @click="activeFilterCategory = 'attribute'"><span>动态属性分类</span><span v-if="attribute.length" class="filter-category-dot" aria-hidden="true"></span></button><button type="button" class="filter-category" :class="{ active: activeFilterCategory === 'status' }" :aria-pressed="activeFilterCategory === 'status'" @click="activeFilterCategory = 'status'"><span>制作状态</span><span v-if="status.length" class="filter-category-dot" aria-hidden="true"></span></button></nav><div class="filter-options-pane"><ProductFilterGroup v-if="activeFilterCategory === 'company'" v-model="company" label="所属公司" all-label="全部公司" :options="filterOptions.companies" @update:model-value="load()" /><ProductFilterGroup v-else-if="activeFilterCategory === 'category'" v-model="category" label="商品分类" all-label="全部分类" :options="filterOptions.categories" @update:model-value="load()" /><ProductFilterGroup v-else-if="activeFilterCategory === 'attribute'" v-model="attribute" label="动态属性分类" all-label="全部属性分类" :options="filterOptions.attributes" @update:model-value="load()" /><ProductFilterGroup v-else v-model="status" label="制作状态" all-label="全部状态" :options="statusOptions" @update:model-value="load()" /></div></div><div class="filter-popover-actions"><button type="button" class="button-link" :disabled="!activeFilterCount" @click="clearFilters">清除筛选</button><button type="button" class="secondary small" @click="showFilters = false">完成</button></div></div></div></form><p v-if="error" class="error-text">{{ error }} <button class="button-link" @click="load(products.length === 0)">重试</button></p><p v-if="loading && !products.length" class="muted">读取中…</p><div v-else-if="products.length" class="product-grid"><article v-for="product in products" :key="product.id" class="product-card"><img v-if="product.attachments.length" class="product-cover" :src="product.attachments[0].thumbnail_url || product.attachments[0].url" alt="" loading="lazy" /><div class="product-card-content"><h2><RouterLink :to="`/products/${product.id}`">{{ productTitle(product) }}</RouterLink></h2><p v-if="product.english_name && product.chinese_name" class="product-english-name">{{ product.english_name }}</p><p v-if="product.company" class="product-company"><RouterLink v-if="product.company_id" :to="`/companies/${product.company_id}`">{{ product.company }}</RouterLink><span v-else>{{ product.company }}</span></p><div class="product-meta"><span v-if="product.category">分类：{{ product.category }}</span><span class="status-badge">{{ statusLabels[product.development_status] }}</span></div></div></article></div><div v-else class="empty-state">还没有商品。可以在这里创建，也可以从一条灵感主动创建。</div><div v-if="products.length < total" ref="loadMoreTrigger" class="load-more-status muted" role="status">{{ loadingMore ? '正在读取…' : '' }}</div></section>
  <ItemCreateDialog v-if="showCreate" kind="product" @created="showCreate = false; refresh()" @cancel="showCreate = false" />
</template>
