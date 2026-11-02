<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import ItemCreateDialog from '../components/ItemCreateDialog.vue'
import { api } from '../api'
import type { ChoiceOption, Page, Product } from '../types'
import { productTitle, developmentStatusLabel } from '../utils'
import { useEvents } from '../events'
import FilterPopover from '../components/FilterPopover.vue'

interface ProductFilterOptions { companies: string[]; categories: string[]; attributes: string[]; statuses: ChoiceOption[] }

const route = useRoute()
const products = ref<Product[]>([]); const total = ref(0); const query = ref(''); const loading = ref(true); const loadingMore = ref(false); const error = ref(''); const showCreate = ref(false);
const filterOptions = ref<ProductFilterOptions>({ companies: [], categories: [], attributes: [], statuses: [{value:'concept',label:'构思中'},{value:'design',label:'设计中'},{value:'production',label:'制作中'},{value:'completed',label:'已完成'}] })
const filters = ref<Record<string, string[]>>({ company: [], category: [], attribute: [], status: [] })
const filterGroups = computed(() => [
  { key: 'company', label: '所属公司', allLabel: '全部公司', options: filterOptions.value.companies },
  { key: 'category', label: '商品分类', allLabel: '全部分类', options: filterOptions.value.categories },
  { key: 'attribute', label: '动态属性分类', allLabel: '全部属性分类', options: filterOptions.value.attributes },
  { key: 'status', label: '制作状态', allLabel: '全部状态', options: filterOptions.value.statuses },
])
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
    const parameters = new URLSearchParams({ q: query.value, limit: '30', offset: String(offset) })
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
<template><section><div class="page-heading"><div><h1>商品库</h1><p>团队已经开始考虑实际制作的具体游戏内商品。</p></div><button class="primary" @click="showCreate = true">创建商品</button></div><p v-if="route.query.company_id" class="muted">正在查看所选公司旗下商品 · <RouterLink to="/products">查看全部公司</RouterLink></p><form class="toolbar" @submit.prevent="searchNow"><input v-model="query" type="search" aria-label="搜索商品" placeholder="搜索名称、公司、分类或描述" /><FilterPopover id="product-filter-popover" v-model="filters" label="商品" :groups="filterGroups" /></form><p v-if="error" class="error-text">{{ error }} <button class="button-link" @click="load(products.length === 0)">重试</button></p><p v-if="loading && !products.length" class="muted">读取中…</p><div v-else-if="products.length" class="product-grid"><article v-for="product in products" :key="product.id" class="product-card"><img v-if="product.attachments.length" class="product-cover" :src="product.attachments[0].thumbnail_url || product.attachments[0].url" alt="" loading="lazy" /><div class="product-card-content"><h2><RouterLink :to="`/products/${product.id}`">{{ productTitle(product) }}</RouterLink></h2><p v-if="product.english_name && product.chinese_name" class="product-english-name">{{ product.english_name }}</p><p v-if="product.company" class="product-company"><RouterLink v-if="product.company_id" :to="`/companies/${product.company_id}`">{{ product.company }}</RouterLink><span v-else>{{ product.company }}</span></p><div class="product-meta"><span v-if="product.category">分类：{{ product.category }}</span><span class="status-badge">{{ developmentStatusLabel(product) }}</span></div></div></article></div><div v-else class="empty-state">还没有商品。可以在这里创建，也可以从一条灵感主动创建。</div><div v-if="products.length < total" ref="loadMoreTrigger" class="load-more-status muted" role="status">{{ loadingMore ? '正在读取…' : '' }}</div></section>
  <ItemCreateDialog v-if="showCreate" kind="product" @created="showCreate = false; refresh()" @cancel="showCreate = false" />
</template>
