<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { onBeforeRouteLeave } from 'vue-router'
import { api, ApiError, auth } from '../api'
import { useEvents } from '../events'
import MarkdownEditor from '../components/MarkdownEditor.vue'
import { clearLocalReadme, currentOfflineScope, getLocalReadme, isOfflineScopeActive, saveLocalReadme } from '../offline'
import type { LocalReadmeDraft, OfflineScope } from '../offline'
import type { ReadmeAuthor, ReadmeAuthors } from '../types'
import { formatDate, toast } from '../utils'
import '../readme.css'

interface Draft { content: string; base_content: string; base_sha: string; version: number; updated_at: string; last_commit_url: string }
interface State { draft: Draft; authors: ReadmeAuthor[]; selected_author_id: string; github: { configured: boolean; repository: string; branch: string; path: string }; remote_error: string }
type Merge = { kind: 'remote'; content: string; sha: string } | { kind: 'draft'; content: string; version: number }

const state = ref<State>()
const content = ref('')
const loading = ref(true)
const busy = ref(false)
const error = ref('')
const editor = ref<InstanceType<typeof MarkdownEditor>>()
const selectedAuthorID = ref('')
const selectedAuthor = computed(() => state.value?.authors.find(author => author.id === selectedAuthorID.value))
const pendingAuthor = ref<ReadmeAuthor>()
const pushOpen = ref(false)
const message = ref('docs: 更新游戏 README')
const messageInput = ref<HTMLInputElement>()
const merge = ref<Merge>()
const mergedContent = ref('')
const recovery = ref<LocalReadmeDraft>()
const localError = ref('')
const dirty = computed(() => !!state.value && content.value !== state.value.draft.content)
const unpublished = computed(() => !!state.value && content.value !== state.value.draft.base_content)
let scope: OfflineScope | undefined
let timer: ReturnType<typeof setTimeout> | undefined
let pendingLocal: Promise<unknown> = Promise.resolve()
let disposed = false
let authorRequest = 0

