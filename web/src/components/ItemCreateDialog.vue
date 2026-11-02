<script setup lang="ts">
import { computed, ref } from 'vue'
import { api } from '../api'
import type { Product, ToolSettings } from '../types'
import { emptyToolSettings, statusLabels } from '../utils'
import CompanySelect from './CompanySelect.vue'
import ToolFields from './ToolFields.vue'
const props = defineProps<{ kind: 'product' | 'tool'; ideaId?: string; initialName?: string; initialDescription?: string }>()
const emit = defineEmits<{ created: [item: Product]; cancel: [] }>()
const noun = computed(() => props.kind === 'tool' ? '道具' : '商品')
const form = ref({ chinese_name: props.initialName || '', english_name: '', company_id: '', brand_name: '', category: '', description: props.initialDescription || '', development_status: 'concept', use_category: '', tool_settings: emptyToolSettings() as ToolSettings, attributes: [] }); const busy = ref(false); const error = ref('')
async function save() { busy.value = true; error.value = ''; try { emit('created', await api<Product>(props.kind === 'tool' ? '/tools' : '/products', { method: 'POST', body: JSON.stringify({ ...form.value, ...(props.ideaId ? { idea_id: props.ideaId } : {}) }) })) } catch (e) { error.value = (e as Error).message } finally { busy.value = false } }
</script>
<template><Teleport to="body"><div class="modal-backdrop" @keydown.esc.stop="!busy && emit('cancel')"><form class="modal" role="dialog" aria-modal="true" :aria-label="`创建${noun}`" @submit.stop.prevent="save"><h2>{{ ideaId ? '由此灵感创建' : '创建' }}{{ noun }}</h2><label :for="`${kind}-create-name`">{{ noun }}中文名（可留空）</label><input :id="`${kind}-create-name`" v-model="form.chinese_name" maxlength="160" autofocus /><label>{{ noun }}英文名（可留空）<input v-model="form.english_name" maxlength="160" /></label><div class="field"><span class="field-label">所属公司（可留空）</span><CompanySelect v-model="form.company_id" /></div><label v-if="kind === 'product'">品牌名（可留空）<input v-model="form.brand_name" maxlength="160" /></label><label>{{ noun }}分类（可留空）<input v-model="form.category" maxlength="100" /></label><label>描述<textarea v-model="form.description" rows="4"></textarea></label><ToolFields v-if="kind === 'tool'" v-model:category="form.use_category" v-model:settings="form.tool_settings" /><label>制作状态<select v-model="form.development_status" aria-label="制作状态"><option v-for="key in ['concept','design','production','completed']" :key="key" :value="key">{{ statusLabels[key] }}</option></select></label><p v-if="error" class="error-text" role="alert">{{ error }}</p><div class="modal-actions"><button type="button" class="secondary" :disabled="busy" @click="emit('cancel')">取消</button><button class="primary" :disabled="busy">{{ busy ? '创建中…' : ideaId ? '创建并关联' : `创建${noun}` }}</button></div></form></div></Teleport></template>
