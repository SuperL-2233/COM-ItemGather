<script setup lang="ts">
import { computed, ref } from 'vue'
import { api } from '../api'
import type { Ingredient, Product, VariantDraft } from '../types'
import { productBaseTitle, productTitle } from '../utils'
import RecordOptionSelect from './RecordOptionSelect.vue'
import ItemPicker from './ItemPicker.vue'
import StatusBadge from './StatusBadge.vue'

const props = defineProps<{ product: Product; locked: boolean }>()
const emit = defineEmits<{ changed: [] }>()
const members = computed(() => props.product.variants || [props.product])
const adding = ref(false)
const draft = ref<VariantDraft>({ variant_name: '', variant_description: '', development_status: 'concept' })
const picking = ref(false)
const target = ref<Product>()
const moveName = ref('')
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
function selectTarget(ingredient: Ingredient) {
  picking.value = false
  if (!ingredient.item || ingredient.item.variant_group_id === props.product.variant_group_id || ingredient.item.id === props.product.id) {
    error.value = '请选择另一个商品分组'; return
  }
  target.value = ingredient.item
  moveName.value = props.product.variant_name
  error.value = ''
}
async function move(detach = false) {
  if (busy.value || props.locked || (!detach && !target.value)) return
  if (detach && !confirm('将当前款移出分组，成为独立商品？当前资料、图片和关联会保留。')) return
  busy.value = true; error.value = ''
  try {
    await api(`/products/${props.product.id}/variant-group`, { method: 'POST', body: JSON.stringify({ version: props.product.version, target_id: detach ? '' : target.value!.id, target_version: detach ? 0 : target.value!.version, variant_name: detach ? props.product.variant_name : moveName.value }) })
    target.value = undefined
    emit('changed')
  } catch (e) { error.value = (e as Error).message }
  finally { busy.value = false }
}
</script>

<template>
  <section class="panel variants-panel" aria-labelledby="variants-heading">
    <div class="variants-heading"><h2 id="variants-heading">口味／样式</h2><button type="button" class="secondary small" :disabled="locked || busy || members.length >= 100" @click="adding = !adding; target = undefined">添加新款</button></div>
    <p class="muted">共 {{ members.length }} 款。每款保留独立图片、制作状态和配方关联。</p>
    <p v-if="locked" class="muted">请先保存当前修改，再添加或调整归组。</p>
    <ul class="variant-members">
      <li v-for="member in members" :key="member.id" :class="{ current: member.id === product.id }">
        <RouterLink :to="`/products/${member.id}`" :aria-current="member.id === product.id ? 'page' : undefined"><img v-if="member.attachments[0]" :src="member.attachments[0].thumbnail_url || member.attachments[0].url" alt="" loading="lazy" /><span>{{ member.variant_name || '未填写口味／样式' }}<small v-if="member.id === product.id">当前款</small><small v-if="member.variant_description">{{ member.variant_description }}</small></span></RouterLink>
        <StatusBadge :label="member.development_status_label" :color="member.development_status_color" />
      </li>
    </ul>
    <form v-if="adding" class="variant-add" @submit.prevent="add">
      <h3>添加新款</h3>
      <label>新款口味／样式<input v-model="draft.variant_name" maxlength="160" placeholder="例如：草莓味、蓝色款" required :disabled="busy" /></label>
      <label>新款差异说明<textarea v-model="draft.variant_description" rows="3" placeholder="颜色、贴图等差异，可留空" :disabled="busy"></textarea></label>
      <div class="field"><span class="field-label">新款制作状态</span><RecordOptionSelect v-model="draft.development_status" kind="statuses" label="新款制作状态" /></div>
      <div class="inline-controls"><button type="button" class="secondary" :disabled="busy" @click="adding = false">取消添加</button><button class="primary" :disabled="busy || locked">{{ busy ? '添加中…' : '保存新款' }}</button></div>
    </form>
    <details class="variant-organize">
      <summary>整理已有商品</summary>
      <p class="muted">将当前款归入另一个商品，或移出当前分组。原物品 ID、图片、配方与灵感关联始终保留。</p>
      <div class="inline-controls"><button type="button" class="secondary small" :disabled="locked || busy" @click="picking = true; adding = false">归入已有商品</button><button v-if="members.length > 1" type="button" class="secondary small" :disabled="locked || busy" @click="move(true)">将当前款移出分组</button></div>
      <form v-if="target" class="variant-add" @submit.prevent="move()">
        <p>把「{{ productTitle(product) }}」归入「{{ productBaseTitle(target) }}」。</p>
        <p class="muted">当前款的名称、公司、分类和通用描述将改用目标商品资料；当前款的差异说明、图片、状态和关联保留。原资料可在修改历史查看。</p>
        <label>当前款归组后的口味／样式<input v-model="moveName" maxlength="160" required :disabled="busy" /></label>
        <div class="inline-controls"><button type="button" class="secondary" :disabled="busy" @click="target = undefined">取消归组</button><button class="primary" :disabled="busy || locked">确认归组</button></div>
      </form>
    </details>
    <p v-if="error" class="error-text" role="alert">{{ error }}</p>
  </section>
  <ItemPicker v-if="picking" title="选择要归入的商品" products-only @select="selectTarget" @cancel="picking = false" />
</template>

<style scoped>
.variants-panel{min-width:0}
.variants-heading{display:flex;justify-content:space-between;align-items:center;gap:.75rem;flex-wrap:wrap}
.variants-heading h2{margin:0}
.variants-panel>p,.variant-organize>p{font-size:.85rem}
.variant-members{list-style:none;padding:0;margin:1rem 0;display:grid;gap:.4rem}
.variant-members li{display:flex;align-items:center;justify-content:space-between;gap:.75rem;padding:.7rem;border:1px solid var(--border);border-radius:7px;min-width:0}
.variant-members li.current{background:var(--surface-soft)}
.variant-members a{display:flex;align-items:center;gap:.7rem;min-width:0;overflow-wrap:anywhere;font-size:.9rem}
.variant-members img{height:44px;width:44px;flex:none;object-fit:contain;border-radius:5px}
.variant-members small{display:block;white-space:pre-wrap;overflow-wrap:anywhere;font-size:.75rem;color:var(--muted);margin-top:.25rem}
.variant-members :deep(.status-badge){flex:none}
.variant-add{border-top:1px solid var(--border);margin-top:1rem;padding-top:1rem;display:grid;gap:.7rem;min-width:0}
.variant-add h3,.variant-add p{margin:0}
.variant-organize{border-top:1px solid var(--border);padding-top:.8rem;margin-top:1rem}
.variant-organize summary{cursor:pointer;font-size:.85rem}
@media(max-width:420px){.variant-members li{flex-wrap:wrap}.variant-members a{flex:1 1 100%}}
</style>
