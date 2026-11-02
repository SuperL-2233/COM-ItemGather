<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import type { Company } from '../types'
import { companyTitle } from '../utils'
import { useEvents } from '../events'
import CompanyCreateDialog from './CompanyCreateDialog.vue'
import SearchCreateSelect from './SearchCreateSelect.vue'
const value = defineModel<string>({ required: true })
const props = defineProps<{ exclude?: string; fallback?: string }>()
const options = ref<Company[]>([])
const creating = ref(false)
const initialName = ref('')
const error = ref('')
const loading = ref(true)
let requestID = 0
const selector = ref<InstanceType<typeof SearchCreateSelect>>()
function option(company: Company) {
  return { value: company.id, label: companyTitle(company), searchText: [company.chinese_name,company.english_name,company.aliases].join(' '), description: company.chinese_name ? company.english_name : '', image: company.attachments[0]?.url }
}
const choices = computed(() => options.value.filter(c => c.id !== props.exclude).map(option))
async function load() {
  const request = ++requestID
  try { const result = await api<{ items: Company[] }>('/companies'); if (request !== requestID) return; options.value = result.items; error.value = '' }
  catch (e) { if (request === requestID) error.value = (e as Error).message }
  finally { loading.value = false }
}
async function create(name: string) {
  let company = options.value.find(c => c.id !== props.exclude && [c.chinese_name,c.english_name].some(text => text.toLocaleLowerCase() === name.toLocaleLowerCase()))
  if (!company) company = await api<Company>('/companies',{ method: 'POST', body: JSON.stringify({ chinese_name: name }) })
  requestID++
  options.value = [...options.value.filter(c => c.id !== company.id),company]
  return option(company)
}
function created(company: Company) {
  requestID++
  options.value = [...options.value.filter(c => c.id !== company.id),company]
  creating.value = false
  void selector.value?.select(company.id)
}
function details(name: string) { initialName.value = name; creating.value = true }
onMounted(load)
useEvents(load)
</script>
<template><div class="company-select"><SearchCreateSelect ref="selector" v-model="value" label="所属公司" search-label="搜索公司" new-label="新公司名称" :options="choices" :create="create" create-label="＋ 创建公司" empty-label="未设置公司" :fallback="fallback" :max-length="160" :loading="loading" :error="error" full-details @details="details" @retry="load" /><CompanyCreateDialog v-if="creating" :initial-name="initialName" @created="created" @cancel="creating = false" /></div></template>
