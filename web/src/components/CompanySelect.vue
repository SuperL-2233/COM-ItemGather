<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import type { Company } from '../types'
import { companyTitle } from '../utils'
import { useEvents } from '../events'
import CompanyCreateDialog from './CompanyCreateDialog.vue'
const value = defineModel<string>({ required: true })
const props = defineProps<{ exclude?: string; fallback?: string }>()
const options = ref<Company[]>([]); const open = ref(false); const query = ref(''); const creating = ref(false); const error = ref('')
const selected = computed(() => options.value.find(c => c.id === value.value))
const filtered = computed(() => options.value.filter(c => c.id !== props.exclude && [c.chinese_name,c.english_name,c.aliases].join(' ').toLocaleLowerCase().includes(query.value.trim().toLocaleLowerCase())))
async function load() { try { options.value = (await api<{ items: Company[] }>('/companies')).items } catch (e) { error.value = (e as Error).message } }
function created(company: Company) { options.value = [...options.value.filter(c => c.id !== company.id),company]; value.value = company.id; creating.value = false; open.value = false; query.value = '' }
onMounted(load); useEvents(load)
</script>
<template><div class="company-select"><button type="button" class="select-trigger" aria-label="所属公司" :aria-expanded="open" @click="open = !open">{{ selected ? companyTitle(selected) : value ? (fallback || '已选公司') : '未设置公司' }} <span aria-hidden="true">⌄</span></button><div v-if="open" class="choice-panel" @keydown.esc.stop="open = false"><input v-model="query" type="search" aria-label="搜索公司" placeholder="搜索公司中文名、英文名或别名" autofocus @keydown.enter.stop.prevent /><div class="choice-list"><button type="button" class="choice-row" @click="value = ''; open = false">未设置公司</button><button v-for="company in filtered" :key="company.id" type="button" class="choice-row" :aria-pressed="value === company.id" @click="value = company.id; open = false"><img v-if="company.attachments[0]" :src="company.attachments[0].url" alt="" class="company-logo-mini" /><span><strong>{{ companyTitle(company) }}</strong><small v-if="company.chinese_name && company.english_name">{{ company.english_name }}</small></span><span v-if="value === company.id" aria-hidden="true">✓</span></button><p v-if="!filtered.length" class="muted">没有匹配公司。</p></div><button type="button" class="button-link" @click="creating = true">＋ 创建公司</button><p v-if="error" class="error-text">{{ error }}</p></div><CompanyCreateDialog v-if="creating" :initial-name="query" @created="created" @cancel="creating = false" /></div></template>
