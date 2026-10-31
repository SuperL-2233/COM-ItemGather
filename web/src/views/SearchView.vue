<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api'
import type { Idea, Product } from '../types'
import IdeaCard from '../components/IdeaCard.vue'
import { statusLabels } from '../utils'

interface SearchResult { ideas: Idea[]; products: Product[]; creators: { id: string; nickname: string }[] }
const route = useRoute()
const router = useRouter()
const query = ref('')
const kind = ref('all')
const ideaType = ref('')
const direction = ref('')
const author = ref('')
const ideas = ref<Idea[]>([])
const products = ref<Product[]>([])
const creators = ref<{ id: string; nickname: string }[]>([])
const searched = ref(false)
const loading = ref(false)
const error = ref('')
const ideaTypes = ['名称', '商品概念', '玩法', '包装', '世界观', '其他']
const directions = ['饮料', '乳制品', '香肠', '罐头', '巧克力', '零食', '日用品', '其他']

function queryValue(value: unknown) { return typeof value === 'string' ? value : '' }
async function search() {
  loading.value = true
  error.value = ''
  const parameters = new URLSearchParams()
  if (query.value.trim()) parameters.set('q', query.value.trim())
  if (kind.value !== 'all') parameters.set('kind', kind.value)
  if (ideaType.value) parameters.set('idea_type', ideaType.value)
  if (direction.value) parameters.set('direction', direction.value)
  if (author.value) parameters.set('author', author.value)
  router.replace({ query: Object.fromEntries(parameters) })
  try {
    const result = await api<SearchResult>(`/search?${parameters}`)
    ideas.value = result.ideas
    products.value = result.products
    creators.value = result.creators
    searched.value = parameters.size > 0
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    loading.value = false
  }
}
function changeKind() {
  if (kind.value === 'products') { ideaType.value = ''; direction.value = ''; author.value = '' }
  search()
}
onMounted(() => {
  query.value = queryValue(route.query.q)
  kind.value = ['ideas', 'products'].includes(queryValue(route.query.kind)) ? queryValue(route.query.kind) : 'all'
  ideaType.value = queryValue(route.query.idea_type)
  direction.value = queryValue(route.query.direction)
  author.value = queryValue(route.query.author)
  search()
})
</script>

<template>
  <section>
    <div class="page-heading"><div><h1>全局搜索</h1><p>搜索原始文字、标题、名称、摘要、标签、分类、动态属性和商品描述。</p></div></div>
    <form class="toolbar" @submit.prevent="search">
      <input v-model="query" type="search" maxlength="200" autofocus placeholder="输入中文或英文关键词" aria-label="全局搜索关键词" />
      <button class="primary" :disabled="loading">{{ loading ? '搜索中…' : '搜索' }}</button>
    </form>
    <form class="toolbar" aria-label="搜索筛选" @submit.prevent="search">
      <select v-model="kind" aria-label="记录类型" @change="changeKind"><option value="all">灵感与商品</option><option value="ideas">只看灵感</option><option value="products">只看商品</option></select>
      <template v-if="kind !== 'products'">
        <select v-model="ideaType" aria-label="内容类型" @change="search"><option value="">全部内容类型</option><option v-for="item in ideaTypes" :key="item" :value="item">{{ item }}</option></select>
        <select v-model="direction" aria-label="商品方向" @change="search"><option value="">全部商品方向</option><option v-for="item in directions" :key="item" :value="item">{{ item }}</option></select>
        <select v-model="author" aria-label="创建者" @change="search"><option value="">全部创建者</option><option v-for="item in creators" :key="item.id" :value="item.id">{{ item.nickname }}</option></select>
      </template>
    </form>
    <p v-if="error" class="error-text">{{ error }}</p>
    <template v-if="searched && !loading">
      <section v-if="kind !== 'products'" class="search-section"><h2>灵感（{{ ideas.length }}）</h2><div v-if="ideas.length" class="idea-list"><IdeaCard v-for="idea in ideas" :key="idea.id" :idea="idea" @changed="search" /></div><p v-else class="muted">没有匹配的灵感。</p></section>
      <section v-if="kind !== 'ideas'" class="search-section"><h2>商品（{{ products.length }}）</h2><div v-if="products.length" class="product-grid"><article v-for="product in products" :key="product.id" class="product-card"><div class="product-card-content"><h2><RouterLink :to="`/products/${product.id}`">{{ product.internal_name }}</RouterLink></h2><p v-if="product.final_brand_name">{{ product.final_brand_name }}</p><p v-if="product.description" class="card-summary">{{ product.description }}</p><div class="product-meta"><span v-if="product.category">{{ product.category }}</span><span class="status-badge">{{ statusLabels[product.development_status] }}</span><span>{{ product.idea_count }} 条关联灵感</span></div></div></article></div><p v-else class="muted">没有匹配的商品。</p></section>
    </template>
    <div v-if="!searched" class="empty-state">输入关键词或选择筛选条件，查找灵感与商品。</div>
  </section>
</template>
