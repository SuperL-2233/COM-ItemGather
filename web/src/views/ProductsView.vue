<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { api } from '../api'
import type { Page, Product } from '../types'
import { productTitle, statusLabels } from '../utils'
import { useEvents } from '../events'
import ProductFilterGroup from '../components/ProductFilterGroup.vue'

interface ProductFilterOptions { companies: string[]; categories: string[]; attributes: string[] }

const products = ref<Product[]>([]); const total = ref(0); const query = ref(''); const company = ref(''); const category = ref(''); const attribute = ref(''); const status = ref(''); const loading = ref(true); const loadingMore = ref(false); const error = ref(''); const showCreate = ref(false); const busy = ref(false)
const filterOptions = ref<ProductFilterOptions>({ companies: [], categories: [], attributes: [] })
const showFilters = ref(false); const filterControl = ref<HTMLElement>(); const filterPopover = ref<HTMLElement>(); const filterButton = ref<HTMLButtonElement>()
const activeFilterCount = computed(() => [company.value, category.value, attribute.value, status.value].filter(Boolean).length)
const statusOptions = ['concept', 'design', 'production', 'completed'].map(value => ({ value, label: statusLabels[value] }))
const loadMoreTrigger = ref<HTMLElement>()
const form = ref({ chinese_name: '', english_name: '', company: '', category: '', description: '', development_status: 'concept', attributes: [] })
let observer: IntersectionObserver | undefined
let requestID = 0

