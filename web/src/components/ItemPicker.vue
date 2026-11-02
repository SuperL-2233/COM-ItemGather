<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { api } from '../api'
import { useEvents } from '../events'
import { useLiveSearch } from '../composables/useLiveSearch'
import type { ChoiceOption, Ingredient, Page, Product } from '../types'
import { productTitle } from '../utils'
import ItemCreateDialog from './ItemCreateDialog.vue'
const props = defineProps<{ title?: string; allowCategory?: boolean }>()
const emit = defineEmits<{ select: [ingredient: Ingredient]; cancel: [] }>()
const query = ref(''); const goods = ref<Product[]>([]); const tools = ref<Product[]>([]); const totals = ref({ product: 0, tool: 0 }); const expanded = ref({ product: true, tool: false }); const creating = ref<'product' | 'tool'>(); const mode = ref('item'); const categories = ref<string[]>([]); const category = ref(''); const examples = ref<Product[]>([]); const categoryTotal = ref(0)
let moreKind: 'product' | 'tool' | undefined
const { loading: busy, error, run, schedule } = useLiveSearch(async signal => {
  const parameters = new URLSearchParams({ q: query.value, limit: '30' })
  if (moreKind) {
    parameters.set('offset', String((moreKind === 'product' ? goods : tools).value.length))
    const page = await api<Page<Product>>(`/${moreKind === 'product' ? 'products' : 'tools'}?${parameters}`, { signal })
    return { kind: moreKind, page }
  }
  const [p, t] = await Promise.all([api<Page<Product>>(`/products?${parameters}`, { signal }), api<Page<Product>>(`/tools?${parameters}`, { signal })])
  return { p, t }
}, result => {
  if (result.kind && result.page) {
    const current = result.kind === 'product' ? goods : tools
    current.value.push(...result.page.items.filter(item => !current.value.some(existing => existing.id === item.id)))
  } else if (result.p && result.t) {
    goods.value = result.p.items; tools.value = result.t.items
    totals.value = { product: result.p.total, tool: result.t.total }
    expanded.value = query.value.trim() ? { product: !!result.p.total, tool: !!result.t.total } : { product: true, tool: false }
  }
})
function load() { moreKind = undefined; return run() }
function more(kind: 'product' | 'tool') { if (!busy.value) { moreKind = kind; void run() } }
function choose(item: Product) { emit('select',{ product_id: item.id, category: '', quantity: 1, item }) }
async function previewCategory() { if (!category.value) { examples.value = []; categoryTotal.value = 0; return }; const selected = category.value; try { const result = await api<Page<Product>>(`/products?${new URLSearchParams({ category: selected, limit: '10' })}`); if (category.value === selected) { examples.value = result.items; categoryTotal.value = result.total } } catch (e) { error.value = (e as Error).message } }
async function loadCategories() { if (!props.allowCategory) return; try { categories.value = (await api<{ items: ChoiceOption[] }>('/product-options/categories')).items.map(option => option.value); if (category.value && !categories.value.includes(category.value)) { category.value = ''; error.value = '该分类已被删除，请重新选择' } } catch (e) { error.value = (e as Error).message } }
onMounted(() => { void load(); void loadCategories() }); useEvents(() => void loadCategories()); watch(query, () => { moreKind = undefined; schedule() }, { flush: 'sync' }); watch(category,previewCategory)

</script>
<template><Teleport to="body"><div class="modal-backdrop" @keydown.esc.stop="emit('cancel')"><section class="modal picker-modal" role="dialog" aria-modal="true" aria-labelledby="item-picker-title"><div class="page-heading"><h2 id="item-picker-title">{{ title || '选择物品' }}</h2><button type="button" class="text-button" aria-label="关闭物品选择" @click="emit('cancel')">关闭</button></div><div v-if="allowCategory" class="filter-tabs"><button type="button" :class="{ active: mode === 'item' }" @click="mode = 'item'">指定物品</button><button type="button" :class="{ active: mode === 'category' }" @click="mode = 'category'">某类商品</button></div><template v-if="mode === 'item'"><input v-model="query" type="search" aria-label="搜索商品和道具" placeholder="搜索商品、道具的中英文名或设定" autofocus @keydown.enter.stop.prevent="load()" /><p v-if="busy && !goods.length && !tools.length" class="muted">读取中…</p><div class="picker-groups"><section v-for="kind in (['product','tool'] as const)" :key="kind" class="picker-group"><button type="button" class="picker-group-toggle" :aria-expanded="expanded[kind]" @click="expanded[kind] = !expanded[kind]"><span>{{ expanded[kind] ? '▾' : '▸' }} {{ kind === 'product' ? '商品' : '道具' }}</span><span class="muted">{{ totals[kind] }}</span></button><div v-if="expanded[kind]" class="choice-list"><button v-for="item in kind === 'product' ? goods : tools" :key="item.id" type="button" class="choice-row" @click="choose(item)"><img v-if="item.attachments[0]" :src="item.attachments[0].thumbnail_url || item.attachments[0].url" alt="" class="item-choice-image" /><span><strong>{{ productTitle(item) }}</strong><small>{{ [item.english_name,item.company].filter(Boolean).join(' · ') }}</small></span><span class="tag">{{ kind === 'product' ? '商品' : '道具' }}</span></button><p v-if="!(kind === 'product' ? goods : tools).length" class="muted">{{ query ? '没有匹配内容。' : '暂无内容。' }}</p><button v-if="(kind === 'product' ? goods : tools).length < totals[kind]" type="button" class="button-link" :disabled="busy" @click="more(kind)">加载更多</button></div></section></div><div class="inline-controls"><button type="button" class="secondary small" @click="creating = 'product'">＋ 创建商品</button><button type="button" class="secondary small" @click="creating = 'tool'">＋ 创建道具</button></div></template><template v-else><label>材料分类<select v-model="category" aria-label="材料分类"><option value="">选择分类</option><option v-for="name in categories" :key="name">{{ name }}</option></select></label><p class="muted">选择后可使用该分类中的任意一种商品。分类可在商品详情中填写。</p><div v-if="category" class="category-preview"><p>任意{{ category }} · 当前 {{ categoryTotal }} 件商品</p><span v-for="item in examples" :key="item.id" class="tag">{{ productTitle(item) }}</span></div><button type="button" class="primary" :disabled="!category" @click="emit('select',{ product_id: '', category, quantity: 1 })">使用此分类</button></template><p v-if="error" class="error-text" role="alert">{{ error }} <button class="button-link" @click="load">重试</button></p></section><ItemCreateDialog v-if="creating" :kind="creating" :initial-name="query" @created="choose" @cancel="creating = undefined" /></div></Teleport></template>
