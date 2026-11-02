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
import { itemPath } from '../utils'
const router = useRouter(); const route = useRoute()
const items = ref<Product[]>([]); const query = ref(''); const categories = ref<string[]>([]); const statuses = ref<ChoiceOption[]>([]); const creating = ref(false); const total = ref(0)
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
<template><section><div class="page-heading"><div><h1>道具库</h1><p>记录道具的获得途径、触发方式与使用效果。</p></div><button class="primary" @click="creating = true">创建道具</button></div><p v-if="route.query.company_id" class="muted">正在查看所选公司旗下道具 · <RouterLink to="/tools">查看全部公司</RouterLink></p><form class="toolbar" @submit.prevent="load()"><input v-model="query" type="search" aria-label="搜索道具" placeholder="搜索名称、效果、公司或获取来源" /><FilterPopover id="tool-filter-popover" v-model="filters" label="道具" :groups="groups" /></form><p v-if="error" class="error-text">{{ error }} <button class="button-link" @click="load()">重试</button></p><p v-if="busy && !items.length" class="muted">读取中…</p><div v-else-if="items.length" class="product-grid"><ItemCard v-for="item in items" :key="item.id" :item="item" /></div><div v-else class="empty-state">{{ query || Object.values(filters).some(values => values.length) ? '没有符合条件的道具。' : '还没有道具，先记录一个名称或效果吧。' }}</div><button v-if="items.length < total" class="secondary load-more" :disabled="busy" @click="more">加载更多</button></section><ItemCreateDialog v-if="creating" kind="tool" @created="created" @cancel="creating = false" /></template>
