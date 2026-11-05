<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import { useEvents } from '../events'
import { categoryIdentity, statusLabels } from '../utils'
import type { ChoiceOption } from '../types'
import SearchCreateSelect from './SearchCreateSelect.vue'

const value = defineModel<string>({ required: true })
const props = defineProps<{ kind: 'categories' | 'other-categories' | 'statuses'; label: string; fallback?: string }>()
const options = ref<ChoiceOption[]>([])
const loading = ref(true)
const error = ref('')
const isStatus = computed(() => props.kind === 'statuses')
const missing = computed(() => !loading.value && !error.value && !!value.value && !options.value.some(option => isStatus.value ? option.value === value.value : categoryIdentity(option.value) === categoryIdentity(value.value)))
let requestID = 0
async function load() {
  const request = ++requestID
  try {
    const result = await api<{ items: ChoiceOption[] }>(`/product-options/${props.kind}`)
    if (request !== requestID) return
    options.value = result.items
    if (isStatus.value && !value.value && options.value.length) value.value = (options.value.find(option => option.value === 'concept') || options.value[0]).value
    error.value = ''
  } catch (e) { if (request === requestID) error.value = (e as Error).message }
  finally { if (request === requestID) loading.value = false }
}
onMounted(load)
useEvents(load)
</script>

<template>
  <div class="record-option-select" :class="{ 'is-status': isStatus }">
    <SearchCreateSelect v-model="value" :label="label" :options="options" create-label="" :allow-create="false" :compact="true" :allow-empty="!isStatus" :empty-label="`未设置${label}`" :fallback="fallback || (isStatus ? statusLabels[value] : value)" :loading="loading" :error="error" @retry="load">
      <template #footer><RouterLink :to="`/settings/options?kind=${kind}`" target="_blank" rel="noopener" :title="`在新标签页管理${label}`">管理{{ label }} ↗</RouterLink></template>
    </SearchCreateSelect>
    <small v-if="missing" class="error-text" role="status">{{ isStatus ? '该制作状态已被删除，请重新选择。' : '该分类已被删除，请重新选择。' }}</small>
  </div>
</template>

<style scoped>
.record-option-select{min-width:0}
.record-option-select.is-status{max-width:22rem}
.record-option-select>small{display:block;margin-top:.35rem}
</style>
