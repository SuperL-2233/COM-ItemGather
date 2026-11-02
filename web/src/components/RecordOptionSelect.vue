<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import { useEvents } from '../events'
import { statusLabels } from '../utils'
import type { ChoiceOption } from '../types'
import SearchCreateSelect from './SearchCreateSelect.vue'
const value = defineModel<string>({ required: true })
const props = defineProps<{ kind: 'categories' | 'statuses'; label: string; fallback?: string }>()
const options = ref<ChoiceOption[]>([])
const loading = ref(true)
let requestID = 0
const error = ref('')
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
onMounted(load)
useEvents(load)
</script>
<template><SearchCreateSelect v-model="value" :label="label" :options="options" :create="create" :create-label="isStatus ? '＋ 新建制作状态' : '＋ 新建分类'" :new-label="isStatus ? '新制作状态' : '新分类名称'" :allow-empty="!isStatus" :empty-label="`未设置${label}`" :fallback="fallback || (isStatus ? statusLabels[value] : value)" :max-length="isStatus ? 80 : 100" :loading="loading" :error="error" @retry="load" /></template>
