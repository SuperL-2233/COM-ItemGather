<script setup lang="ts">
import { computed, onMounted, ref, useId } from 'vue'
import { api, ApiError } from '../api'
import { categoryIdentity } from '../utils'
import type { CategoryDeletion, CategoryUsage, ChoiceOption } from '../types'
import SearchCreateSelect from './SearchCreateSelect.vue'
const props = defineProps<{ name: string; options: ChoiceOption[]; create: (name: string) => Promise<ChoiceOption> }>()
const emit = defineEmits<{ deleted: [result: CategoryDeletion]; cancel: [] }>()
const id = useId()
const usage = ref<CategoryUsage>()
const cancelButton = ref<HTMLButtonElement>()
const loading = ref(true)
const busy = ref(false)
const error = ref('')
const mode = ref('clear')
const replacement = ref('')
const choices = computed(() => props.options.filter(option => categoryIdentity(option.value) !== categoryIdentity(props.name)))
const needsReplacement = computed(() => (usage.value?.recipe_count || 0) > 0)
const hasReferences = computed(() => (usage.value?.item_count || 0) + (usage.value?.recipe_count || 0) > 0)
const canDelete = computed(() => !!usage.value && !loading.value && !busy.value && ((mode.value === 'clear' && !needsReplacement.value) || (mode.value === 'replace' && choices.value.some(option => option.value === replacement.value))))
async function load() {
  loading.value = true
  error.value = ''
  try {
    usage.value = await api<CategoryUsage>(`/product-options/categories/usage?${new URLSearchParams({ name: props.name })}`)
    if (needsReplacement.value) mode.value = 'replace'
  } catch (e) { usage.value = undefined; error.value = (e as Error).message }
  finally { loading.value = false }
}
async function remove() {
  if (!canDelete.value || !usage.value) return
  busy.value = true
  error.value = ''
  try {
    const result = await api<CategoryDeletion>('/product-options/categories',{ method: 'DELETE', body: JSON.stringify({ name: props.name, token: usage.value.token, replacement: mode.value === 'replace' ? replacement.value : '' }) })
    emit('deleted',result)
  } catch (e) {
    if (e instanceof ApiError && e.code === 'category_changed' && e.details?.usage) {
      usage.value = e.details.usage
      if (needsReplacement.value) mode.value = 'replace'
    }
    if (e instanceof ApiError && e.status === 404) usage.value = undefined
    error.value = (e as Error).message
  } finally { busy.value = false }
}
onMounted(() => { void load(); cancelButton.value?.focus() })
</script>
<template><Teleport to="body"><div class="modal-backdrop" @keydown.esc.stop.prevent="!busy && emit('cancel')"><form class="modal category-delete-modal" role="dialog" aria-modal="true" :aria-labelledby="`${id}-title`" @submit.stop.prevent="remove"><h2 :id="`${id}-title`">删除分类“{{ name }}”</h2><p v-if="loading" class="muted">正在检查分类使用情况…</p><template v-else-if="usage"><p v-if="hasReferences">此分类用于 {{ usage.item_count }} 件商品／道具、{{ usage.recipe_count }} 套配方（包含回收站中的内容）。</p><p v-else>此分类当前未被使用，确认后会从分类选项中移除。</p><p class="muted">商品、道具和配方本身都会保留。</p><fieldset v-if="hasReferences" :disabled="busy" class="category-delete-actions"><legend>关联内容如何处理</legend><label class="category-delete-choice"><input v-model="mode" type="radio" value="clear" :name="`${id}-mode`" :disabled="needsReplacement" />清空关联物品的分类</label><p v-if="needsReplacement" class="muted">配方使用该分类作为材料，需要选择替代分类。</p><label class="category-delete-choice"><input v-model="mode" type="radio" value="replace" :name="`${id}-mode`" />替换为其他分类</label><SearchCreateSelect v-if="mode === 'replace'" v-model="replacement" label="替代分类" :options="choices" :create="create" create-label="＋ 新建分类" new-label="新分类名称" empty-label="请选择替代分类" :max-length="100" /><p v-if="mode === 'replace' && replacement && !choices.some(option => option.value === replacement)" class="error-text">替代分类已被删除或不可用，请重新选择。</p><p v-if="mode === 'replace'" class="muted category-delete-note">配方中已有相同分类材料时，数量会合并。</p></fieldset></template><p v-if="error" class="error-text" role="alert">{{ error }} <button v-if="!usage" type="button" class="button-link" @click="load">重试</button></p><div class="modal-actions"><button ref="cancelButton" type="button" class="secondary" :disabled="busy" autofocus @click="emit('cancel')">取消</button><button class="danger-button" :disabled="!canDelete">{{ busy ? '删除中…' : '确认删除分类' }}</button></div></form></div></Teleport></template>
