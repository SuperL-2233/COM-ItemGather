<script setup lang="ts">
import { computed, ref } from 'vue'
import { api } from '../api'
import type { Attribute, ItemKind, Product, ToolSettings, VariantDraft } from '../types'
import { emptyToolSettings, itemCollection, itemLabel } from '../utils'
import CompanySelect from './CompanySelect.vue'
import ToolFields from './ToolFields.vue'
import RecordOptionSelect from './RecordOptionSelect.vue'
import VariantDraftEditor from './VariantDraftEditor.vue'
import AttributeEditor from './AttributeEditor.vue'
const props = defineProps<{ kind: ItemKind; ideaId?: string; initialName?: string; initialDescription?: string }>()
const emit = defineEmits<{ created: [item: Product]; cancel: [] }>()
const noun = computed(() => itemLabel(props.kind))
const variants = ref<VariantDraft[]>([])
const form = ref({ chinese_name: props.initialName || '', english_name: '', company_id: '', category: '', description: props.initialDescription || '', development_status: '', use_category: '', tool_settings: emptyToolSettings() as ToolSettings, attributes: [] as Attribute[] }); const busy = ref(false); const error = ref('')
async function save() { if (busy.value) return; busy.value = true; error.value = ''; try { emit('created', await api<Product>(`/${itemCollection(props.kind)}`, { method: 'POST', body: JSON.stringify({ ...form.value, ...(props.kind === 'product' ? { variants: variants.value } : {}), ...(props.ideaId ? { idea_id: props.ideaId } : {}) }) })) } catch (e) { error.value = (e as Error).message } finally { busy.value = false } }
</script>
<template><Teleport to="body"><div class="modal-backdrop" @keydown.esc.stop="!busy && emit('cancel')"><form class="modal" role="dialog" aria-modal="true" :aria-label="`创建${noun}`" @submit.stop.prevent="save"><h2>{{ ideaId ? '由此灵感创建' : '创建' }}{{ noun }}</h2><label :for="`${kind}-create-name`">{{ noun }}中文名（可留空）</label><input :id="`${kind}-create-name`" v-model="form.chinese_name" maxlength="160" autofocus /><label>{{ noun }}英文名（可留空）<input v-model="form.english_name" maxlength="160" /></label><div v-if="kind !== 'other'" class="field"><span class="field-label">所属公司（可留空）</span><CompanySelect v-model="form.company_id" /></div><div class="field"><span class="field-label">{{ noun }}分类（可留空）</span><RecordOptionSelect v-model="form.category" :kind="kind === 'other' ? 'other-categories' : 'categories'" :label="`${noun}分类`" /></div><label>描述<textarea v-model="form.description" rows="4"></textarea></label><ToolFields v-if="kind === 'tool'" v-model:category="form.use_category" v-model:settings="form.tool_settings" /><div class="field"><span class="field-label">制作状态</span><RecordOptionSelect v-model="form.development_status" kind="statuses" label="制作状态" /></div><VariantDraftEditor v-if="kind === 'product'" v-model="variants" :initial-status="form.development_status" /><div v-if="kind === 'other'" class="field"><span class="field-label">动态属性</span><AttributeEditor v-model="form.attributes" /></div><p v-if="error" class="error-text" role="alert">{{ error }}</p><div class="modal-actions"><button type="button" class="secondary" :disabled="busy" @click="emit('cancel')">取消</button><button class="primary" :disabled="busy">{{ busy ? '创建中…' : ideaId ? '创建并关联' : `创建${noun}` }}</button></div></form></div></Teleport></template>
