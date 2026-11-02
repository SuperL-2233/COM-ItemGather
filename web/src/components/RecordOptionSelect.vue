<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue'
import { api } from '../api'
import { useEvents } from '../events'
import { categoryIdentity, statusLabels } from '../utils'
import type { CategoryDeletion, ChoiceOption } from '../types'
import SearchCreateSelect from './SearchCreateSelect.vue'
import CategoryDeleteDialog from './CategoryDeleteDialog.vue'
const value = defineModel<string>({ required: true })
const props = withDefaults(defineProps<{ kind: 'categories' | 'statuses'; label: string; fallback?: string; updateOnDelete?: boolean }>(), { updateOnDelete: true })
const emit = defineEmits<{ categoryDeleted: [result: CategoryDeletion] }>()
const options = ref<ChoiceOption[]>([])
const loading = ref(true)
let requestID = 0
const error = ref('')
const deleting = ref<ChoiceOption>()
const selector = ref<InstanceType<typeof SearchCreateSelect>>()
const missing = computed(() => !isStatus.value && !loading.value && !error.value && !!value.value && !options.value.some(option => categoryIdentity(option.value) === categoryIdentity(value.value)))
const isStatus = computed(() => props.kind === 'statuses')
async function load() {
  const request = ++requestID
  try {
    const result = await api<{ items: ChoiceOption[] }>(`/product-options/${props.kind}`)
    if (request !== requestID) return
    options.value = result.items
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
onMounted(load)
useEvents(load)
</script>
<template><SearchCreateSelect ref="selector" v-model="value" :label="label" :options="options" :create="create" :create-label="isStatus ? '＋ 新建制作状态' : '＋ 新建分类'" :new-label="isStatus ? '新制作状态' : '新分类名称'" :allow-empty="!isStatus" :empty-label="`未设置${label}`" :fallback="fallback || (isStatus ? statusLabels[value] : value)" :max-length="isStatus ? 80 : 100" :loading="loading" :error="error" :allow-remove="!isStatus" @remove="deleting = $event" @retry="load" /><small v-if="missing" class="error-text" role="status">该分类已被删除，请重新选择。</small><CategoryDeleteDialog v-if="deleting" :name="deleting.value" :options="options" :create="create" @deleted="deleted" @cancel="cancelDelete" /></template>
