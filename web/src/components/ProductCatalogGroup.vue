<script setup lang="ts">
import { computed } from 'vue'
import type { Product } from '../types'
import { productBaseTitle } from '../utils'
import ProductCatalogItem from './ProductCatalogItem.vue'
import StatusBadge from './StatusBadge.vue'

const props = defineProps<{ product: Product; layout: 'list' | 'grid'; selectable?: boolean; selected?: boolean; selectionDisabled?: boolean; query?: string; expanded?: boolean }>()
const emit = defineEmits<{ toggle: []; expand: [] }>()
const members = computed(() => props.product.variants || [])
const grouped = computed(() => !props.selectable && members.value.length > 1)
const heading = computed(() => grouped.value ? { ...props.product, variant_name: '', attachments: props.product.attachments.length ? props.product.attachments : members.value.find(member => member.attachments.length)?.attachments || [] } : props.product)
const matches = computed(() => {
  const term = props.query?.trim().toLocaleLowerCase()
  return term ? members.value.filter(member => `${member.variant_name} ${member.variant_description}`.toLocaleLowerCase().includes(term)).map(member => member.variant_name || '未填写口味／样式').join('、') : ''
})
const states = computed(() => [...new Set(members.value.map(item => item.development_status))])
</script>

<template>
  <ProductCatalogItem v-if="!grouped" :product="product" :layout="layout" :selectable="selectable" :selected="selected" :selection-disabled="selectionDisabled" @toggle="emit('toggle')" />
  <div v-else class="catalog-group" :class="`is-${layout}`">
    <ProductCatalogItem :product="heading" :layout="layout" :status-summary="states.length > 1 ? `${states.length} 种制作状态` : ''" />
    <div class="catalog-group-footer">
      <button type="button" class="button-link" :aria-label="`${expanded ? '收起' : '展开'}${productBaseTitle(product)}的口味／样式`" :aria-expanded="expanded" @click="emit('expand')">{{ expanded ? '▾' : '▸' }} {{ members.length }} 款口味／样式</button>
      <span v-if="matches" class="muted">匹配：{{ matches }}</span>
    </div>
    <ul v-if="expanded" class="catalog-group-members">
      <li v-for="member in members" :key="member.id">
        <RouterLink :to="`/products/${member.id}`"><img v-if="member.attachments[0]" :src="member.attachments[0].thumbnail_url || member.attachments[0].url" alt="" loading="lazy" /><span>{{ member.variant_name || '未填写口味／样式' }}</span></RouterLink>
        <StatusBadge :label="member.development_status_label" :color="member.development_status_color" />
      </li>
    </ul>
  </div>
</template>

<style scoped>
.catalog-group{min-width:0;border-top:1px solid var(--border)}
.catalog-group :deep(.catalog-item.is-list){border-top:0}
.catalog-group-footer{display:flex;align-items:center;gap:.7rem;flex-wrap:wrap;padding:0 1.1rem .8rem;font-size:.78rem}
.catalog-group-members{list-style:none;margin:0;padding:0 1.1rem .8rem;display:grid;gap:.4rem}
.catalog-group-members li{display:flex;justify-content:space-between;align-items:center;gap:.75rem;min-width:0;border-top:1px solid var(--border);padding:.65rem 0}
.catalog-group-members a{display:flex;align-items:center;gap:.5rem;min-width:0;overflow-wrap:anywhere;font-size:.85rem}
.catalog-group-members img{height:32px;width:32px;object-fit:contain;flex:none;border-radius:4px}
.catalog-group.is-grid{border:1px solid var(--border);border-radius:9px;background:var(--surface);overflow:hidden}
.catalog-group.is-grid :deep(.catalog-item){border:0}
</style>