async function load(reset = true) {
  if (!reset && (loading.value || loadingMore.value || products.value.length >= total.value)) return
  const id = reset ? ++requestID : requestID
  error.value = ''
  if (reset) { loading.value = true; loadingMore.value = false }
  else loadingMore.value = true
  try {
    const offset = reset ? 0 : products.value.length
    const parameters = new URLSearchParams({ q: query.value, company: company.value, category: category.value, attribute: attribute.value, status: status.value, limit: '30', offset: String(offset) })
    const result = await api<Page<Product>>(`/products?${parameters}`)
    if (id !== requestID) return
    products.value = reset ? result.items : [...products.value, ...result.items.filter(item => !products.value.some(existing => existing.id === item.id))]
    total.value = result.total
  } catch (e) {
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
async function loadFilters() {
  try { filterOptions.value = await api<ProductFilterOptions>('/products/filters') }
  catch (e) { error.value = (e as Error).message }
}
function refresh() { void load(); void loadFilters() }
async function create() { busy.value = true; try { await api<Product>('/products', { method: 'POST', body: JSON.stringify(form.value) }); showCreate.value = false; form.value = { chinese_name: '', english_name: '', company: '', category: '', description: '', development_status: 'concept', attributes: [] }; refresh() } catch (e) { error.value = (e as Error).message } finally { busy.value = false } }
async function toggleFilters() {
  showFilters.value = !showFilters.value
  if (showFilters.value) {
    await nextTick()
    filterPopover.value?.querySelector('input')?.focus()
  }
}
function clearFilters() { company.value = ''; category.value = ''; attribute.value = ''; status.value = ''; void load() }
function closeOnOutside(event: PointerEvent) { if (showFilters.value && !filterControl.value?.contains(event.target as Node)) showFilters.value = false }
function closeOnEscape(event: KeyboardEvent) { if (event.key === 'Escape' && showFilters.value) { showFilters.value = false; filterButton.value?.focus() } }

watch(loadMoreTrigger, (element, previous) => {
  if (previous) observer?.unobserve(previous)
  if (element) observer?.observe(element)
})
onMounted(() => {
  observer = new IntersectionObserver(entries => {
    if (entries.some(entry => entry.isIntersecting)) void load(false)
  }, { rootMargin: '600px 0px' })
  if (loadMoreTrigger.value) observer.observe(loadMoreTrigger.value)
  document.addEventListener('pointerdown', closeOnOutside)
  document.addEventListener('keydown', closeOnEscape)
  refresh()
})
onUnmounted(() => {
  requestID++
  observer?.disconnect()
  document.removeEventListener('pointerdown', closeOnOutside)
  document.removeEventListener('keydown', closeOnEscape)
})
useEvents(refresh)
</script>
<template><section><div class="page-heading"><div><h1>商品库</h1><p>团队已经开始考虑实际制作的具体游戏内商品。</p></div><button class="primary" @click="showCreate = true">创建商品</button></div><form class="toolbar" @submit.prevent="load()"><input v-model="query" placeholder="搜索名称、公司、分类或描述" /><button class="secondary">搜索</button><div ref="filterControl" class="filter-control"><button ref="filterButton" type="button" class="secondary filter-button" :class="{ active: activeFilterCount }" aria-label="筛选商品" :aria-expanded="showFilters" aria-controls="product-filter-popover" @click="toggleFilters"><svg class="filter-icon" viewBox="0 0 24 24" aria-hidden="true"><path fill="currentColor" d="M3 4h18l-7 8v6l-4 2v-8L3 4Z" /></svg><span v-if="activeFilterCount" class="filter-count">{{ activeFilterCount }}</span></button><div v-if="showFilters" id="product-filter-popover" ref="filterPopover" class="filter-popover" role="dialog" aria-label="商品筛选"><ProductFilterGroup v-model="company" label="所属公司" all-label="全部公司" :options="filterOptions.companies" @update:model-value="load()" /><ProductFilterGroup v-model="category" label="商品分类" all-label="全部分类" :options="filterOptions.categories" @update:model-value="load()" /><ProductFilterGroup v-model="attribute" label="动态属性分类" all-label="全部属性分类" :options="filterOptions.attributes" @update:model-value="load()" /><ProductFilterGroup v-model="status" label="制作状态" all-label="全部状态" :options="statusOptions" @update:model-value="load()" /><div class="filter-popover-actions"><button type="button" class="button-link" :disabled="!activeFilterCount" @click="clearFilters">清除筛选</button><button type="button" class="secondary small" @click="showFilters = false">完成</button></div></div></div></form><p v-if="error" class="error-text">{{ error }} <button class="button-link" @click="load(products.length === 0)">重试</button></p><p v-if="loading" class="muted">读取中…</p><div v-else-if="products.length" class="product-grid"><article v-for="product in products" :key="product.id" class="product-card"><img v-if="product.attachments.length" class="product-cover" :src="product.attachments[0].thumbnail_url || product.attachments[0].url" alt="" loading="lazy" /><div class="product-card-content"><h2><RouterLink :to="`/products/${product.id}`">{{ productTitle(product) }}</RouterLink></h2><p v-if="product.english_name && product.chinese_name" class="product-english-name">{{ product.english_name }}</p><p v-if="product.company" class="product-company">{{ product.company }}</p><div class="product-meta"><span v-if="product.category">分类：{{ product.category }}</span><span class="status-badge">{{ statusLabels[product.development_status] }}</span></div></div></article></div><div v-else class="empty-state">还没有商品。可以在这里创建，也可以从一条灵感主动创建。</div><div v-if="products.length < total" ref="loadMoreTrigger" class="load-more-status muted" role="status">{{ loadingMore ? '正在读取…' : '' }}</div></section>
  <div v-if="showCreate" class="modal-backdrop"><form class="modal" @submit.prevent="create"><h2>创建商品</h2><div class="field"><label for="product-name">商品中文名（可留空）</label><input id="product-name" v-model="form.chinese_name" maxlength="160" autofocus /></div><div class="field"><label>商品英文名（可留空）</label><input v-model="form.english_name" maxlength="160" /></div><div class="field"><label>所属公司（可留空）</label><input v-model="form.company" maxlength="160" /></div><div class="field"><label>商品分类（可留空）</label><input v-model="form.category" maxlength="100" /></div><div class="field"><label>描述</label><textarea v-model="form.description" rows="5"></textarea></div><div class="field"><label>制作状态</label><select v-model="form.development_status"><option v-for="key in ['concept','design','production','completed']" :key="key" :value="key">{{ statusLabels[key] }}</option></select></div><div class="modal-actions"><button type="button" class="secondary" @click="showCreate = false">取消</button><button class="primary" :disabled="busy">创建商品</button></div></form></div>
</template>
