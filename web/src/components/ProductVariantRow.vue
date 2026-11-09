<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { onBeforeRouteLeave } from 'vue-router'
import { api, ApiError } from '../api'
import type { Attachment, Product } from '../types'
import RecordOptionSelect from './RecordOptionSelect.vue'
import StatusBadge from './StatusBadge.vue'
import AttributeEditor from './AttributeEditor.vue'
import AttachmentList from './AttachmentList.vue'
import ConflictDialog from './ConflictDialog.vue'
import RelatedRecipes from './RelatedRecipes.vue'
import LinkManager from './LinkManager.vue'
import RevisionPanel from './RevisionPanel.vue'

const props = defineProps<{ variant: Product; parentId: string; locked: boolean }>()
const emit = defineEmits<{ changed: [] }>()
const editing = ref(false)
const more = ref(false)
const busy = ref(false)
const error = ref('')
const notice = ref(false)
const fields = (item: Product) => ({ variant_name: item.variant_name, variant_description: item.variant_description, development_status: item.development_status, attributes: item.attributes.map(({ key, value, certainty }) => ({ key, value, certainty })) })
const draft = ref(fields(props.variant))
const version = ref(props.variant.version)
const saved = ref(JSON.stringify(draft.value))
const images = ref<Attachment[]>(props.variant.attachments)
const dirty = computed(() => editing.value && JSON.stringify(draft.value) !== saved.value)
const conflict = ref<{ current: Product; submitted: Record<string, unknown> }>()
function apply(item: Product) { draft.value = fields(item); saved.value = JSON.stringify(draft.value); version.value = item.version; notice.value = false }
watch(() => props.variant, item => { images.value = item.attachments; if (!dirty.value) apply(item); else if (version.value !== item.version) notice.value = true })
function cancel() { editing.value = false; apply(props.variant); error.value = '' }
async function save() {
  if (busy.value || props.locked) return
  busy.value = true; error.value = ''
  try {
    const item = await api<Product>(`/products/${props.parentId}/variants/${props.variant.id}`, { method: 'PATCH', body: JSON.stringify({ ...draft.value, version: version.value }) })
    apply(item); editing.value = false; emit('changed')
  } catch (e) {
    if (e instanceof ApiError && e.status === 409 && e.details?.current) conflict.value = { current: e.details.current, submitted: draft.value }
    else error.value = (e as Error).message
  } finally { busy.value = false }
}
async function remove() {
  if (busy.value || props.locked || !confirm(`删除“${props.variant.variant_name || '未命名口味／样式'}”？图片与配方关联会保留在回收站。`)) return
  busy.value = true; error.value = ''
  try { await api(`/products/${props.parentId}/variants/${props.variant.id}`, { method: 'DELETE', body: JSON.stringify({ version: props.variant.version }) }); emit('changed') }
  catch (e) { error.value = (e as Error).message }
  finally { busy.value = false }
}
async function upload(files: File[]) {
  if (busy.value) return
  busy.value = true; error.value = ''
  try {
    for (const file of files) {
      const data = new FormData(); data.append('product_id', props.variant.id); data.append('file', file); data.append('client_id', crypto.randomUUID())
      images.value = [...images.value, await api<Attachment>('/attachments', { method: 'POST', body: data })]
    }
    emit('changed')
  } catch (e) { error.value = (e as Error).message }
  finally { busy.value = false }
}
async function deleteImage(id: string) {
  if (busy.value || !confirm('删除这张图片？')) return
  busy.value = true; error.value = ''
  try { await api(`/attachments/${id}`, { method: 'DELETE' }); images.value = images.value.filter(item => item.id !== id); emit('changed') }
  catch (e) { error.value = (e as Error).message }
  finally { busy.value = false }
}
function useServer() { if (conflict.value) apply(conflict.value.current); conflict.value = undefined; emit('changed') }
async function mergeLatest() { if (!conflict.value) return; version.value = conflict.value.current.version; conflict.value = undefined; await save() }
function beforeUnload(event: BeforeUnloadEvent) { if (dirty.value) { event.preventDefault(); event.returnValue = '' } }
onMounted(() => window.addEventListener('beforeunload', beforeUnload))
onUnmounted(() => window.removeEventListener('beforeunload', beforeUnload))
onBeforeRouteLeave(() => !dirty.value || confirm('有未保存的口味／样式修改，确定离开？'))
</script>

