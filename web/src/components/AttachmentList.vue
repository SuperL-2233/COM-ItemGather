<script setup lang="ts">
import { ref } from 'vue'
import type { Attachment } from '../types'
import { fileSize } from '../utils'
const props = defineProps<{ items: Attachment[]; busy?: boolean }>()
const emit = defineEmits<{ upload: [files: File[]]; delete: [id: string] }>()
const input = ref<HTMLInputElement>()
const dragging = ref(false)
const dragDepth = ref(0)
const error = ref('')
function upload(incoming: File[] | FileList) {
  const files = Array.from(incoming)
  if (!files.length || props.busy) return
  const images = files.filter(file => ['image/jpeg','image/png','image/webp'].includes(file.type))
  error.value = images.length === files.length ? '' : '仅支持 JPEG、PNG 和 WebP 图片，其他文件未添加'
  if (images.length) emit('upload', images)
}
function choose(event: Event) {
  const target = event.target as HTMLInputElement
  upload(target.files || [])
  target.value = ''
}
function dragEnter(event: DragEvent) {
  if (props.busy || !event.dataTransfer?.types.includes('Files')) return
  dragDepth.value++
  dragging.value = true
}
function dragLeave() {
  if (!dragging.value) return
  dragDepth.value = Math.max(0, dragDepth.value - 1)
  if (!dragDepth.value) dragging.value = false
}
function drop(event: DragEvent) {
  dragging.value = false
  dragDepth.value = 0
  if (event.dataTransfer?.files.length) upload(event.dataTransfer.files)
}
</script>
<template>
  <div class="attachments">
    <div v-if="items.length" class="attachment-grid"><figure v-for="item in items" :key="item.id"><a :href="item.url" target="_blank" rel="noopener"><img :src="item.thumbnail_url || item.url" :alt="item.original_name" loading="lazy" /></a><figcaption><span :title="item.original_name">{{ item.original_name }}</span><small>{{ fileSize(item.size) }}</small><button class="text-button danger-text" @click="emit('delete', item.id)">删除</button></figcaption></figure></div>
    <div class="attachment-dropzone" :class="{ dragging, 'is-disabled': busy }" role="region" aria-label="图片附件上传区域" :aria-busy="busy" @dragenter.prevent="dragEnter" @dragover.prevent @dragleave.prevent="dragLeave" @drop.prevent="drop">
      <div><strong>{{ busy ? '正在上传图片…' : '拖拽图片到这里' }}</strong><small>JPEG、PNG、WebP，可多选</small></div>
      <input ref="input" hidden type="file" accept="image/jpeg,image/png,image/webp" multiple @change="choose" />
      <button type="button" class="secondary small" :disabled="busy" @click="input?.click()">{{ busy ? '上传中…' : '+ 添加图片' }}</button>
    </div>
    <p v-if="error" class="error-text attachment-error" role="alert">{{ error }}</p>
  </div>
</template>

<style scoped>
.attachment-dropzone{display:flex;align-items:center;justify-content:space-between;gap:1rem;flex-wrap:wrap;margin-top:1rem;padding:1rem;border:1px dashed var(--border);border-radius:8px;background:color-mix(in srgb,var(--surface-soft) 45%,var(--surface));transition:border-color .15s,background .15s}
.attachment-dropzone>div{flex:1;min-width:150px}
.attachment-dropzone strong{display:block;font-size:.85rem;font-weight:500}
.attachment-dropzone small{display:block;margin-top:.25rem;color:var(--muted);font-size:.75rem}
.attachment-dropzone.dragging{border-color:var(--accent);background:var(--surface-soft);box-shadow:inset 0 0 0 1px var(--accent)}
.attachment-dropzone.is-disabled{opacity:.65}
.attachment-error{margin:.5rem 0 0;font-size:.82rem}
.attachments:not(:has(.attachment-grid)) .attachment-dropzone{margin-top:0}
</style>
