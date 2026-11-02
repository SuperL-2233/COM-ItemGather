<script setup lang="ts">
import { computed } from 'vue'
import StatusBadge from './StatusBadge.vue'
import type { RecipeGroup } from '../types'
import { developmentStatusLabel, productTitle } from '../utils'

const props = defineProps<{ group: RecipeGroup; layout: 'list' | 'grid' }>()
const cover = computed(() => props.group.output.attachments[0])
const pending = computed(() => !props.group.recipes.length || props.group.recipes.some(recipe => !recipe.ingredients.length))
</script>

<template>
  <article class="product-card recipe-catalog-item" :class="`is-${layout}`">
    <div class="recipe-identity">
      <div v-if="cover || layout === 'list'" class="recipe-visual" aria-hidden="true">
        <img v-if="cover" :src="cover.thumbnail_url || cover.url" alt="" loading="lazy" />
        <svg v-else viewBox="0 0 24 24"><path d="m12 3 8 4.5v9L12 21l-8-4.5v-9L12 3Z" /><path d="m4 7.5 8 4.5 8-4.5M12 12v9M8 5.25l8 4.5" /></svg>
      </div>
      <div class="recipe-names">
        <h2><RouterLink :to="`/recipes/${group.id}`">{{ productTitle(group.output) }}</RouterLink></h2>
        <p v-if="group.output.chinese_name && group.output.english_name" class="recipe-english">{{ group.output.english_name }}</p>
      </div>
    </div>
    <div class="recipe-type"><span class="recipe-field-label">类型</span><span class="tag">{{ group.output.item_kind === 'tool' ? '道具' : '商品' }}</span></div>
    <div class="recipe-count"><span class="recipe-field-label">配方数量</span><span>{{ group.recipes.length }} 套配方</span><small v-if="pending" class="muted">待补充</small></div>
    <div class="recipe-status">
      <span class="recipe-field-label">制作状态</span>
      <span v-if="group.output.deleted_at" class="recipe-status-badge is-deleted">产物已删除</span><StatusBadge v-else-if="layout === 'list'" class="recipe-status-label" :label="developmentStatusLabel(group.output)" :color="group.output.development_status_color" />
    </div>
  </article>
</template>

<style scoped>
.recipe-catalog-item{display:grid;grid-template-columns:var(--catalog-columns);grid-template-areas:"identity type count status";align-items:center;gap:var(--catalog-gap);padding:1rem 1.1rem;min-width:0;overflow:hidden}
.recipe-catalog-item.is-list{border:0;border-radius:0;border-top:1px solid var(--border)}
.recipe-catalog-item.is-list:last-child{border-radius:0 0 8px 8px}
.recipe-catalog-item.is-list:hover{background:color-mix(in srgb,var(--surface-soft) 50%,var(--surface))}
.recipe-identity{grid-area:identity;display:flex;align-items:center;gap:.8rem;min-width:0}
.recipe-visual{display:grid;place-items:center;flex:none;width:44px;height:44px;border:1px solid var(--border);border-radius:7px;background:var(--surface-soft);overflow:hidden;color:var(--muted)}
.recipe-visual>img{display:block;width:100%;height:100%;object-fit:contain}
.recipe-visual>svg{width:22px;height:22px;fill:none;stroke:currentColor;stroke-width:1.3;stroke-linecap:round;stroke-linejoin:round;opacity:.65}
.recipe-names{min-width:0}
.recipe-catalog-item h2{font-size:.95rem;font-weight:650;line-height:1.5;overflow-wrap:anywhere;margin:0}
.recipe-catalog-item h2 a:hover{color:var(--accent)}
.recipe-english{font-size:.76rem;line-height:1.5;color:var(--muted);overflow-wrap:anywhere;margin:.15rem 0 0}
.recipe-type{grid-area:type;min-width:0}
.recipe-count{grid-area:count;min-width:0;font-size:.82rem;line-height:1.5}
.recipe-count>small{display:block;font-size:.75rem}
.recipe-field-label{position:absolute;width:1px;height:1px;padding:0;overflow:hidden;clip-path:inset(50%);white-space:nowrap}
.recipe-status{grid-area:status;min-width:0}
.recipe-status-badge{--status-color:var(--muted);display:inline-flex;align-items:center;gap:.4rem;padding:.25rem .55rem;border-radius:5px;background:color-mix(in srgb,var(--status-color) 10%,var(--surface));color:var(--status-color);font-size:.75rem;line-height:1.4;overflow-wrap:anywhere;max-width:100%}
.recipe-status-badge::before{content:"";width:6px;height:6px;border-radius:50%;background:currentColor;flex:none}
.recipe-status-badge.is-deleted{--status-color:var(--danger)}
.recipe-catalog-item.is-grid{grid-template-columns:repeat(2,minmax(0,1fr));grid-template-areas:"identity identity" "type count" "status status";gap:.8rem;padding:1.1rem;border-radius:9px;align-items:start}
.is-grid .recipe-identity{flex-direction:column;align-items:stretch;gap:1rem}
.is-grid .recipe-visual{width:calc(100% + 2.2rem);height:auto;aspect-ratio:16/9;margin:-1.1rem -1.1rem 0;border:0;border-bottom:1px solid var(--border);border-radius:8px 8px 0 0}
.is-grid .recipe-visual>img{object-fit:cover}
.is-grid h2{font-size:1.05rem}
.is-grid .recipe-count{justify-self:end;text-align:right}
.is-grid .recipe-count>small{display:inline;margin-left:.5rem}
@media(max-width:600px){
  .recipe-catalog-item.is-list{grid-template-columns:minmax(0,1fr) auto;grid-template-areas:"identity identity" "type status" "count status";gap:.4rem .6rem;padding:.9rem}
  .is-list .recipe-identity{gap:.65rem;margin-bottom:.15rem}
  .is-list .recipe-type,.is-list .recipe-count{margin-left:calc(44px + .65rem)}
  .is-list .recipe-status{align-self:end}
  .is-list .recipe-status-badge,.is-list .recipe-status-label{max-width:7rem}
}
</style>
