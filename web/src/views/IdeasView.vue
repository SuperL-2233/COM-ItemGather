<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from 'vue'
import { api } from '../api'
import { listOutbox, queueOutbox, removeOutbox, saveDraft, getDraft, clearDraft, currentOfflineScope, isOfflineScopeActive, type OutboxItem, type PendingFile } from '../offline'
import type { Idea, Page } from '../types'
import IdeaCard from '../components/IdeaCard.vue'
import { useEvents } from '../events'
import { toast } from '../utils'
import FilterPopover from '../components/FilterPopover.vue'
import { useLiveSearch } from '../composables/useLiveSearch'
import { loadListPages, useListNavigation } from '../composables/listNavigation'

const scope = currentOfflineScope()
const requests = new AbortController()
let disposed = false
let draftChanged = false
const active = () => !disposed && isOfflineScopeActive(scope)
type IdeaListState = { items: Idea[]; total: number; query: string; filters: Record<string, string[]> }
const previous: IdeaListState | undefined = useListNavigation<IdeaListState>(() => ({ items: ideas.value, total: total.value, query: query.value, filters: filters.value }))
const text = ref(''); const files = ref<PendingFile[]>([]); const ideas = ref<Idea[]>(previous?.items || []); const total = ref(previous?.total || 0); const composer = ref<HTMLTextAreaElement>(); const fileInput = ref<HTMLInputElement>(); const dragging = ref(false); const syncState = ref(''); const syncError = ref(false); const outbox = ref<OutboxItem[]>([]); const activeJob = ref(''); const syncing = new Set<string>(); let draftTimer: ReturnType<typeof setTimeout> | undefined;
const placeholder = '想到什么就写什么。每次记录一条灵感，名称、概念、玩法、包装都可以。'
function addFiles(incoming: File[] | FileList) {
  const allowed = Array.from(incoming).filter(file => ['image/jpeg','image/png','image/webp'].includes(file.type))
  if (allowed.length !== Array.from(incoming).length) toast('仅支持 JPEG、PNG 和 WebP 图片')
  const room = Math.max(0, 10 - files.value.length)
  files.value.push(...allowed.slice(0, room).map(file => ({ id: crypto.randomUUID(), blob: file, name: file.name || `截图-${Date.now()}.png`, type: file.type })))
  if (allowed.length > room) toast('每条灵感最多添加 10 张图片')
}
function paste(event: ClipboardEvent) { if (event.clipboardData?.files.length) { addFiles(event.clipboardData.files); event.preventDefault() } }
function drop(event: DragEvent) { dragging.value = false; if (event.dataTransfer?.files.length) addFiles(event.dataTransfer.files) }
function choose(event: Event) { const input = event.target as HTMLInputElement; if (input.files) addFiles(input.files); input.value = '' }
function keydown(event: KeyboardEvent) { if ((event.ctrlKey || event.metaKey) && event.key === 'Enter') { event.preventDefault(); submit() } }
function globalKey(event: KeyboardEvent) { const target = event.target as HTMLElement; if (event.key === '/' && !['INPUT','TEXTAREA','SELECT'].includes(target.tagName) && !target.isContentEditable) { event.preventDefault(); composer.value?.focus() } }
function persistDraft() {
  clearTimeout(draftTimer)
  if (!draftChanged || !isOfflineScopeActive(scope)) return
  draftChanged = false
  void saveDraft(scope, text.value, [...files.value]).catch((cause: Error) => {
    if (!active()) return
    draftChanged = true
    syncState.value = `本机草稿保存失败：${cause.message}`; syncError.value = true
  })
}
watch([text, files], () => { draftChanged = true; clearTimeout(draftTimer); draftTimer = setTimeout(persistDraft, 350) }, { deep: true })
async function refreshOutbox() {
  if (!active()) return
  const pending = await listOutbox(scope)
  if (active()) outbox.value = pending.sort((a,b) => a.created_at.localeCompare(b.created_at))
}
async function submit() {
  if (!active() || (!text.value.trim() && !files.value.length) || activeJob.value) return
  const item: OutboxItem = { id: crypto.randomUUID(), user_id: scope.userID, text: text.value, files: [...files.value], created_at: new Date().toISOString() }
  activeJob.value = item.id; syncError.value = false
  try {
    await queueOutbox(scope, item)
    if (!active()) return
    syncState.value = '已暂存在本机'; text.value = ''; files.value = []
    await clearDraft(scope); await refreshOutbox(); await syncItem(item)
  } catch (e) { if (active()) { syncState.value = '本机暂存失败，请勿关闭页面'; syncError.value = true; toast((e as Error).message) } }
  finally { activeJob.value = '' }
}
async function syncItem(item: OutboxItem) {
  if (!active() || item.user_id !== scope.userID || syncing.has(item.id)) return
  syncing.add(item.id); syncState.value = '正在同步'; syncError.value = false
  try {
    if (!item.idea_id) {
      const idea = await api<Idea>('/ideas', { method: 'POST', accountID: scope.userID, signal: requests.signal, body: JSON.stringify({ client_id: item.id, content: item.text }) })
      if (!active()) return
      item.idea_id = idea.id; item.error = ''; await queueOutbox(scope, item)
      ideas.value = [idea, ...ideas.value.filter(existing => existing.id !== idea.id)]; total.value++
    }
    while (item.files.length && active()) {
      const file = item.files[0]
      const form = new FormData(); form.append('idea_id', item.idea_id); form.append('client_id', file.id); form.append('file', file.blob, file.name)
      await api('/attachments', { method: 'POST', accountID: scope.userID, signal: requests.signal, body: form })
      if (!active()) return
      item.files.shift(); await queueOutbox(scope, item)
    }
    if (!active()) return
    await removeOutbox(scope, item.id); syncState.value = '已保存到服务器'; syncError.value = false; await loadIdeas()
  } catch (e) {
    if (!active()) return
    item.error = (e as Error).message; syncState.value = item.idea_id ? '记录已保存，图片同步失败' : '同步失败，内容仍保留在本机'; syncError.value = true
    await queueOutbox(scope, item); toast(item.error)
  } finally { syncing.delete(item.id); if (active()) await refreshOutbox() }
}
async function syncAll() {
  if (!active()) return
  for (const item of (await listOutbox(scope)).sort((a,b) => a.created_at.localeCompare(b.created_at))) {
    if (!active()) return
    await syncItem(item)
  }
}
const query = ref(previous?.query || '')
const filters = ref<Record<string, string[]>>(previous?.filters || { scope: [] })
const filterGroups = [{ key: 'scope', label: '浏览范围', allLabel: '全部灵感', multiple: false, options: [{ value: 'favorites', label: '收藏' }, { value: 'unlinked', label: '未关联商品／道具' }, { value: 'mine', label: '我创建的' }] }]
let append = false
let preserveDepth = !!previous
const { loading, error, run, schedule, refresh } = useLiveSearch(signal => {
  const parameters = new URLSearchParams({ q: query.value, filter: filters.value.scope[0] || 'all', limit: '20', offset: String(append ? ideas.value.length : 0) })
  const fetchPage = (offset: number, limit: number) => {
    parameters.set('offset', String(offset)); parameters.set('limit', String(limit))
    return api<Page<Idea>>(`/ideas?${parameters}`, { accountID: scope.userID, signal })
  }
  return !append && preserveDepth ? loadListPages(Math.max(20, ideas.value.length), fetchPage) : fetchPage(append ? ideas.value.length : 0, 20)
}, result => {
  if (!active()) return
  ideas.value = append ? [...ideas.value, ...result.items.filter(item => !ideas.value.some(existing => existing.id === item.id))] : result.items
  total.value = result.total
})
loading.value = true
function loadIdeas(reset = true, preserve = false) {
  if (!active() || (!reset && loading.value)) return
  append = !reset
  preserveDepth = preserve
  return run()
}
watch(query, () => { append = false; preserveDepth = false; schedule() }, { flush: 'sync' })
watch(filters, () => void loadIdeas())
function online() { if (active()) { syncState.value = '网络已恢复'; void syncAll() } }
onMounted(async () => {
  window.addEventListener('keydown', globalKey); window.addEventListener('online', online); window.addEventListener('pagehide', persistDraft)
  try { const draft = await getDraft(scope); if (active() && draft) { text.value = draft.text; files.value = draft.files || []; if (text.value || files.value.length) syncState.value = '已恢复本机草稿' } }
  catch { if (active()) syncState.value = '本机草稿不可用' }
  if (!active()) return
  await Promise.all([loadIdeas(true, true), refreshOutbox()])
  if (active()) await syncAll()
})
onUnmounted(() => {
  persistDraft(); disposed = true; requests.abort(); clearTimeout(draftTimer)
  window.removeEventListener('keydown', globalKey); window.removeEventListener('online', online); window.removeEventListener('pagehide', persistDraft)
})
useEvents(() => { if (active()) { if (refresh()) { append = false; preserveDepth = true }; void refreshOutbox() } })
</script>
<template>
  <section aria-labelledby="ideas-heading"><div class="page-heading"><div><h1 id="ideas-heading">灵感库</h1><p>不用填写标题或分类，先把想法记下来。</p></div></div>
    <div class="composer" :class="{ dragging }" @dragover.prevent="dragging = true" @dragleave.prevent="dragging = false" @drop.prevent="drop">
      <label for="idea-input" class="hidden">记录一条灵感</label><textarea id="idea-input" ref="composer" v-model="text" :placeholder="placeholder" maxlength="200000" @keydown="keydown" @paste="paste"></textarea>
      <div v-if="files.length" class="file-chips"><span v-for="(file,index) in files" :key="`${file.name}-${index}`" class="file-chip">{{ file.name }}<button type="button" aria-label="移除图片" @click="files.splice(index,1)">×</button></span></div>
      <div class="composer-footer"><input ref="fileInput" hidden type="file" accept="image/jpeg,image/png,image/webp" multiple @change="choose" /><button class="secondary small" type="button" @click="fileInput?.click()">添加图片</button><span class="composer-hint">每次一条 · / 聚焦 · Ctrl + Enter 保存 · 可粘贴或拖入图片</span><button class="primary" type="button" :disabled="(!text.trim() && !files.length) || !!activeJob" @click="submit">保存灵感</button></div>
      <p v-if="syncState" class="composer-status" :class="syncError ? 'error-text' : 'muted'" role="status">{{ syncState }}</p>
    </div>
    <div v-for="item in outbox" :key="item.id" class="outbox"><strong>{{ item.idea_id ? '部分已同步' : '已暂存在本机' }}</strong> · {{ item.text || `${item.files.length} 张图片` }} <span v-if="item.error" class="error-text">{{ item.error }}</span><button class="button-link" @click="syncItem(item)">重试</button></div>
    <form class="toolbar" @submit.prevent="loadIdeas()"><input v-model="query" type="search" aria-label="搜索灵感" placeholder="搜索内容、标题或分类" /><FilterPopover id="idea-filter-popover" v-model="filters" label="灵感" :groups="filterGroups" /></form>
    <p v-if="error" class="error-text">{{ error }} <button class="button-link" @click="loadIdeas()">重试</button></p><p v-if="loading && !ideas.length" class="muted">读取中…</p><div v-else-if="ideas.length" class="idea-list"><IdeaCard v-for="idea in ideas" :key="idea.id" :idea="idea" @changed="loadIdeas()" /></div><div v-else class="empty-state">还没有符合条件的灵感。直接从上方写下第一条吧。</div><button v-if="ideas.length < total" class="secondary load-more" :disabled="loading" @click="loadIdeas(false)">加载更多（{{ ideas.length }} / {{ total }}）</button>
  </section>
</template>
