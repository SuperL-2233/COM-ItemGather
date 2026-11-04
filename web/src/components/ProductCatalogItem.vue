<script setup lang="ts">
import { computed } from 'vue'
import StatusBadge from './StatusBadge.vue'
import type { Product } from '../types'
import { developmentStatusLabel, itemPath, productTitle } from '../utils'

const props = defineProps<{ product: Product; layout: 'list' | 'grid'; selectable?: boolean; selected?: boolean; selectionDisabled?: boolean }>()
const emit = defineEmits<{ toggle: [] }>()
const cover = computed(() => props.product.attachments[0])
const categoryLabel = computed(() => props.product.item_kind === 'tool' ? '使用类别' : '商品分类')
const category = computed(() => props.product.item_kind === 'tool' ? props.product.use_category : props.product.category)
</script>

<template>
  <article class="product-card catalog-item" :class="[`is-${layout}`, { 'is-selectable': selectable, 'is-selected': selected, 'has-cover': !!cover }]" @click="selectable && !selectionDisabled && emit('toggle')">
    <div class="catalog-identity">
      <label v-if="selectable" class="catalog-select" @click.stop><input type="checkbox" :checked="selected" :disabled="selectionDisabled" :aria-label="`选择商品：${productTitle(product)}`" @change="emit('toggle')" /></label>
      <div v-if="cover || layout === 'list'" class="catalog-visual" aria-hidden="true">
        <img v-if="cover" :src="cover.thumbnail_url || cover.url" alt="" loading="lazy" />
        <svg v-else viewBox="0 0 24 24"><path d="m12 3 8 4.5v9L12 21l-8-4.5v-9L12 3Z" /><path d="m4 7.5 8 4.5 8-4.5M12 12v9M8 5.25l8 4.5" /></svg>
      </div>
      <div class="catalog-names">
        <h2><span v-if="selectable">{{ productTitle(product) }}</span><RouterLink v-else :to="itemPath(product)">{{ productTitle(product) }}</RouterLink></h2>
        <p v-if="product.chinese_name && product.english_name" class="catalog-english">{{ product.english_name }}</p>
      </div>
    </div>
    <div class="catalog-company">
      <span class="catalog-field-label">所属公司</span>
      <RouterLink v-if="!selectable && product.company_id && !product.company_deleted" :to="`/companies/${product.company_id}`">{{ product.company }}</RouterLink>
      <span v-else :class="{ 'catalog-unset': !product.company }">{{ product.company || '未设置公司' }}</span>
    </div>
    <div class="catalog-category">
      <span class="catalog-field-label">{{ categoryLabel }}</span>
      <span v-if="category" class="catalog-category-tag">{{ category }}</span>
      <span v-else class="catalog-unset">{{ product.item_kind === 'tool' ? '未设置类别' : '未分类' }}</span>
    </div>
    <div class="catalog-status">
      <span class="catalog-field-label">制作状态</span>
      <StatusBadge class="catalog-status-badge" :label="developmentStatusLabel(product)" :color="product.development_status_color" />
    </div>
  </article>
</template>