function readCurrent() { content.value = editor.value?.getValue() ?? content.value; return content.value }
function applyDraft(draft: Draft, replace = true) {
  if (!state.value) return
  state.value.draft = draft
  if (replace) content.value = draft.content
}
async function backupLocal() {
  if (!state.value || !scope || !isOfflineScopeActive(scope)) return
  const { repository, branch } = state.value.github
  const snapshot = { content: content.value, version: state.value.draft.version, base_sha: state.value.draft.base_sha }
  pendingLocal = saveLocalReadme(scope, repository, branch, snapshot)
  try { await pendingLocal; localError.value = '' }
  catch { if (scope && isOfflineScopeActive(scope)) localError.value = '本机备份暂时不可用，请使用“保存”保留草稿。' }
}
async function clearBackup() {
  clearTimeout(timer)
  await pendingLocal.catch(() => {})
  try {
    if (state.value && scope && isOfflineScopeActive(scope)) await clearLocalReadme(scope, state.value.github.repository, state.value.github.branch)
  } catch { localError.value = '本机备份暂时不可用，网站中保存的草稿不受影响。' }
}
watch(content, () => {
  clearTimeout(timer)
  if (!loading.value && dirty.value) timer = setTimeout(() => { void backupLocal() }, 600)
})
function openMerge(target: Merge, candidate = readCurrent()) {
  merge.value = target; mergedContent.value = candidate; pushOpen.value = false
}
function handleError(cause: unknown) {
  error.value = (cause as Error).message
  const candidate = merge.value ? mergedContent.value : readCurrent()
  if (cause instanceof ApiError && cause.code === 'readme_remote_conflict' && cause.details?.remote) {
    openMerge({ kind: 'remote', ...cause.details.remote }, candidate)
  } else if (cause instanceof ApiError && cause.code === 'readme_draft_conflict' && cause.details?.draft) {
    openMerge({ kind: 'draft', content: cause.details.draft.content, version: cause.details.draft.version }, candidate)
  } else if (cause instanceof ApiError && ['readme_author_conflict', 'readme_author_unavailable'].includes(cause.code)) {
    pushOpen.value = false; pendingAuthor.value = undefined
    void refreshAuthors().catch(() => {})
  }
}
async function run(action: () => Promise<void>) {
  if (busy.value) return
  error.value = ''; busy.value = true
  try { await action() } catch (cause) { handleError(cause) } finally { busy.value = false }
}
async function load() {
  loading.value = true; error.value = ''
  try {
    const result = await api<State>('/readme')
    if (disposed) return
    state.value = result; content.value = result.draft.content
    selectedAuthorID.value = result.selected_author_id
    error.value = result.remote_error
    scope = currentOfflineScope()
    try {
      const local = await getLocalReadme(scope, result.github.repository, result.github.branch)
      if (disposed || !isOfflineScopeActive(scope)) return
      if (local && local.content !== result.draft.content) {
        if (local.version === result.draft.version && local.base_sha === result.draft.base_sha) {
          content.value = local.content; toast('已恢复本机尚未保存的 README 修改')
        } else recovery.value = local
      }
    } catch { localError.value = '本机备份暂时不可用，仍可编辑并保存草稿。' }
  } catch (cause) { handleError(cause) } finally { loading.value = false }
}
async function persistDraft(notify = true) {
  if (!state.value) return
  const snapshot = readCurrent()
  if (snapshot !== state.value.draft.content) {
    const draft = await api<Draft>('/readme', { method: 'PUT', body: JSON.stringify({ content: snapshot, version: state.value.draft.version }) })
    applyDraft(draft, false)
  }
  await clearBackup()
  if (notify) toast('草稿已保存，尚未推送到 GitHub')
}
function save() { void run(() => persistDraft()) }
async function refreshAuthors() {
  const request = ++authorRequest
  const result = await api<ReadmeAuthors>('/readme/authors')
  if (disposed || request !== authorRequest || !state.value) return
  state.value.authors = result.items
  state.value.selected_author_id = result.selected_author_id
  if (!result.items.some(author => author.id === selectedAuthorID.value)) selectedAuthorID.value = ''
}
function onFocus() { if (!loading.value && state.value) void refreshAuthors().catch(() => {}) }
function onVisibilityChange() { if (document.visibilityState === 'visible') onFocus() }
useEvents(onFocus)
function selectAuthor() {
  const author = selectedAuthor.value
  const previous = state.value?.selected_author_id || ''
  ++authorRequest
  void run(async () => {
    try {
      await api('/readme/author', { method: 'PUT', body: JSON.stringify({ author_id: author?.id || '', author_version: author?.version || 0 }) })
      if (state.value) state.value.selected_author_id = author?.id || ''
    } catch (cause) { selectedAuthorID.value = previous; throw cause }
  })
}
function updateRemote() {
  void run(async () => {
    if (!state.value) return
    readCurrent()
    if (content.value !== state.value.draft.base_content) {
      const remote = await api<{ content: string; sha: string }>('/readme/remote')
      openMerge({ kind: 'remote', ...remote })
    } else {
      const draft = await api<Draft>('/readme/refresh', { method: 'POST', body: JSON.stringify({ version: state.value.draft.version }) })
      applyDraft(draft); await clearBackup(); toast('已读取 GitHub 上的 README')
    }
  })
}
function preparePush() {
  readCurrent()
  const author = selectedAuthor.value
  if (!state.value || !author) { error.value = '请先选择本次提交的作者，可在设置中新增作者'; return }
  if (!state.value.draft.base_sha) { error.value = '请先点击“读取远端”，读取 README 并合并草稿后再提交'; return }
  error.value = ''; pendingAuthor.value = { ...author }; pushOpen.value = true
  void nextTick(() => messageInput.value?.focus())
}
function push() {
  void run(async () => {
    if (!state.value || !pendingAuthor.value) return
    const author = { ...pendingAuthor.value }
    await persistDraft(false)
    const draft = await api<Draft>('/readme/push', { method: 'POST', body: JSON.stringify({ version: state.value.draft.version, message: message.value, author_id: author.id, author_version: author.version }) })
    applyDraft(draft); await clearBackup(); pushOpen.value = false; toast('README 已提交并推送到 GitHub')
  })
}
function acceptMerge() {
  void run(async () => {
    if (!state.value || !merge.value) return
    const target = merge.value
    const draft = await api<Draft>(target.kind === 'remote' ? '/readme/rebase' : '/readme', {
      method: target.kind === 'remote' ? 'POST' : 'PUT',
      body: JSON.stringify(target.kind === 'remote'
        ? { version: state.value.draft.version, remote_sha: target.sha, content: mergedContent.value }
        : { version: target.version, content: mergedContent.value }),
    })
    applyDraft(draft); merge.value = undefined; recovery.value = undefined; await clearBackup(); toast('合并草稿已保存，尚未推送')
  })
}
function recoverOlderDraft() {
  if (state.value && recovery.value) openMerge({ kind: 'draft', content: state.value.draft.content, version: state.value.draft.version }, recovery.value.content)
}
async function discardRecovery() { recovery.value = undefined; await clearBackup() }
function beforeUnload(event: BeforeUnloadEvent) {
  if (scope && isOfflineScopeActive(scope) && state.value && readCurrent() !== state.value.draft.content) {
    void backupLocal(); event.preventDefault(); event.returnValue = ''
  }
}
onBeforeRouteLeave(async () => {
  if (!scope || !isOfflineScopeActive(scope) || auth.user?.id !== scope.userID) return true
  readCurrent()
  if (dirty.value && !confirm('README 的修改尚未保存，确定离开？本机备份会保留供下次恢复。')) return false
  if (dirty.value) await backupLocal()
  return true
})
onMounted(() => { void load(); window.addEventListener('beforeunload', beforeUnload); window.addEventListener('focus', onFocus); document.addEventListener('visibilitychange', onVisibilityChange) })
onBeforeUnmount(() => { disposed = true; clearTimeout(timer); window.removeEventListener('beforeunload', beforeUnload); window.removeEventListener('focus', onFocus); document.removeEventListener('visibilitychange', onVisibilityChange) })
</script>

