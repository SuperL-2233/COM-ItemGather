<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'
import type { RecipeGroup } from '../types'
import { productTitle } from '../utils'
import { useEvents } from '../events'
const router = useRouter()
const props = defineProps<{ itemId: string; itemCategory?: string; itemKind?: string }>()
const groups = ref<RecipeGroup[]>([]); const error = ref(''); const busy = ref(false)
const outputs = computed(() => groups.value.filter(g => g.output_id === props.itemId)); const usages = computed(() => groups.value.filter(g => g.recipes.some(r => r.ingredients.some(i => i.product_id === props.itemId || (!!i.category && props.itemKind === 'product' && i.category === props.itemCategory)))))
async function load() { try { groups.value = (await api<{ items: RecipeGroup[] }>(`/recipe-groups?item_id=${props.itemId}`)).items } catch (e) { error.value = (e as Error).message } }
async function create() { busy.value = true; error.value = ''; try { const g = await api<RecipeGroup>('/recipe-groups',{ method: 'POST', body: JSON.stringify({ output_id: props.itemId }) }); await router.push(`/recipes/${g.id}`) } catch (e) { error.value = (e as Error).message } finally { busy.value = false } }
onMounted(load); useEvents(load)
</script>
<template><section class="panel"><h2>合成关系</h2><h3>合成获得</h3><p v-if="!outputs.length" class="muted">还没有记录合成方式。</p><p v-for="g in outputs" :key="g.id"><RouterLink :to="`/recipes/${g.id}`">查看 {{ g.recipes.length }} 套配方 →</RouterLink></p><button v-if="!outputs.length" type="button" class="secondary small" :disabled="busy" @click="create">为此物品添加配方</button><h3 class="related-heading">作为材料</h3><p v-if="!usages.length" class="muted">尚未用于其他配方。</p><p v-for="g in usages" :key="g.id"><RouterLink :to="`/recipes/${g.id}`">{{ productTitle(g.output) }} →</RouterLink></p><p v-if="error" class="error-text">{{ error }}</p></section></template>