<template>
  <article :id="`variant-${variant.id}`" class="product-variant" :aria-label="variant.variant_name || '未命名口味／样式'">
    <div class="variant-heading"><h3>{{ variant.variant_name || '未命名口味／样式' }}</h3><StatusBadge :label="variant.development_status_label" :color="variant.development_status_color" /><div class="variant-actions"><button type="button" class="text-button" :disabled="busy || locked" @click="editing ? cancel() : editing = true">{{ editing ? '取消编辑' : '编辑' }}</button><button type="button" class="text-button danger-text" :disabled="busy || locked || dirty" @click="remove">删除</button></div></div>
    <template v-if="!editing"><p v-if="variant.variant_description" class="variant-description">{{ variant.variant_description }}</p><div v-if="images.length" class="variant-images"><a v-for="attachment in images" :key="attachment.id" :href="attachment.url" target="_blank" rel="noopener"><img :src="attachment.thumbnail_url || attachment.url" :alt="attachment.original_name" loading="lazy" /></a></div></template>
    <template v-else>
      <form class="variant-form" @submit.prevent="save">
        <label>口味／样式名称<input v-model="draft.variant_name" maxlength="160" required :disabled="busy" /></label>
        <label>差异说明<textarea v-model="draft.variant_description" rows="3" :disabled="busy"></textarea></label>
        <div class="field"><span class="field-label">制作状态</span><RecordOptionSelect v-model="draft.development_status" kind="statuses" :label="`${variant.variant_name || '口味／样式'}制作状态`" :fallback="variant.development_status_label" /></div>
        <div class="field"><span class="field-label">动态属性</span><AttributeEditor v-model="draft.attributes" /></div>
        <p v-if="notice" class="ai-status">这个口味／样式已有其他修改，保存时会显示差异。</p>
        <button class="primary" :disabled="busy || locked || !dirty">{{ busy ? '保存中…' : '保存口味／样式' }}</button>
      </form>
      <section class="variant-attachments"><h4>图片</h4><AttachmentList :items="images" :busy="busy" @upload="upload" @delete="deleteImage" /></section>
    </template>
    <details class="variant-related" @toggle="more = ($event.target as HTMLDetailsElement).open"><summary>关联与修改历史</summary><template v-if="more"><RelatedRecipes :item-id="variant.id" :item-category="variant.category" item-kind="product" /><LinkManager owner-type="product" :owner-id="variant.id" :links="variant.links || []" @changed="emit('changed')" /><RevisionPanel kind="product" :id="variant.id" :version="variant.version" @restored="emit('changed')" /></template></details>
    <p v-if="error" class="error-text" role="alert">{{ error }}</p>
    <ConflictDialog v-if="conflict" :current="conflict.current" :submitted="conflict.submitted" @server="useServer" @merge="mergeLatest" @cancel="conflict = undefined" />
  </article>
</template>

<style scoped>
.product-variant{border:1px solid var(--border);border-radius:8px;padding:1rem;min-width:0;scroll-margin-top:8rem}
.variant-heading{display:flex;align-items:center;gap:.65rem;flex-wrap:wrap}
.variant-heading h3{font-size:1rem;margin:0;overflow-wrap:anywhere}
.variant-actions{display:flex;gap:.6rem;margin-left:auto}
.variant-description{white-space:pre-wrap;overflow-wrap:anywhere;font-size:.85rem;margin:.75rem 0}
.variant-form{display:grid;gap:.75rem;margin:1rem 0}
.variant-form .primary{justify-self:start}
.variant-images{display:flex;gap:.5rem;flex-wrap:wrap;margin:.75rem 0}
.variant-images img{width:72px;height:72px;object-fit:contain;border:1px solid var(--border);border-radius:5px}
.variant-attachments{margin-top:1rem}.variant-attachments h4{margin:0 0 .5rem;font-size:.9rem}
.variant-related{margin-top:.75rem;font-size:.8rem}.variant-related summary{cursor:pointer;color:var(--muted)}
.variant-form label{display:block;font-size:.85rem;font-weight:600}
.variant-form label>input,.variant-form label>textarea{display:block;width:100%;margin-top:.4rem;font-weight:400}
</style>
