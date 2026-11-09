<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api'
import { formatDate } from '../utils'
import type { GitHubIssue, IssuesPage } from '../issues'
import '../issues.css'

const route = useRoute(); const router = useRouter()
const state = computed(() => ['closed', 'all'].includes(String(route.query.state)) ? String(route.query.state) : 'open')
const items = ref<GitHubIssue[]>([])
const repository = ref(''); const configured = ref(true); const nextPage = ref(0)
const loading = ref(false); const error = ref('')
let controller: AbortController | undefined
let generation = 0
async function load(reset = true) {
  if (!reset && (loading.value || !nextPage.value)) return
  controller?.abort(); controller = new AbortController()
  const current = ++generation
  loading.value = true; error.value = ''
  try {
    const result = await api<IssuesPage>(`/issues?state=${state.value}&page=${reset ? 1 : nextPage.value}`, { signal: controller.signal })
    if (current !== generation) return
    items.value = reset ? result.items : [...items.value, ...result.items.filter(issue => !items.value.some(existing => existing.number === issue.number))]
    repository.value = result.repository; configured.value = result.configured; nextPage.value = result.next_page
  } catch (cause) { if (current === generation && (cause as Error).name !== 'AbortError') error.value = (cause as Error).message }
  finally { if (current === generation) loading.value = false }
}
function filter(value: string) { void router.replace({ query: value === 'open' ? {} : { state: value } }) }
watch(state, () => { items.value = []; nextPage.value = 0; void load() }, { immediate: true })
onBeforeUnmount(() => { generation++; controller?.abort() })
</script>

<template>
  <section class="issues-page">
    <header class="page-heading issue-page-heading"><div><h1>问题</h1><p>追踪游戏的问题、建议与待办</p></div><div class="heading-actions"><button class="secondary" :disabled="loading" @click="load()">刷新</button><RouterLink v-if="configured" class="primary" to="/issues/new">新建问题</RouterLink></div></header>
    <div class="issue-list-toolbar"><div class="filter-tabs" role="group" aria-label="问题状态"><button :class="{ active: state === 'open' }" :aria-pressed="state === 'open'" @click="filter('open')">待处理</button><button :class="{ active: state === 'closed' }" :aria-pressed="state === 'closed'" @click="filter('closed')">已关闭</button><button :class="{ active: state === 'all' }" :aria-pressed="state === 'all'" @click="filter('all')">全部</button></div><a v-if="repository" :href="`https://github.com/${repository}/issues`" target="_blank" rel="noopener noreferrer">在 GitHub 查看 ↗</a></div>
    <p v-if="!configured" class="issue-notice">问题功能尚未连接 GitHub，请管理员完成现有 GitHub App 配置，并授权 Issues 读写权限。</p>
    <p v-if="error" class="error-text issue-notice" role="alert">{{ error }} <button class="text-button" :disabled="loading" @click="load()">重试</button></p>
    <div class="issue-list" :aria-busy="loading">
      <RouterLink v-for="issue in items" :key="issue.number" :to="`/issues/${issue.number}`" class="issue-row">
        <span class="issue-state-icon" :class="issue.state" aria-hidden="true">{{ issue.state === 'open' ? '◉' : '✓' }}</span>
        <div class="issue-row-main"><div class="issue-row-title"><span>{{ issue.title }}</span><span v-for="label in issue.labels" :key="label.name" class="tag issue-label">{{ label.name }}</span></div><div class="issue-row-meta"><span>#{{ issue.number }}</span><span>{{ issue.user.login }}</span><span>更新于 {{ formatDate(issue.updated_at) }}</span><span v-if="issue.assignees.length">负责人 {{ issue.assignees.map(user => user.login).join('、') }}</span></div></div>
        <span class="issue-comment-count">{{ issue.comments }} 条评论</span>
      </RouterLink>
    </div>
    <p v-if="loading && !items.length" class="issue-loading" role="status">正在读取 GitHub 问题…</p>
    <div v-else-if="configured && !error && !items.length && !nextPage" class="empty-state">{{ state === 'open' ? '目前没有待处理的问题' : state === 'closed' ? '还没有已关闭的问题' : '还没有问题' }}</div>
    <button v-if="nextPage && !error" class="secondary load-more" :disabled="loading" @click="load(false)">{{ loading ? '正在加载…' : '加载更多问题' }}</button>
  </section>
</template>
