<script setup lang="ts">
import { ref } from 'vue'
import type { Attachment } from '../types'
import { fileSize } from '../utils'
defineProps<{ items: Attachment[]; busy?: boolean }>()
const emit = defineEmits<{ upload: [files: File[]]; delete: [id: string] }>()
const input = ref<HTMLInputElement>()
function choose(event: Event) { const target = event.target as HTMLInputElement; const files = Array.from(target.files || []); if (files.length) emit('upload', files); target.value = '' }
</script>
<template>
  <div class="attachments">
    <div v-if="items.length" class="attachment-grid"><figure v-for="item in items" :key="item.id"><a :href="item.url" target="_blank" rel="noopener"><img :src="item.thumbnail_url || item.url" :alt="item.original_name" loading="lazy" /></a><figcaption><span :title="item.original_name">{{ item.original_name }}</span><small>{{ fileSize(item.size) }}</small><button class="text-button danger-text" @click="emit('delete', item.id)">删除</button></figcaption></figure></div>
    <input ref="input" hidden type="file" accept="image/jpeg,image/png,image/webp" multiple @change="choose" />
    <button type="button" class="secondary small" :disabled="busy" @click="input?.click()">{{ busy ? '上传中…' : '+ 添加图片' }}</button>
  </div>
</template>
