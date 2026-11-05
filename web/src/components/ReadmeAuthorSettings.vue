<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api, ApiError } from '../api'
import { useEvents } from '../events'
import type { ReadmeAuthor, ReadmeAuthors } from '../types'
import { toast } from '../utils'

const authors = ref<ReadmeAuthor[]>([])
const editing = ref<ReadmeAuthor>()
const name = ref('')
const email = ref('')
const busy = ref(false)
const loading = ref(true)
const error = ref('')
let loadGeneration = 0

async function load() { const generation = ++loadGeneration; const result = await api<ReadmeAuthors>('/readme/authors'); if (generation === loadGeneration) authors.value = result.items }
async function run(action: () => Promise<void>) {
  if (busy.value) return
  busy.value = true; error.value = ''
  try { await action() }
  catch (cause) {
    error.value = (cause as Error).message
    if (cause instanceof ApiError && ['readme_author_conflict', 'readme_author_unavailable'].includes(cause.code)) {
      await load().catch(() => {})
      error.value += '。当前输入已保留，请重新打开作者的编辑表单核对。'
    }
  } finally { busy.value = false; loading.value = false }
}
function reset() { editing.value = undefined; name.value = ''; email.value = '' }
function edit(author: ReadmeAuthor) { editing.value = { ...author }; name.value = author.name; email.value = author.email; error.value = '' }
function save() {
  void run(async () => {
    const current = editing.value
    await api<ReadmeAuthor>(current ? `/readme/authors/${current.id}` : '/readme/authors', {
      method: current ? 'PATCH' : 'POST',
      body: JSON.stringify({ name: name.value, email: email.value, ...(current ? { version: current.version } : {}) }),
    })
    reset(); await load(); toast(current ? '作者资料已更新' : '作者已添加，可在读我页面选择')
  })
}
function remove(author: ReadmeAuthor) {
  if (!confirm(`删除作者“${author.name}”（${author.email}）？以后提交时将无法选择该作者。`)) return
  void run(async () => {
    await api(`/readme/authors/${author.id}`, { method: 'DELETE', body: JSON.stringify({ version: author.version }) })
    if (editing.value?.id === author.id) reset()
    await load(); toast('作者已删除')
  })
}
onMounted(() => { void run(load) })
useEvents(() => { if (!loading.value) void load().catch(() => {}) })
</script>

<template>
  <section class="panel readme-author-settings">
    <div class="page-heading"><div><h2>提交作者</h2><p class="muted">统一管理团队的提交姓名和 GitHub 邮箱，在“读我”页面选择本次提交的作者。</p></div><RouterLink class="secondary small" to="/readme">前往读我</RouterLink></div>
    <p v-if="error" class="error-text" role="alert">{{ error }}</p>
    <form class="author-edit-form" @submit.prevent="save">
      <h3>{{ editing ? '编辑作者' : '新增作者' }}</h3>
      <div class="author-edit-fields"><div class="field"><label for="commit-author-name">作者姓名</label><input id="commit-author-name" v-model="name" maxlength="100" required :disabled="busy" /></div><div class="field"><label for="commit-author-email">GitHub 提交邮箱</label><input id="commit-author-email" v-model="email" type="email" maxlength="254" placeholder="GitHub 关联邮箱或专属 noreply 邮箱" required :disabled="busy" /></div></div>
      <div class="form-actions"><button class="primary" :disabled="busy">{{ editing ? '保存作者' : '添加作者' }}</button><button v-if="editing" type="button" class="secondary" :disabled="busy" @click="reset">取消编辑</button></div>
      <p class="muted author-email-note">可从 GitHub 的 Settings → Emails 复制提交邮箱；邮箱决定提交关联的 GitHub 账号。作者列表对团队成员共享。</p>
    </form>
    <p v-if="loading" class="muted" aria-live="polite">正在加载作者…</p>
    <p v-else-if="!authors.length" class="muted">还没有提交作者，请先添加。</p>
    <ul v-else class="commit-author-list" aria-label="提交作者列表"><li v-for="author in authors" :key="author.id" class="admin-row"><div><strong>{{ author.name }}</strong><small>{{ author.email }}</small></div><button class="secondary small" :aria-label="`编辑作者 ${author.name}`" :disabled="busy" @click="edit(author)">编辑</button><button class="text-button danger-text" :aria-label="`删除作者 ${author.name}`" :disabled="busy" @click="remove(author)">删除</button></li></ul>
  </section>
</template>

<style scoped>
.readme-author-settings{grid-column:1/-1;min-width:0}.readme-author-settings .page-heading{gap:1rem;align-items:start}.readme-author-settings .page-heading p{font-size:.88rem;line-height:1.7}.author-edit-form{padding:1rem;background:var(--surface-soft);border:1px solid var(--border);border-radius:8px;margin-bottom:1.3rem}.author-edit-fields{display:grid;grid-template-columns:1fr 2fr;gap:1rem}.author-email-note{font-size:.8rem;line-height:1.7;margin:.8rem 0 0}.commit-author-list{list-style:none;padding:0;margin:0}.commit-author-list .admin-row{gap:.7rem}.commit-author-list .admin-row>div{min-width:0;overflow-wrap:anywhere}.commit-author-list .admin-row small{line-height:1.5}.author-edit-form .form-actions{justify-content:flex-start}
@media(max-width:760px){.author-edit-fields{grid-template-columns:1fr;gap:0}.readme-author-settings .page-heading{flex-wrap:wrap}.commit-author-list .admin-row{flex-wrap:wrap}.commit-author-list .admin-row>div{flex-basis:100%}}
</style>
