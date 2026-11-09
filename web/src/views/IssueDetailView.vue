<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { onBeforeRouteLeave, useRoute } from 'vue-router'
import { api, auth } from '../api'
import { formatDate, toast } from '../utils'
import IssueMarkdown from '../components/IssueMarkdown.vue'
import IssueComposer from '../components/IssueComposer.vue'
import type { GitHubIssue, IssueComment, IssueCommentsPage } from '../issues'
import '../issues.css'

const route = useRoute(); const number = String(route.params.number)
const issue = ref<GitHubIssue>(); const comments = ref<IssueComment[]>([]); const nextPage = ref(0)
const loading = ref(true); const commentsLoading = ref(false); const busy = ref(false)
const error = ref(''); const commentsError = ref(''); const body = ref('')
const controller = new AbortController()
async function loadComments(reset = true) {
  if (busy.value || commentsLoading.value || (!reset && !nextPage.value)) return
  commentsLoading.value = true; commentsError.value = ''
  try {
    const result = await api<IssueCommentsPage>(`/issues/${number}/comments?page=${reset ? 1 : nextPage.value}`, { signal: controller.signal })
    comments.value = reset ? result.items : [...comments.value, ...result.items.filter(item => !comments.value.some(existing => existing.id === item.id))].sort((a, b) => a.id - b.id)
    nextPage.value = result.next_page
  } catch (cause) { if ((cause as Error).name !== 'AbortError') commentsError.value = (cause as Error).message }
  finally { commentsLoading.value = false }
}
async function load() {
  loading.value = true; error.value = ''
  try {
    const result = await api<{ issue: GitHubIssue }>(`/issues/${number}`, { signal: controller.signal })
    issue.value = result.issue
    await loadComments()
  } catch (cause) { if ((cause as Error).name !== 'AbortError') error.value = (cause as Error).message }
  finally { loading.value = false }
}
async function changeState() {
  if (!issue.value || busy.value) return
  busy.value = true; error.value = ''
  try {
    issue.value = await api<GitHubIssue>(`/issues/${number}`, { method: 'PATCH', body: JSON.stringify({ state: issue.value.state === 'open' ? 'closed' : 'open' }) })
    toast(issue.value.state === 'closed' ? '问题已关闭' : '问题已重新打开')
  } catch (cause) { error.value = (cause as Error).message }
  finally { busy.value = false }
}
async function comment() {
  if (busy.value || loading.value || commentsLoading.value || !body.value.trim()) return
  busy.value = true; commentsError.value = ''
  try {
    const result = await api<IssueComment>(`/issues/${number}/comments`, { method: 'POST', body: JSON.stringify({ body: body.value }) })
    if (!comments.value.some(existing => existing.id === result.id)) comments.value.push(result)
    if (issue.value) issue.value.comments++
    body.value = ''; toast('评论已发布到 GitHub')
  } catch (cause) { commentsError.value = (cause as Error).message }
  finally { busy.value = false }
}
function beforeUnload(event: BeforeUnloadEvent) { if (body.value) { event.preventDefault(); event.returnValue = '' } }
onMounted(() => { void load(); window.addEventListener('beforeunload', beforeUnload) })
onBeforeUnmount(() => { controller.abort(); window.removeEventListener('beforeunload', beforeUnload) })
onBeforeRouteLeave(() => !busy.value && (!body.value || confirm('评论尚未发布，确定离开并丢弃填写内容？')))
</script>

<template>
  <section class="issues-page issue-detail-page">
    <RouterLink class="issue-back" to="/issues">← 返回问题</RouterLink>
    <p v-if="error" class="error-text issue-notice" role="alert">{{ error }} <button class="text-button" :disabled="loading || busy" @click="load()">刷新重试</button></p>
    <p v-if="loading && !issue" class="issue-loading" role="status">正在读取问题…</p>
    <template v-if="issue">
      <header class="issue-detail-heading"><div><div class="issue-detail-title"><h1>{{ issue.title }} <span>#{{ issue.number }}</span></h1><span class="issue-state-badge" :class="issue.state">{{ issue.state === 'open' ? '待处理' : '已关闭' }}</span></div><p class="issue-row-meta">{{ issue.user.login }} · 创建于 {{ formatDate(issue.created_at) }} · {{ issue.comments }} 条评论</p></div><div class="heading-actions"><a class="secondary" :href="issue.html_url" target="_blank" rel="noopener noreferrer">在 GitHub 查看 ↗</a><button class="secondary" :disabled="busy || loading" @click="changeState">{{ issue.state === 'open' ? '关闭问题' : '重新打开' }}</button></div></header>
      <div v-if="issue.labels.length || issue.assignees.length" class="issue-detail-labels"><span v-for="label in issue.labels" :key="label.name" class="tag">{{ label.name }}</span><span v-if="issue.assignees.length" class="muted">负责人 {{ issue.assignees.map(user => user.login).join('、') }}</span></div>
      <article class="panel issue-description" aria-label="问题内容"><IssueMarkdown :content="issue.body" /></article>
      <div class="issue-comments-heading"><h2>讨论</h2><button class="text-button" :disabled="loading || busy || commentsLoading" @click="load()">刷新讨论</button></div>
      <article v-for="item in comments" :key="item.id" class="panel issue-comment" :aria-label="`${item.user.login} 的评论`"><header><strong>{{ item.user.login }}</strong><a :href="item.html_url" target="_blank" rel="noopener noreferrer">{{ formatDate(item.created_at) }}</a></header><IssueMarkdown :content="item.body" /></article>
      <button v-if="nextPage" class="secondary load-more" :disabled="commentsLoading || busy" @click="loadComments(false)">{{ commentsLoading ? '正在加载…' : '加载更多评论' }}</button>
      <p v-if="commentsLoading && !comments.length" class="muted" role="status">正在读取评论…</p>
      <p v-if="commentsError" class="error-text issue-notice" role="alert">{{ commentsError }} <button class="text-button" :disabled="commentsLoading || busy" @click="loadComments()">重新读取评论</button></p>
      <p v-if="issue.locked" class="issue-notice">此问题已锁定讨论，请在 GitHub 中查看。</p>
      <section v-else class="panel issue-reply"><h2>添加评论</h2><IssueComposer v-model="body" label="评论内容" :disabled="busy" placeholder="补充信息，或讨论解决办法…" /><div class="issue-reply-actions"><p class="issue-publishing-identity">通过 GitHub 机器人发布，正文会注明 {{ auth.user?.nickname }}。</p><button class="primary" :disabled="busy || loading || commentsLoading || !body.trim()" @click="comment">{{ busy ? '正在处理…' : '发布评论' }}</button></div></section>
    </template>
  </section>
</template>
