<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { onBeforeRouteLeave, useRoute, useRouter } from 'vue-router'
import { api } from '../api'
import { useListBackLocation } from '../composables/listNavigation'
import type { RecipeGroup } from '../types'
import { itemLabel, itemPath, productTitle } from '../utils'
import { useEvents } from '../events'
import RecipeEditor from '../components/RecipeEditor.vue'
const backTo = useListBackLocation('/recipes')
const route = useRoute(); const router = useRouter(); const id = route.params.id as string; const group = ref<RecipeGroup>(); const error = ref(''); const busy = ref(false); const dirtyCards = ref<Record<string, boolean>>({}); const dirty = computed(() => Object.values(dirtyCards.value).some(Boolean))
async function load() { try { group.value = await api<RecipeGroup>(`/recipe-groups/${id}`) } catch (e) { error.value = (e as Error).message } }
async function add(fields?: Record<string, unknown>) { busy.value = true; error.value = ''; try { await api(`/recipe-groups/${id}/recipes`,{ method: 'POST', body: JSON.stringify(fields ? { ...fields, name: fields.name ? `${fields.name}（副本）` : '配方副本' } : { output_quantity: 1, verification: 'untested', ingredients: [] }) }); await load() } catch (e) { error.value = (e as Error).message } finally { busy.value = false } }
async function remove() { if (!group.value || !confirm('将这个产物的配方页面移入回收站？其中的配方会一并保留，产物本身不受影响。')) return; try { await api(`/recipe-groups/${id}`,{ method: 'DELETE', body: JSON.stringify({ version: group.value.version }) }); dirtyCards.value = {}; await router.push(backTo.value) } catch (e) { error.value = (e as Error).message } }
function beforeUnload(event: BeforeUnloadEvent) { if (dirty.value) { event.preventDefault(); event.returnValue = '' } }
onBeforeRouteLeave(() => !dirty.value || confirm('还有未保存的配方修改，确定离开？')); onMounted(() => { void load(); window.addEventListener('beforeunload',beforeUnload) }); onBeforeUnmount(() => window.removeEventListener('beforeunload',beforeUnload)); useEvents(() => void load())
</script>
<template><div><div v-if="group" class="page-heading"><div><RouterLink :to="backTo">← 返回合成配方</RouterLink><h1>{{ productTitle(group.output) }}</h1><p>{{ itemLabel(group.output.item_kind) }} · {{ group.recipes.length }} 套配方 <span v-if="group.output.deleted_at" class="danger-text">· 产物已删除，可在回收站恢复</span></p></div><button class="text-button danger-text" @click="remove">移入回收站</button></div><p v-if="error" class="error-text">{{ error }}</p><template v-if="group"><section class="panel output-summary"><img v-if="group.output.attachments[0]" :src="group.output.attachments[0].thumbnail_url || group.output.attachments[0].url" alt="" /><div><p>{{ group.output.description || group.output.tool_settings?.effect || '产物设定待补充' }}</p><RouterLink v-if="!group.output.deleted_at" :to="itemPath(group.output)">查看完整{{ itemLabel(group.output.item_kind) }}设定 →</RouterLink></div></section><div class="page-heading"><h2>合成方案</h2><button class="primary" :disabled="busy" @click="add()">＋ 添加配方</button></div><RecipeEditor v-for="(recipe,index) in group.recipes" :key="recipe.id" :recipe="recipe" :index="index" @changed="load" @copy="add" @dirty="dirtyCards[recipe.id] = $event" /><div v-if="!group.recipes.length" class="empty-state">还没有配方，点击“添加配方”记录第一套方案。</div></template><p v-else-if="!error" class="muted">读取中…</p></div></template>
