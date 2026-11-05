<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api'
import type { Company, Idea, Product, RecipeGroup } from '../types'
import IdeaCard from '../components/IdeaCard.vue'
import { companyTitle, productTitle } from '../utils'
import ItemCard from '../components/ItemCard.vue'
import FilterPopover from '../components/FilterPopover.vue'
import { useLiveSearch } from '../composables/useLiveSearch'
import { useEvents } from '../events'
import { useListNavigation } from '../composables/listNavigation'

interface SearchResult { ideas: Idea[]; products: Product[]; tools: Product[]; other_items: Product[]; companies: Company[]; recipes: RecipeGroup[]; creators: { id: string; nickname: string }[]; has_more: Record<string, boolean>; limit: number }
const route = useRoute(); const router = useRouter()
type SearchState = SearchResult & { query: string; filters: Record<string, string[]>; searched: boolean; visibleKinds: string[] }
const previous: SearchState | undefined = useListNavigation<SearchState>(() => ({ query: query.value, filters: filters.value, searched: searched.value, visibleKinds: visibleKinds.value, ideas: ideas.value, products: products.value, tools: tools.value, other_items: otherItems.value, companies: companies.value, recipes: recipes.value, creators: creators.value, has_more: hasMore.value, limit: limit.value }))
const query = ref(previous?.query || '')
const filters = ref<Record<string, string[]>>(previous?.filters || { kind: [], idea_type: [], direction: [], author: [] })
const ideas = ref<Idea[]>(previous?.ideas || []); const products = ref<Product[]>(previous?.products || []); const tools = ref<Product[]>(previous?.tools || []); const otherItems = ref<Product[]>(previous?.other_items || []); const companies = ref<Company[]>(previous?.companies || []); const recipes = ref<RecipeGroup[]>(previous?.recipes || [])
const creators = ref<SearchResult['creators']>(previous?.creators || [])
const searched = ref(previous?.searched || false)
const visibleKinds = ref<string[]>(previous?.visibleKinds || [])
const hasMore = ref<Record<string, boolean>>(previous?.has_more || {})
const limit = ref(previous?.limit || 50)
const kindOptions = [{ value: 'ideas', label: '灵感' }, { value: 'products', label: '商品' }, { value: 'tools', label: '道具' }, { value: 'other-items', label: '其他物品' }, { value: 'companies', label: '公司' }, { value: 'recipes', label: '合成配方' }]
const includesIdeas = computed(() => !filters.value.kind.length || filters.value.kind.includes('ideas'))
const groups = computed(() => [
  { key: 'kind', label: '记录类型', allLabel: '全部内容', options: kindOptions },
  ...(includesIdeas.value ? [
    { key: 'idea_type', label: '内容类型', allLabel: '全部内容类型', options: ['名称', '商品概念', '玩法', '包装', '世界观', '其他'] },
    { key: 'direction', label: '商品方向', allLabel: '全部商品方向', options: ['饮料', '乳制品', '香肠', '罐头', '巧克力', '零食', '日用品', '其他'] },
  ] : []),
  { key: 'author', label: '创建者', allLabel: '全部创建者', options: creators.value.map(user => ({ value: user.id, label: user.nickname })) },
])
function parameters() {
  const parameters = new URLSearchParams()
  if (query.value.trim()) parameters.set('q', query.value.trim())
  for (const [key, values] of Object.entries(filters.value)) for (const value of values) parameters.append(key, value)
  return parameters
}
const { loading, error, run: search, schedule, refresh } = useLiveSearch(async signal => {
  const params = parameters()
  const urlQuery: Record<string, string | string[]> = {}
  for (const key of new Set(params.keys())) { const values = params.getAll(key); urlQuery[key] = values.length === 1 ? values[0] : values }
  if (route.path === '/search') void router.replace({ path: '/search', query: urlQuery })
  const kinds = filters.value.kind.length ? [...filters.value.kind] : kindOptions.map(option => option.value)
  const ideaOnly = !!(filters.value.idea_type.length || filters.value.direction.length)
  return { result: await api<SearchResult>(`/search?${params}`, { signal }), searched: params.size > 0, kinds: ideaOnly ? kinds.filter(kind => kind === 'ideas') : kinds }
}, ({ result, searched: didSearch, kinds }) => {
  ideas.value = result.ideas; products.value = result.products; tools.value = result.tools; otherItems.value = result.other_items; companies.value = result.companies; recipes.value = result.recipes; creators.value = result.creators
  searched.value = didSearch; visibleKinds.value = kinds; hasMore.value = result.has_more; limit.value = result.limit
})
let restoring = false
function restoreRoute() {
  if (route.path !== '/search') return
  const nextQuery = typeof route.query.q === 'string' ? route.query.q : ''
  const nextFilters = Object.fromEntries(Object.keys(filters.value).map(key => {
    const raw = route.query[key]
    const values = (Array.isArray(raw) ? raw : [raw]).filter((value): value is string => typeof value === 'string' && !!value)
    return [key, key === 'kind' ? values.filter(value => kindOptions.some(option => option.value === value)) : values]
  }))
  if (nextQuery === query.value.trim() && JSON.stringify(nextFilters) === JSON.stringify(filters.value)) return
  restoring = true; query.value = nextQuery; filters.value = nextFilters; restoring = false
  void search()
}
function changeFilters(value: Record<string, string[]>) {
  if (value.kind.length && !value.kind.includes('ideas')) { value.idea_type = []; value.direction = [] }
  filters.value = value
  void search()
}
watch(query, () => { if (!restoring) schedule() }, { flush: 'sync' })
watch(() => route.query, restoreRoute)
onMounted(() => { restoreRoute(); if (!loading.value) void search() })
useEvents(refresh)
</script>
<template>
  <section>
    <div class="page-heading"><div><h1>全局搜索</h1><p>搜索灵感、商品、道具、其他物品、公司和合成配方，也可通过材料查找产物。</p></div></div>
    <form class="toolbar" @submit.prevent="search">
      <input v-model="query" type="search" maxlength="200" :autofocus="!previous" placeholder="输入中文或英文关键词" aria-label="全局搜索关键词" />
      <FilterPopover id="search-filter-popover" :model-value="filters" label="搜索结果" :groups="groups" @update:model-value="changeFilters" />
    </form>
    <p v-if="filters.idea_type.length || filters.direction.length" class="muted">内容类型和商品方向用于筛选灵感。</p>
    <p v-if="Object.values(hasMore).some(Boolean)" class="muted" role="status">每类最多展示 {{ limit }} 条结果，请继续输入关键词或添加筛选条件缩小范围。</p>
    <p v-if="loading && !searched" class="muted">读取中…</p>
    <p v-if="error" class="error-text">{{ error }}</p>
    <template v-if="searched">
      <section v-if="visibleKinds.includes('ideas')" class="search-section"><h2>灵感（{{ ideas.length }}）</h2><div v-if="ideas.length" class="idea-list"><IdeaCard v-for="idea in ideas" :key="idea.id" :idea="idea" @changed="search" /></div><p v-else class="muted">没有匹配的灵感。</p></section>
      <section v-for="collection in [{kind:'products',label:'商品',items:products},{kind:'tools',label:'道具',items:tools},{kind:'other-items',label:'其他物品',items:otherItems}].filter(c => visibleKinds.includes(c.kind))" :key="collection.kind" class="search-section"><h2>{{ collection.label }}（{{ collection.items.length }}）</h2><div v-if="collection.items.length" class="product-grid"><ItemCard v-for="item in collection.items" :key="item.id" :item="item" /></div><p v-else class="muted">没有匹配的{{ collection.label }}。</p></section>
      <section v-if="visibleKinds.includes('companies')" class="search-section"><h2>公司（{{ companies.length }}）</h2><div v-if="companies.length" class="product-grid"><article v-for="company in companies" :key="company.id" class="product-card"><div class="product-card-content"><h2><RouterLink :to="`/companies/${company.id}`">{{ companyTitle(company) }}</RouterLink></h2><p class="product-english-name">{{ company.english_name }}</p><p class="card-summary">{{ company.description }}</p><span class="muted">{{ company.item_count }} 件关联物品</span></div></article></div><p v-else class="muted">没有匹配的公司。</p></section>
      <section v-if="visibleKinds.includes('recipes')" class="search-section"><h2>合成产物（{{ recipes.length }}）</h2><div v-if="recipes.length" class="product-grid"><article v-for="group in recipes" :key="group.id" class="product-card"><div class="product-card-content"><h2><RouterLink :to="`/recipes/${group.id}`">{{ productTitle(group.output) }}</RouterLink></h2><span class="muted">{{ group.recipe_count ?? group.recipes.length }} 套配方</span></div></article></div><p v-else class="muted">没有匹配的配方。</p></section>
    </template>
    <div v-if="!searched && !loading" class="empty-state">输入关键词或选择筛选条件，查找游戏设定。</div>
  </section>
</template>
