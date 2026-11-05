<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref } from 'vue'

defineProps<{ count: number; disabled: boolean; hasProducts: boolean; allDisplayedSelected: boolean }>()
const emit = defineEmits<{ edit: []; cancel: []; toggleDisplayed: [] }>()
const actionsOpen = ref(false)
const actions = ref<HTMLElement>()
const more = ref<HTMLButtonElement>()

function closeActions(restoreFocus = false) {
  if (!actionsOpen.value) return
  actionsOpen.value = false
  if (restoreFocus) void nextTick(() => more.value?.focus())
}
function toggleDisplayed() { closeActions(true); emit('toggleDisplayed') }
function outside(event: MouseEvent) {
  if (actionsOpen.value && actions.value && !event.composedPath().includes(actions.value)) closeActions()
}
function focusOut(event: FocusEvent) {
  if (event.relatedTarget && !actions.value?.contains(event.relatedTarget as Node)) closeActions()
}
function keydown(event: KeyboardEvent) {
  if (event.key === 'Escape' && actionsOpen.value) {
    event.preventDefault()
    event.stopPropagation()
    closeActions(true)
  }
}
onMounted(() => document.addEventListener('click', outside))
onUnmounted(() => document.removeEventListener('click', outside))
</script>

<template>
  <div class="product-selection-bar" role="region" aria-label="商品多选操作">
    <p role="status">已选 <strong>{{ count }}</strong> 件<span class="selection-count-noun">商品</span></p>
    <button type="button" class="primary small edit-selected" :disabled="!count || disabled" @click="closeActions(); emit('edit')">批量修改</button>
    <div ref="actions" class="selection-more" @keydown="keydown" @focusout="focusOut">
      <button ref="more" type="button" class="secondary selection-more-trigger" aria-label="更多多选操作" aria-controls="product-selection-actions" :aria-expanded="actionsOpen" @click="actionsOpen = !actionsOpen">
        <svg viewBox="0 0 20 20" aria-hidden="true"><circle cx="4" cy="10" r="1" /><circle cx="10" cy="10" r="1" /><circle cx="16" cy="10" r="1" /></svg>
      </button>
      <div id="product-selection-actions" class="selection-secondary-actions" :class="{ 'is-open': actionsOpen }">
        <button type="button" class="secondary small select-displayed" :disabled="disabled || !hasProducts" @click="toggleDisplayed">{{ allDisplayedSelected ? '取消全选' : '全选已显示' }}</button>
        <button type="button" class="text-button cancel-selection" @click="emit('cancel')">取消多选</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.product-selection-bar{position:fixed;left:1.5rem;right:1.5rem;bottom:calc(.75rem + env(safe-area-inset-bottom,0px));z-index:30;display:flex;align-items:center;gap:.6rem;max-width:calc(1060px - 3rem);margin:0 auto;padding:.65rem .8rem;background:color-mix(in srgb,var(--accent) 5%,var(--surface));border:1px solid var(--focus);border-radius:10px;box-shadow:var(--shadow)}
.product-selection-bar p{margin:0 auto 0 0;font-size:.85rem;color:var(--muted);white-space:nowrap}
.product-selection-bar strong{color:var(--text)}
.product-selection-bar button{font-size:.8rem;white-space:nowrap}
.selection-more{position:relative;flex:none}
.selection-more-trigger{display:none}
.selection-secondary-actions{display:flex;align-items:center;gap:.6rem}
:global(body:has(.product-selection-bar) .toast){bottom:calc(5.75rem + env(safe-area-inset-bottom,0px))}
@media(max-width:760px){.product-selection-bar{left:max(.85rem,env(safe-area-inset-left,0px));right:max(.85rem,env(safe-area-inset-right,0px))}}
@media(max-width:600px){
  .product-selection-bar{gap:.5rem;padding:.5rem .6rem}
  .product-selection-bar p{font-size:.8rem}
  .product-selection-bar button{min-height:2.75rem}
  .selection-count-noun{display:none}
  .selection-more-trigger{display:grid;place-items:center;width:2.75rem;padding:0}
  .selection-more-trigger svg{width:1.25rem;height:1.25rem;fill:currentColor}
  .selection-secondary-actions{position:absolute;bottom:calc(100% + 1rem);right:0;display:none;min-width:10rem;padding:.35rem;border:1px solid var(--menu-border);border-radius:9px;background:var(--menu-bg);box-shadow:var(--menu-shadow)}
  .selection-secondary-actions.is-open{display:flex;flex-direction:column;align-items:stretch;gap:.2rem}
  .selection-secondary-actions button{justify-content:flex-start;width:100%;padding:.65rem .75rem;border:0;border-radius:5px;background:transparent;color:var(--menu-text);text-align:left}
  .selection-secondary-actions button:hover{background:var(--menu-hover)}
  :global(body:has(.product-selection-bar .is-open) .toast){bottom:calc(13rem + env(safe-area-inset-bottom,0px))}
}
</style>
