<script setup lang="ts">
import type { VariantDraft } from '../types'
import RecordOptionSelect from './RecordOptionSelect.vue'

const drafts = defineModel<VariantDraft[]>({ required: true })
const props = defineProps<{ initialStatus?: string }>()
function add() {
  if (drafts.value.length < 100) drafts.value = [...drafts.value, { variant_name: '', variant_description: '', development_status: props.initialStatus || 'concept' }]
}
</script>

<template>
  <section class="variant-drafts" aria-label="口味／样式">
    <div class="variant-draft-heading"><strong>口味／样式（可选）</strong><button type="button" class="secondary small" :disabled="drafts.length >= 100" @click="add">添加口味／样式</button></div>
    <p class="muted">有多款时在这里一起添加，商品名称、公司和分类只填一次。不添加则创建普通商品。</p>
    <fieldset v-for="(draft, index) in drafts" :key="index" class="variant-draft">
      <legend>第 {{ index + 1 }} 款</legend>
      <label>口味／样式<input v-model="draft.variant_name" :aria-label="`第${index + 1}款口味／样式`" maxlength="160" placeholder="例如：原味、草莓味、蓝色款" required /></label>
      <label>差异说明<textarea v-model="draft.variant_description" :aria-label="`第${index + 1}款差异说明`" rows="2" placeholder="颜色、贴图等差异，可留空"></textarea></label>
      <div class="field"><span class="field-label">制作状态</span><RecordOptionSelect v-model="draft.development_status" kind="statuses" :label="`第${index + 1}款制作状态`" /></div>
      <button type="button" class="text-button danger-text" :aria-label="`移除第${index + 1}款`" @click="drafts = drafts.filter((_, position) => position !== index)">移除这款</button>
    </fieldset>
  </section>
</template>

<style scoped>
.variant-drafts{margin:1rem 0;min-width:0}
.variant-draft-heading{display:flex;align-items:center;justify-content:space-between;gap:.75rem;flex-wrap:wrap}
.variant-drafts>p{font-size:.85rem}
.variant-draft{min-width:0;border:1px solid var(--border);border-radius:8px;padding:.8rem;margin:.75rem 0;display:grid;gap:.6rem}
.variant-draft legend{font-size:.85rem;padding:0 .3rem}
.variant-draft .field{margin:0}
</style>
