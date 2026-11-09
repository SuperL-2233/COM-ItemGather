<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { api, ApiError } from '../api'
import type { ChoiceOption, Company, Product } from '../types'
import { companyTitle } from '../utils'
import { useEvents } from '../events'
import SearchCreateSelect from './SearchCreateSelect.vue'

const props = defineProps<{ products: Product[] }>()
const emit = defineEmits<{ saved: [count: number]; cancel: []; reselect: [] }>()
const dialog = ref<HTMLDialogElement>()
const heading = ref<HTMLElement>()
const previousFocus = document.activeElement as HTMLElement | null
const keep = 'keep'
const category = ref(keep)
const status = ref(keep)
const company = ref(keep)
const categories = ref<ChoiceOption[]>([])
const statuses = ref<ChoiceOption[]>([])
const companies = ref<Company[]>([])
const loading = ref(true)
const loadError = ref('')
const error = ref('')
const busy = ref(false)
const needsReselection = ref(false)
let requestID = 0
function choices(options: ChoiceOption[]) {
  return [{ value: keep, label: '保持不变' }, ...options.map(option => ({ ...option, value: `set:${option.value}` }))]
}
const categoryChoices = computed(() => choices(categories.value))
const statusChoices = computed(() => choices(statuses.value))
const companyChoices = computed(() => choices(companies.value.map(item => ({ value: item.id, label: companyTitle(item), searchText: `${item.chinese_name} ${item.english_name} ${item.aliases}`, version: item.version }))))
const changes = computed(() => {
  const result: Record<string, string> = {}
  if (category.value !== keep) result.category = category.value.slice(4)
  if (status.value !== keep) result.development_status = status.value.slice(4)
  if (company.value !== keep) result.company_id = company.value.slice(4)
  return result
})
const validChoices = computed(() => (!category.value || categoryChoices.value.some(item => item.value === category.value)) && statusChoices.value.some(item => item.value === status.value) && (!company.value || companyChoices.value.some(item => item.value === company.value)))
const canSave = computed(() => !loading.value && !loadError.value && !busy.value && !needsReselection.value && validChoices.value && Object.keys(changes.value).length > 0)
async function load() {
  const request = ++requestID
  try {
    const [categoryResult, statusResult, companyResult] = await Promise.all([
      api<{ items: ChoiceOption[] }>('/product-options/categories'),
      api<{ items: ChoiceOption[] }>('/product-options/statuses'),
      api<{ items: Company[] }>('/companies'),
    ])
    if (request !== requestID) return
    categories.value = categoryResult.items
    statuses.value = statusResult.items
    companies.value = companyResult.items
    loadError.value = ''
  } catch (e) { if (request === requestID) loadError.value = (e as Error).message }
  finally { if (request === requestID) loading.value = false }
}
async function save() {
  if (!canSave.value) return
  busy.value = true
  error.value = ''
  try {
    const result = await api<{ updated: number }>('/products/batch', { method: 'PATCH', body: JSON.stringify({ items: props.products.map(item => ({ id: item.id, version: item.version })), ...changes.value }) })
    emit('saved', result.updated)
  } catch (e) {
    error.value = (e as Error).message
    needsReselection.value = e instanceof ApiError && ['version_conflict', 'selection_changed'].includes(e.code)
  } finally { busy.value = false }
}
function cancel() { if (!busy.value) emit('cancel') }
onMounted(() => { dialog.value?.showModal(); heading.value?.focus(); void load() })
onUnmounted(() => { requestID++; dialog.value?.close(); if (previousFocus?.isConnected) previousFocus.focus() })
useEvents(load)
</script>

<template>
  <Teleport to="body">
    <dialog ref="dialog" class="modal batch-edit-dialog" aria-labelledby="batch-edit-title" aria-describedby="batch-edit-description" @cancel.prevent="cancel">
      <form @submit.prevent="save">
        <h2 id="batch-edit-title" ref="heading" tabindex="-1">批量修改商品</h2>
        <p id="batch-edit-description" class="muted">已选 {{ products.length }} 件商品。只修改你选择的项目。公司和分类由同一商品的各款共享，会一起更新。</p>
        <fieldset :disabled="busy || loading">
          <div class="field"><span class="field-label">商品分类</span><SearchCreateSelect v-model="category" label="批量商品分类" :options="categoryChoices" create-label="" :allow-create="false" compact empty-label="清空分类" :loading="loading" /></div>
          <div class="field"><span class="field-label">制作状态</span><SearchCreateSelect v-model="status" label="批量制作状态" :options="statusChoices" create-label="" :allow-create="false" :allow-empty="false" compact :loading="loading" /></div>
          <div class="field"><span class="field-label">所属公司</span><SearchCreateSelect v-model="company" label="批量所属公司" :options="companyChoices" create-label="" :allow-create="false" compact empty-label="清空公司" :loading="loading" /></div>
        </fieldset>
        <p v-if="loadError" class="error-text" role="alert">{{ loadError }} <button type="button" class="button-link" @click="load">重试</button></p>
        <p v-else-if="!loading && !validChoices" class="error-text" role="alert">所选选项已被移除，请重新选择。</p>
        <p v-if="error" class="error-text" role="alert">{{ error }} <button v-if="needsReselection" type="button" class="button-link" @click="emit('reselect')">重新选择商品</button></p>
        <div class="modal-actions"><button type="button" class="secondary" :disabled="busy" @click="cancel">取消</button><button class="primary" :disabled="!canSave">{{ busy ? '正在应用…' : `应用到 ${products.length} 件商品` }}</button></div>
      </form>
    </dialog>
  </Teleport>
</template>

<style scoped>
.batch-edit-dialog{width:min(460px,calc(100% - 1.3rem));max-height:93dvh;overflow:visible;color:var(--text);margin:auto}
.batch-edit-dialog::backdrop{background:#1118}
.batch-edit-dialog h2{margin-bottom:.6rem;outline:none}
.batch-edit-dialog p{font-size:.85rem;line-height:1.6}
.batch-edit-dialog fieldset{border:0;padding:0;margin:1.3rem 0 0;min-width:0}
.batch-edit-dialog .field{margin-bottom:1rem}
.batch-edit-dialog .field:last-child{margin-bottom:0}
.batch-edit-dialog .field-label{display:block;margin-bottom:.4rem}
.batch-edit-dialog .modal-actions{margin-top:1.5rem}
@media(max-height:540px){.batch-edit-dialog{overflow:auto}}
</style>
