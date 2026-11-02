<script setup lang="ts">
import { ref } from 'vue'
import { api } from '../api'
import type { Attachment } from '../types'
const props = defineProps<{ companyId: string }>()
const items = defineModel<Attachment[]>({ required: true })
const busy = ref(false); const error = ref(''); const input = ref<HTMLInputElement>()
async function upload(file?: File) { if (!file || busy.value) return; error.value = ''; if (file.size > 3*1024*1024) { error.value = 'Logo 不得超过 3 MiB'; return }; if (!['image/jpeg','image/png','image/webp'].includes(file.type)) { error.value = '请选择 PNG、JPG 或 WebP 图片'; return }; busy.value = true; try { const data = new FormData(); data.append('company_id',props.companyId); data.append('replace_id',items.value[0]?.id || ''); data.append('file',file); data.append('client_id',crypto.randomUUID()); items.value = [await api<Attachment>('/attachments',{ method: 'POST', body: data })] } catch (e) { error.value = (e as Error).message } finally { busy.value = false } }
function select(event: Event) { const field = event.target as HTMLInputElement; void upload(field.files?.[0]); field.value = '' }
async function remove() { if (!items.value[0] || !confirm('移除公司 Logo？')) return; busy.value = true; error.value = ''; try { await api(`/attachments/${items.value[0].id}`,{ method: 'DELETE' }); items.value = [] } catch (e) { error.value = (e as Error).message } finally { busy.value = false } }
</script>
<template><div class="logo-dropzone" tabindex="0" aria-label="公司 Logo 上传区域" @dragover.prevent @drop.prevent="upload($event.dataTransfer?.files[0])" @paste="upload($event.clipboardData?.files[0])"><a v-if="items[0]" :href="items[0].url" target="_blank" rel="noopener"><img :src="items[0].url" alt="公司 Logo" class="company-logo-preview" /></a><div v-else class="logo-placeholder">Logo 待补充</div><p class="muted">PNG、JPG 或 WebP，单张不超过 3 MiB。支持拖入或粘贴图片。</p><input ref="input" hidden type="file" accept="image/jpeg,image/png,image/webp" @change="select" /><div class="inline-controls"><button type="button" class="secondary small" :disabled="busy" @click="input?.click()">{{ busy ? '上传中…' : items[0] ? '更换 Logo' : '上传 Logo' }}</button><button v-if="items[0]" type="button" class="text-button danger-text" :disabled="busy" @click="remove">移除 Logo</button></div><p v-if="error" class="error-text" role="alert">{{ error }}</p></div></template>
