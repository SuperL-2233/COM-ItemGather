<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { api } from '../api'
import { useEvents } from '../events'
import { useLiveSearch } from '../composables/useLiveSearch'
import type { ChoiceOption, Ingredient, ItemKind, Page, Product } from '../types'
import { itemCollection, itemLabel, productTitle } from '../utils'
import ItemCreateDialog from './ItemCreateDialog.vue'
const props = defineProps<{ title?: string; allowCategory?: boolean }>()
const emit = defineEmits<{ select: [ingredient: Ingredient]; cancel: [] }>()
const kinds = ['product', 'tool', 'other'] as const
const query = ref('')
const items = ref<Record<ItemKind, Product[]>>({ product: [], tool: [], other: [] })
const totals = ref<Record<ItemKind, number>>({ product: 0, tool: 0, other: 0 })
const expanded = ref<Record<ItemKind, boolean>>({ product: true, tool: false, other: false })
const creating = ref<ItemKind>()
const mode = ref('item'); const categories = ref<string[]>([]); const category = ref(''); const examples = ref<Product[]>([]); const categoryTotal = ref(0)
let moreKind: ItemKind | undefined
const { loading: busy, error, run, schedule } = useLiveSearch(async signal => {
  const parameters = new URLSearchParams({ q: query.value, limit: '30' })
  const selectedKinds = moreKind ? [moreKind] : kinds
  const append = !!moreKind
  return Promise.all(selectedKinds.map(async kind => {
    parameters.set('offset', String(append ? items.value[kind].length : 0))
    const page = await api<Page<Product>>(`/${itemCollection(kind)}?${parameters}`, { signal })
    return { kind, page, append }
  }))
}, results => {
  for (const { kind, page, append } of results) {
    items.value[kind] = append ? [...items.value[kind], ...page.items.filter(item => !items.value[kind].some(existing => existing.id === item.id))] : page.items
    totals.value[kind] = page.total
    if (!append) expanded.value[kind] = query.value.trim() ? !!page.total : kind === 'product'
  }
})
function load() { moreKind = undefined; return run() }
function more(kind: ItemKind) { if (!busy.value) { moreKind = kind; void run() } }
function choose(item: Product) { emit('select',{ product_id: item.id, category: '', quantity: 1, item }) }
async function previewCategory() { if (!category.value) { examples.value = []; categoryTotal.value = 0; return }; const selected = category.value; try { const result = await api<Page<Product>>(`/products?${new URLSearchParams({ category: selected, limit: '10' })}`); if (category.value === selected) { examples.value = result.items; categoryTotal.value = result.total } } catch (e) { error.value = (e as Error).message } }
async function loadCategories() { if (!props.allowCategory) return; try { categories.value = (await api<{ items: ChoiceOption[] }>('/product-options/categories')).items.map(option => option.value); if (category.value && !categories.value.includes(category.value)) { category.value = ''; error.value = '该分类已被删除，请重新选择' } } catch (e) { error.value = (e as Error).message } }
onMounted(() => { void load(); void loadCategories() }); useEvents(() => void loadCategories()); watch(query, () => { moreKind = undefined; schedule() }, { flush: 'sync' }); watch(category,previewCategory)

</script>
<template><Teleport to="body"><div class="modal-backdrop" @keydown.esc.stop="emit('cancel')"><section class="modal picker-modal" role="dialog" aria-modal="true" aria-labelledby="item-picker-title"><div class="page-heading"><h2 id="item-picker-title">{{ title || '选择物品' }}</h2><button type="button" class="text-button" aria-label="关闭物品选择" @click="emit('cancel')">关闭</button></div><div v-if="allowCategory" class="filter-tabs"><button type="button" :class="{ active: mode === 'item' }" @click="mode = 'item'">指定物品</button><button type="button" :class="{ active: mode === 'category' }" @click="mode = 'category'">某类商品</button></div><template v-if="mode === 'item'"><input v-model="query" type="search" aria-label="搜索物品" placeholder="搜索商品、道具或其他物品的中英文名及设定" autofocus @keydown.enter.stop.prevent="load()" /><p v-if="busy && !kinds.some(kind => items[kind].length)" class="muted">读取中…</p><div class="picker-groups"><section v-for="kind in kinds" :key="kind" class="picker-group"><button type="button" class="picker-group-toggle" :aria-expanded="expanded[kind]" @click="expanded[kind] = !expanded[kind]"><span>{{ expanded[kind] ? '▾' : '▸' }} {{ itemLabel(kind) }}</span><span class="muted">{{ totals[kind] }}</span></button><div v-if="expanded[kind]" class="choice-list"><button v-for="item in items[kind]" :key="item.id" type="button" class="choice-row" @click="choose(item)"><img v-if="item.attachments[0]" :src="item.attachments[0].thumbnail_url || item.attachments[0].url" alt="" class="item-choice-image" /><span><strong>{{ productTitle(item) }}</strong><small>{{ [item.english_name,item.company].filter(Boolean).join(' · ') }}</small></span><span class="tag">{{ itemLabel(kind) }}</span></button><p v-if="!items[kind].length" class="muted">{{ query ? '没有匹配内容。' : '暂无内容。' }}</p><button v-if="items[kind].length < totals[kind]" type="button" class="button-link" :disabled="busy" @click="more(kind)">加载更多</button></div></section></div><div class="inline-controls"><button type="button" class="secondary small" @click="creating = 'product'">＋ 创建商品</button><button type="button" class="secondary small" @click="creating = 'tool'">＋ 创建道具</button><button type="button" class="secondary small" @click="creating = 'other'">＋ 创建其他物品</button></div></template><template v-else><label>材料分类<select v-model="category" aria-label="材料分类"><option value="">选择分类</option><option v-for="name in categories" :key="name">{{ name }}</option></select></label><p class="muted">选择后可使用该分类中的任意一种商品。分类可在商品详情中填写。</p><div v-if="category" class="category-preview"><p>任意{{ category }} · 当前 {{ categoryTotal }} 件商品</p><span v-for="item in examples" :key="item.id" class="tag">{{ productTitle(item) }}</span></div><button type="button" class="primary" :disabled="!category" @click="emit('select',{ product_id: '', category, quantity: 1 })">使用此分类</button></template><p v-if="error" class="error-text" role="alert">{{ error }} <button class="button-link" @click="load">重试</button></p></section><ItemCreateDialog v-if="creating" :kind="creating" :initial-name="query" @created="choose" @cancel="creating = undefined" /></div></Teleport></template>
