<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue'
import { api } from '../api'
import { useEvents } from '../events'
import { categoryIdentity, statusLabels } from '../utils'
import type { CategoryDeletion, ChoiceOption, StatusDeletion } from '../types'
import SearchCreateSelect from './SearchCreateSelect.vue'
import CategoryDeleteDialog from './CategoryDeleteDialog.vue'
import StatusDeleteDialog from './StatusDeleteDialog.vue'
const value = defineModel<string>({ required: true })
const props = withDefaults(defineProps<{ kind: 'categories' | 'statuses'; label: string; fallback?: string; updateOnDelete?: boolean }>(), { updateOnDelete: true })
const emit = defineEmits<{ categoryDeleted: [result: CategoryDeletion]; statusDeleted: [result: StatusDeletion] }>()
const options = ref<ChoiceOption[]>([])
const loading = ref(true)
let requestID = 0
const error = ref('')
const deleting = ref<ChoiceOption>()
const selector = ref<InstanceType<typeof SearchCreateSelect>>()
const missing = computed(() => !loading.value && !error.value && !!value.value && !options.value.some(option => isStatus.value ? option.value === value.value : categoryIdentity(option.value) === categoryIdentity(value.value)))
const isStatus = computed(() => props.kind === 'statuses')
async function load() {
  const request = ++requestID
  try {
    const result = await api<{ items: ChoiceOption[] }>(`/product-options/${props.kind}`)
    if (request !== requestID) return
    options.value = result.items
    if (isStatus.value && !value.value && options.value.length) value.value = options.value[0].value
    error.value = ''
  } catch (e) { if (request === requestID) error.value = (e as Error).message }
  finally { loading.value = false }
}
async function create(name: string) {
  const result = await api<{ items: ChoiceOption[]; option: ChoiceOption }>(`/product-options/${props.kind}`, { method: 'POST', body: JSON.stringify({ name }) })
  requestID++
  options.value = result.items
  return result.option
}
function cancelDelete() { deleting.value = undefined; void nextTick(() => selector.value?.focus()) }
function deleted(result: CategoryDeletion) {
  requestID++
  options.value = options.value.filter(option => categoryIdentity(option.value) !== categoryIdentity(result.name))
  if (props.updateOnDelete !== false && categoryIdentity(value.value) === categoryIdentity(result.name)) value.value = result.replacement
  deleting.value = undefined
  emit('categoryDeleted',result)
  void nextTick(() => selector.value?.focus())
}
function statusDeleted(result: StatusDeletion) {
  requestID++
  options.value = options.value.filter(option => option.value !== result.id)
  if (props.updateOnDelete && value.value === result.id) value.value = result.replacement
  deleting.value = undefined
  emit('statusDeleted', result)
  void nextTick(() => selector.value?.focus())
}
onMounted(load)
useEvents(load)
</script>
<template>
  <SearchCreateSelect ref="selector" v-model="value" :label="label" :options="options" :create="create" :create-label="isStatus ? '＋ 新建制作状态' : '＋ 新建分类'" :new-label="isStatus ? '新制作状态' : '新分类名称'" :allow-empty="!isStatus" :empty-label="`未设置${label}`" :fallback="fallback || (isStatus ? statusLabels[value] : value)" :max-length="isStatus ? 80 : 100" :loading="loading" :error="error" :allow-remove="true" :remove-noun="isStatus ? '制作状态' : '分类'" @remove="deleting = $event" @retry="load" />
  <small v-if="missing" class="error-text" role="status">{{ isStatus ? '该制作状态已被删除，请重新选择。' : '该分类已被删除，请重新选择。' }}</small>
  <StatusDeleteDialog v-if="deleting && isStatus" :option="deleting" :options="options" :create="create" @deleted="statusDeleted" @cancel="cancelDelete" />
  <CategoryDeleteDialog v-else-if="deleting" :name="deleting.value" :options="options" :create="create" @deleted="deleted" @cancel="cancelDelete" />
</template>