<template>
  <section class="readme-page">
    <header class="readme-heading">
      <div class="readme-title"><h1>读我</h1><span v-if="state?.github.branch" class="readme-branch" :aria-label="`当前分支 ${state.github.branch}`">{{ state.github.branch }}</span></div>
      <div v-if="state" class="readme-actions"><button class="secondary" :disabled="busy || !state.github.configured" @click="updateRemote">读取远端</button><button class="secondary" title="保存个人草稿" :disabled="busy || !dirty" @click="save">{{ busy ? '处理中…' : '保存' }}</button><button class="primary" :disabled="busy || !state.github.configured || !unpublished" @click="preparePush">提交推送</button></div>
    </header>
    <p v-if="error" class="readme-notice error-text" role="alert">{{ error }}</p>
    <p v-if="localError" class="readme-notice">{{ localError }}</p>
    <div v-if="recovery" class="readme-notice"><span>发现另一份本机未保存的草稿，保存版本已变化，可以比较后恢复。</span><button class="secondary small" @click="recoverOlderDraft">比较并恢复</button><button class="text-button" @click="discardRecovery">忽略本机草稿</button></div>
    <p v-if="loading" class="muted" aria-live="polite">正在读取 README…</p>
    <button v-else-if="!state" class="secondary" @click="load">重新加载</button>
    <template v-if="state && !loading">
      <div v-if="!state.github.configured" class="readme-notice">尚未连接游戏仓库。现在可以编辑和保存个人草稿，管理员完成 GitHub App 配置后即可读取和推送 README。</div>
      <div class="readme-workspace">
        <div class="readme-document-toolbar">
          <div class="readme-author-selection"><div class="field"><label for="readme-author-choice">提交作者</label><select id="readme-author-choice" v-model="selectedAuthorID" :disabled="busy || pushOpen || !state.authors.length" @change="selectAuthor"><option value="" disabled>{{ state.authors.length ? '选择本次提交的作者' : '请先在设置中添加作者' }}</option><option v-for="author in state.authors" :key="author.id" :value="author.id">{{ author.name }} · {{ author.email }}</option></select></div><RouterLink to="/settings?tab=readme-authors" target="_blank" rel="noopener noreferrer">管理作者 ↗</RouterLink></div>
          <div class="readme-status" role="status"><span :class="{ 'readme-unsaved': dirty }">{{ dirty ? '有未保存的修改' : unpublished ? '草稿已保存 · 尚未推送' : state.draft.base_sha ? '与已读取的 GitHub 版本一致' : '个人草稿' }}</span><div class="readme-save-meta"><time v-if="state.draft.updated_at" :datetime="state.draft.updated_at">保存于 {{ formatDate(state.draft.updated_at) }}</time><a v-if="state.draft.last_commit_url" :href="state.draft.last_commit_url" target="_blank" rel="noopener noreferrer">查看上次提交 ↗</a></div></div>
        </div>
        <MarkdownEditor ref="editor" v-model="content" :disabled="busy" @save="save" @error="error = $event" />
      </div>
    </template>

    <div v-if="pushOpen && state && pendingAuthor" class="modal-backdrop" @click.self="!busy && (pushOpen = false)">
      <form class="modal readme-push-modal" role="dialog" aria-modal="true" aria-labelledby="readme-push-title" @submit.prevent="push" @keydown.esc.stop.prevent="!busy && (pushOpen = false)">
        <h2 id="readme-push-title">提交并推送 README</h2><p>将当前文档提交到 <strong>{{ state.github.branch }}</strong> 分支。</p><p class="muted">作者：{{ pendingAuthor.name }} &lt;{{ pendingAuthor.email }}&gt;</p>
        <div class="field"><label for="readme-commit-message">提交说明</label><input id="readme-commit-message" ref="messageInput" v-model="message" maxlength="500" required :disabled="busy" /></div>
        <details class="readme-commit-preview"><summary>查看本次提交全文</summary><pre>{{ content }}</pre></details>
        <p v-if="error" class="error-text" role="alert">{{ error }}</p>
        <div class="form-actions"><button type="button" class="secondary" :disabled="busy" @click="pushOpen = false">取消</button><button class="primary" :disabled="busy || !message.trim()">{{ busy ? '正在提交…' : '确认提交推送' }}</button></div>
      </form>
    </div>
    <div v-if="merge" class="modal-backdrop" @click.self="!busy && (merge = undefined)">
      <section class="modal readme-merge-modal" role="dialog" aria-modal="true" aria-labelledby="readme-merge-title" @keydown.esc.stop.prevent="!busy && (merge = undefined)">
        <h2 id="readme-merge-title">{{ merge.kind === 'remote' ? '比较远端并合并' : '比较保存版本并合并' }}</h2><p>当前草稿已保留。参考左侧最新内容，在右侧合并需要保留的修改，完成后保存草稿。</p>
        <div class="readme-merge-columns"><div class="field"><label for="readme-incoming">{{ merge.kind === 'remote' ? 'GitHub 最新版本' : '最新保存版本' }}</label><textarea id="readme-incoming" :value="merge.content" readonly spellcheck="false" /></div><div class="field"><label for="readme-merged">合并后的 Markdown</label><textarea id="readme-merged" v-model="mergedContent" spellcheck="false" :disabled="busy" /></div></div>
        <div class="form-actions"><button class="text-button" :disabled="busy" @click="mergedContent = merge.content">将最新全文填入右侧</button><button class="secondary" :disabled="busy" @click="merge = undefined">返回继续编辑</button><button class="primary" :disabled="busy" @click="acceptMerge">保存合并草稿</button></div>
        <p v-if="error" class="error-text" role="alert">{{ error }}</p>
      </section>
    </div>
  </section>
</template>
