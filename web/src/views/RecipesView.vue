<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'
import type { Ingredient, RecipeGroup } from '../types'
import { productTitle } from '../utils'
import { useEvents } from '../events'
import { useLiveSearch } from '../composables/useLiveSearch'
import ItemPicker from '../components/ItemPicker.vue'
const router = useRouter(); const items = ref<RecipeGroup[]>([]); const query = ref(''); const picking = ref(false); const busy = ref(false);
const { loading, error, run: load, schedule, refresh } = useLiveSearch(signal => api<{ items: RecipeGroup[] }>(`/recipe-groups?q=${encodeURIComponent(query.value)}`, { signal }), result => { items.value = result.items })
async function selected(ingredient: Ingredient) { if (busy.value) return; busy.value = true; error.value = ''; try { const group = await api<RecipeGroup>('/recipe-groups',{ method: 'POST', body: JSON.stringify({ output_id: ingredient.product_id }) }); picking.value = false; await router.push(`/recipes/${group.id}`) } catch (e) { error.value = (e as Error).message; picking.value = false } finally { busy.value = false } }
onMounted(load); watch(query, schedule, { flush: 'sync' }); useEvents(refresh)
</script>
<template><section><div class="page-heading"><div><h1>合成配方</h1><p>按产物归组，在每个产物中管理多套合成方案。</p></div><button class="primary" :disabled="busy" @click="picking = true">添加合成产物</button></div><form class="toolbar" @submit.prevent="load"><input v-model="query" type="search" aria-label="搜索配方" placeholder="搜索产物、材料、分类或配方备注" /></form><p v-if="error" class="error-text">{{ error }} <button class="button-link" @click="load">重试</button></p><p v-if="loading && !items.length" class="muted">读取中…</p><div v-else-if="items.length" class="product-grid"><article v-for="group in items" :key="group.id" class="product-card"><img v-if="group.output.attachments[0]" class="product-cover" :src="group.output.attachments[0].thumbnail_url || group.output.attachments[0].url" alt="" loading="lazy" /><div class="product-card-content"><h2><RouterLink :to="`/recipes/${group.id}`">{{ productTitle(group.output) }}</RouterLink></h2><p v-if="group.output.chinese_name && group.output.english_name" class="product-english-name">{{ group.output.english_name }}</p><div class="product-meta"><span class="tag">{{ group.output.item_kind === 'tool' ? '道具' : '商品' }}</span><span>{{ group.recipes.length }} 套配方</span><span v-if="!group.recipes.length || group.recipes.some(r => !r.ingredients.length)" class="muted">待补充</span><span v-if="group.output.deleted_at" class="danger-text">产物已删除</span></div></div></article></div><div v-else class="empty-state">{{ query ? '没有匹配的产物或材料。' : '选择一个商品或道具作为产物，开始记录配方。' }}</div></section><ItemPicker v-if="picking" title="选择合成产物" @select="selected" @cancel="picking = false" /></template>
