<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { onBeforeRouteLeave } from 'vue-router'
import { api, ApiError, auth } from '../api'
import MarkdownEditor from '../components/MarkdownEditor.vue'
import { clearLocalReadme, currentOfflineScope, getLocalReadme, isOfflineScopeActive, saveLocalReadme } from '../offline'
import type { LocalReadmeDraft, OfflineScope } from '../offline'
import { formatDate, toast } from '../utils'
import '../readme.css'

interface Draft { content: string; base_content: string; base_sha: string; version: number; updated_at: string; last_commit_url: string }
interface Author { name: string; email: string }
interface State { draft: Draft; author: Author; github: { configured: boolean; repository: string; branch: string; path: string }; remote_error: string }
type Merge = { kind: 'remote'; content: string; sha: string } | { kind: 'draft'; content: string; version: number }

const state = ref<State>()
const content = ref('')
const loading = ref(true)
const busy = ref(false)
const error = ref('')
const editor = ref<InstanceType<typeof MarkdownEditor>>()
const authorName = ref('')
const authorEmail = ref('')
const authorOpen = ref(false)
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
    authorName.value = result.author.name; authorEmail.value = result.author.email; authorOpen.value = !result.author.email
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
function saveAuthor() {
  void run(async () => {
    const author = await api<Author>('/readme/author', { method: 'PUT', body: JSON.stringify({ name: authorName.value, email: authorEmail.value }) })
    if (state.value) state.value.author = author
    authorOpen.value = false; toast('提交身份已保存')
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
  if (!state.value?.author.email) { authorOpen.value = true; error.value = '请先保存你的提交姓名和 GitHub 提交邮箱'; return }
  if (!state.value.draft.base_sha) { error.value = '请先点击“读取远端”，读取 README 并合并草稿后再提交'; return }
  error.value = ''; pushOpen.value = true
  void nextTick(() => messageInput.value?.focus())
}
function push() {
  void run(async () => {
    if (!state.value) return
    await persistDraft(false)
    const draft = await api<Draft>('/readme/push', { method: 'POST', body: JSON.stringify({ version: state.value.draft.version, message: message.value }) })
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
onMounted(() => { void load(); window.addEventListener('beforeunload', beforeUnload) })
onBeforeUnmount(() => { disposed = true; clearTimeout(timer); window.removeEventListener('beforeunload', beforeUnload) })
</script>

<template>
  <section class="readme-page">
    <div class="page-heading readme-heading">
      <div><div class="readme-eyebrow">游戏文档</div><h1>读我 <span class="readme-filename">README.md</span></h1><p v-if="state?.github.repository">{{ state.github.repository }} <span class="readme-branch">{{ state.github.branch }}</span></p><p v-else>在这里阅读、编辑游戏说明。</p></div>
      <div v-if="state" class="readme-actions"><button class="secondary" :disabled="busy || !state.github.configured" @click="updateRemote">读取远端</button><button class="secondary" :disabled="busy || !dirty" @click="save">{{ busy ? '处理中…' : '保存' }}</button><button class="primary" :disabled="busy || !state.github.configured || !unpublished" @click="preparePush">提交推送</button></div>
    </div>
    <p v-if="error" class="readme-notice error-text" role="alert">{{ error }}</p>
    <p v-if="localError" class="readme-notice">{{ localError }}</p>
    <div v-if="recovery" class="readme-notice"><span>发现另一份本机未保存的草稿，保存版本已变化，可以比较后恢复。</span><button class="secondary small" @click="recoverOlderDraft">比较并恢复</button><button class="text-button" @click="discardRecovery">忽略本机草稿</button></div>
    <p v-if="loading" class="muted" aria-live="polite">正在读取 README…</p>
    <button v-else-if="!state" class="secondary" @click="load">重新加载</button>
    <template v-if="state && !loading">
      <div v-if="!state.github.configured" class="readme-notice">尚未连接游戏仓库。现在可以编辑和保存个人草稿，管理员完成 GitHub App 配置后即可读取和推送 README。</div>
      <details class="readme-author panel" :open="authorOpen" @toggle="authorOpen = ($event.target as HTMLDetailsElement).open">
        <summary>提交身份 <span v-if="state.author.email">{{ state.author.name }} · {{ state.author.email }}</span><span v-else>首次提交前设置</span></summary>
        <form @submit.prevent="saveAuthor"><div class="field"><label for="readme-author-name">提交姓名</label><input id="readme-author-name" v-model="authorName" maxlength="100" required /></div><div class="field"><label for="readme-author-email">GitHub 提交邮箱</label><input id="readme-author-email" v-model="authorEmail" type="email" maxlength="254" placeholder="从 GitHub 的 Settings → Emails 复制" required /></div><button class="secondary" :disabled="busy">保存提交身份</button></form>
        <p class="muted">使用 GitHub 账号关联的邮箱或专属 noreply 邮箱，提交作者会关联到你。由机器人代为推送，无需授权个人账号。</p>
      </details>
      <div class="readme-status" role="status"><span :class="{ 'readme-unsaved': dirty }">{{ dirty ? '有未保存的修改' : unpublished ? '草稿已保存 · 尚未推送' : state.draft.base_sha ? '与已读取的 GitHub 版本一致' : '个人草稿' }}</span><span v-if="state.draft.updated_at">保存于 {{ formatDate(state.draft.updated_at) }}</span><a v-if="state.draft.last_commit_url" :href="state.draft.last_commit_url" target="_blank" rel="noopener noreferrer">查看上次提交 ↗</a></div>
      <MarkdownEditor ref="editor" v-model="content" :disabled="busy" @save="save" @error="error = $event" />
      <p class="readme-footnote">“保存”保留你的个人草稿；“提交推送”会将修改发布到游戏仓库。其他成员的草稿互不覆盖。</p>
    </template>

    <div v-if="pushOpen && state" class="modal-backdrop" @click.self="!busy && (pushOpen = false)">
      <form class="modal readme-push-modal" role="dialog" aria-modal="true" aria-labelledby="readme-push-title" @submit.prevent="push" @keydown.esc.stop.prevent="!busy && (pushOpen = false)">
        <h2 id="readme-push-title">提交并推送 README</h2><p>将当前全文提交到 <strong>{{ state.github.repository }}</strong> 的 <strong>{{ state.github.branch }}</strong> 分支。</p><p class="muted">作者：{{ state.author.name }} &lt;{{ state.author.email }}&gt;</p>
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
