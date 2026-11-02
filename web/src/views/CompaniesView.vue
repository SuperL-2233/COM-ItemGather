<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'
import type { Company } from '../types'
import { companyTitle } from '../utils'
import { useEvents } from '../events'
import { useLiveSearch } from '../composables/useLiveSearch'
import CompanyCreateDialog from '../components/CompanyCreateDialog.vue'
const router = useRouter(); const items = ref<Company[]>([]); const query = ref(''); const creating = ref(false);
const { loading, error, run: load, schedule, refresh } = useLiveSearch(signal => api<{ items: Company[] }>(`/companies?q=${encodeURIComponent(query.value)}`, { signal }), result => { items.value = result.items })
function created(company: Company) { creating.value = false; void router.push(`/companies/${company.id}`) }
onMounted(load); watch(query, schedule, { flush: 'sync' }); useEvents(refresh)
</script>
<template><section><div class="page-heading"><div><h1>公司库</h1><p>维护游戏中的公司、标识与世界观设定。</p></div><button class="primary" @click="creating = true">创建公司</button></div><form class="toolbar" @submit.prevent="load"><input v-model="query" type="search" aria-label="搜索公司" placeholder="搜索中文名、英文名、别名或设定" /></form><p v-if="error" class="error-text">{{ error }} <button class="button-link" @click="load">重试</button></p><p v-if="loading && !items.length" class="muted">读取中…</p><div v-else-if="items.length" class="product-grid"><article v-for="company in items" :key="company.id" class="product-card company-card"><RouterLink :to="`/companies/${company.id}`" class="company-cover"><img v-if="company.attachments[0]" :src="company.attachments[0].url" alt="" loading="lazy" /><span v-else class="logo-placeholder">{{ companyTitle(company).slice(0,2) }}</span></RouterLink><div class="product-card-content"><h2><RouterLink :to="`/companies/${company.id}`">{{ companyTitle(company) }}</RouterLink></h2><p v-if="company.chinese_name && company.english_name" class="product-english-name">{{ company.english_name }}</p><p class="card-summary">{{ company.description || '公司设定待补充' }}</p><span class="muted">{{ company.item_count }} 件关联商品与道具</span></div></article></div><div v-else class="empty-state">{{ query ? '没有匹配公司。' : '还没有公司，可以先创建名称，再补充 Logo 和设定。' }}</div></section><CompanyCreateDialog v-if="creating" @created="created" @cancel="creating = false" /></template>