<style scoped>
.catalog-item{position:relative;display:grid;grid-template-columns:var(--catalog-columns);grid-template-areas:"identity company category status";align-items:center;gap:var(--catalog-gap);padding:1rem 1.1rem;overflow:visible;min-width:0}
.catalog-item.is-list{border:0;border-radius:0;border-top:1px solid var(--border)}
.catalog-item.is-list:last-child{border-radius:0 0 8px 8px}
.catalog-item.is-list:hover{background:color-mix(in srgb,var(--surface-soft) 50%,var(--surface))}
.catalog-identity{grid-area:identity;display:flex;align-items:center;gap:.8rem;min-width:0}
.catalog-visual{display:grid;place-items:center;flex:none;width:44px;height:44px;border:1px solid var(--border);border-radius:7px;background:var(--surface-soft);overflow:hidden;color:var(--muted)}
.catalog-visual>img{display:block;width:100%;height:100%;object-fit:contain}
.catalog-visual>svg{width:22px;height:22px;fill:none;stroke:currentColor;stroke-width:1.3;stroke-linecap:round;stroke-linejoin:round;opacity:.65}
.catalog-names{min-width:0}
.catalog-item h2{font-size:.95rem;font-weight:650;line-height:1.5;overflow-wrap:anywhere;margin:0}
.catalog-item h2 a:hover{color:var(--accent)}
.catalog-english{font-family:inherit;font-size:.76rem;line-height:1.5;color:var(--muted);overflow-wrap:anywhere;margin:.15rem 0 0}
.catalog-company{grid-area:company;font-size:.82rem;line-height:1.5;overflow-wrap:anywhere;min-width:0}
.catalog-company>a{color:var(--text)}
.catalog-company>a:hover{color:var(--accent)}
.catalog-category{grid-area:category;min-width:0;font-size:.78rem;overflow-wrap:anywhere}
.catalog-category-tag{display:inline-block;max-width:100%;padding:.2rem .5rem;border-radius:4px;background:var(--surface-soft);color:var(--muted);line-height:1.4}
.catalog-unset{font-size:.78rem;color:var(--muted)}
.catalog-field-label{position:absolute;width:1px;height:1px;padding:0;overflow:hidden;clip-path:inset(50%);white-space:nowrap}
.catalog-status{grid-area:status;min-width:0}
.catalog-item.is-grid{grid-template-columns:repeat(2,minmax(0,1fr));grid-template-areas:"identity identity" "company company" "category status";gap:.8rem;padding:1.1rem;border-radius:9px;align-items:start}
.is-grid .catalog-identity{display:flex;flex-direction:column;align-items:stretch;gap:1rem}
.is-grid .catalog-visual{width:calc(100% + 2.2rem);height:auto;aspect-ratio:16/10;margin:-1.1rem -1.1rem 0;border:0;border-bottom:1px solid var(--border);border-radius:8px 8px 0 0}
.is-grid .catalog-names{min-height:3rem}
.is-grid h2{font-size:1.05rem}
.is-grid .catalog-company{padding-bottom:.8rem;border-bottom:1px solid var(--border)}
.is-grid .catalog-category,.is-grid .catalog-status{align-self:center}
.is-grid .catalog-status{justify-self:end}
.catalog-item.is-selectable{cursor:pointer}
.catalog-item.is-selected,.catalog-item.is-selected:hover{background:color-mix(in srgb,var(--accent) 7%,var(--surface));box-shadow:inset 3px 0 var(--accent)}
.catalog-select{flex:none;display:grid;place-items:center;width:1.5rem;min-height:2.65rem;cursor:pointer}
.catalog-select input{width:1.05rem;height:1.05rem;padding:0;margin:0;accent-color:var(--accent);cursor:pointer}
.is-list .catalog-select{width:44px;min-height:44px}
.is-list.is-selectable .catalog-visual{display:none}
.is-grid .catalog-select{position:absolute;top:.7rem;left:.7rem;z-index:1;width:1.85rem;height:1.85rem;min-height:0;background:var(--surface);border:1px solid var(--border);border-radius:5px}
.is-grid.is-selectable:not(.has-cover) .catalog-names{padding-left:2rem}
@media(max-width:600px){
  .catalog-item.is-list{grid-template-columns:minmax(0,1fr) auto;grid-template-areas:"identity identity" "company status" "category status";gap:.4rem .6rem;padding:.9rem}
  .is-list .catalog-identity{gap:.65rem;margin-bottom:.15rem}
  .is-list .catalog-company,.is-list .catalog-category{margin-left:calc(44px + .65rem)}
  .is-list .catalog-category{line-height:1.4}
  .is-list .catalog-status{align-self:end}
  .is-list .catalog-status-badge{max-width:7rem}
}
</style>
