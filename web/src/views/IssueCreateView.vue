<script setup lang="ts">
import { onBeforeUnmount, ref } from 'vue'
import { onBeforeRouteLeave, useRouter } from 'vue-router'
import { api, auth } from '../api'
import { toast } from '../utils'
import IssueComposer from '../components/IssueComposer.vue'
import type { GitHubIssue } from '../issues'
import '../issues.css'

const router = useRouter()
const title = ref(''); const body = ref(''); const busy = ref(false); const error = ref('')
let posted = false
async function submit() {
  if (busy.value || !title.value.trim()) return
  busy.value = true; error.value = ''
  try {
    const issue = await api<GitHubIssue>('/issues', { method: 'POST', body: JSON.stringify({ title: title.value, body: body.value }) })
    posted = true; toast('问题已发布到 GitHub'); await router.replace(`/issues/${issue.number}`)
  } catch (cause) { error.value = (cause as Error).message }
  finally { busy.value = false }
}
function beforeUnload(event: BeforeUnloadEvent) { if (!posted && (title.value || body.value)) { event.preventDefault(); event.returnValue = '' } }
window.addEventListener('beforeunload', beforeUnload)
onBeforeUnmount(() => window.removeEventListener('beforeunload', beforeUnload))
onBeforeRouteLeave(() => posted || (!busy.value && ((!title.value && !body.value) || confirm('问题尚未发布，确定离开并丢弃填写内容？'))))
</script>

<template>
  <section class="issues-page issue-create-page">
    <RouterLink class="issue-back" to="/issues">← 返回问题</RouterLink>
    <header class="page-heading"><div><h1>新建问题</h1><p>描述遇到的问题，或记录一个改进建议</p></div></header>
    <div class="panel issue-create-form"><div class="field"><label for="issue-title">标题</label><input id="issue-title" v-model="title" :disabled="busy" maxlength="256" placeholder="用一句话概括问题" /></div><div class="field"><label>问题说明</label><IssueComposer v-model="body" label="问题说明" :disabled="busy" placeholder="发生了什么？如何复现？期望的结果是什么？" /></div><p class="issue-publishing-identity">通过 GitHub 机器人发布，正文会注明 {{ auth.user?.nickname }}。</p><p v-if="error" class="error-text" role="alert">{{ error }}</p><div class="form-actions"><button class="secondary" :disabled="busy" @click="router.push('/issues')">取消</button><button class="primary push-right" :disabled="busy || !title.trim()" @click="submit">{{ busy ? '正在发布…' : '发布问题' }}</button></div></div>
  </section>
</template>
