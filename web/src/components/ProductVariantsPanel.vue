<script setup lang="ts">
import { ref } from 'vue'
import { api } from '../api'
import type { Product, VariantDraft } from '../types'
import RecordOptionSelect from './RecordOptionSelect.vue'
import ProductVariantRow from './ProductVariantRow.vue'

const props = defineProps<{ product: Product; locked: boolean }>()
const emit = defineEmits<{ changed: [] }>()
const adding = ref(false)
const draft = ref<VariantDraft>({ variant_name: '', variant_description: '', development_status: 'concept' })
const busy = ref(false)
const error = ref('')
async function add() {
  if (busy.value || props.locked) return
  busy.value = true; error.value = ''
  try {
    await api(`/products/${props.product.id}/variants`, { method: 'POST', body: JSON.stringify({ ...draft.value, version: props.product.version }) })
    adding.value = false
    draft.value = { variant_name: '', variant_description: '', development_status: 'concept' }
    emit('changed')
  } catch (e) { error.value = (e as Error).message }
  finally { busy.value = false }
}
</script>

<template>
  <section class="panel variants-panel" aria-labelledby="variants-heading">
    <div class="variants-heading"><h2 id="variants-heading">商品口味／样式</h2><button type="button" class="secondary small" :disabled="locked || busy || (product.variants?.length || 0) >= 100" @click="adding = !adding">添加口味／样式</button></div>
    <p v-if="locked" class="muted">请先保存商品资料。</p>
    <div v-if="product.variants?.length" class="variant-list">
      <ProductVariantRow v-for="variant in product.variants" :key="variant.id" :variant="variant" :parent-id="product.id" :locked="locked || busy" @changed="emit('changed')" />
    </div>
    <p v-else class="muted">尚未添加口味／样式，可留空。</p>
    <form v-if="adding" class="variant-add" @submit.prevent="add">
      <h3>添加口味／样式</h3>
      <label>口味／样式名称<input v-model="draft.variant_name" maxlength="160" placeholder="例如：玫瑰荔枝味、草莓味、巧克力味" required :disabled="busy" /></label>
      <label>差异说明<textarea v-model="draft.variant_description" rows="3" placeholder="颜色、贴图等差异，可留空" :disabled="busy"></textarea></label>
      <div class="field"><span class="field-label">制作状态</span><RecordOptionSelect v-model="draft.development_status" kind="statuses" label="口味／样式制作状态" /></div>
      <div class="inline-controls"><button type="button" class="secondary" :disabled="busy" @click="adding = false">取消</button><button class="primary" :disabled="busy || locked">{{ busy ? '添加中…' : '添加' }}</button></div>
    </form>
    <p v-if="error" class="error-text" role="alert">{{ error }}</p>
  </section>
</template>

<style scoped>
.variants-panel{min-width:0}
.variants-heading{display:flex;justify-content:space-between;align-items:center;gap:.75rem;flex-wrap:wrap}
.variants-heading h2{margin:0}
.variant-list{display:grid;gap:.75rem;margin-top:1rem}
.variant-add{border-top:1px solid var(--border);margin-top:1rem;padding-top:1rem;display:grid;gap:.7rem;min-width:0}
.variant-add h3{margin:0;font-size:1rem}
.variant-add label{display:block;font-size:.85rem;font-weight:600}
.variant-add label>input,.variant-add label>textarea{display:block;width:100%;margin-top:.4rem;font-weight:400}
</style>
