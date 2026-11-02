<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import type { FilterDefinition } from '../types'
import ProductFilterGroup from './ProductFilterGroup.vue'

const props = defineProps<{ id: string; label: string; groups: FilterDefinition[]; modelValue: Record<string, string[]> }>()
const emit = defineEmits<{ 'update:modelValue': [value: Record<string, string[]>] }>()
const open = ref(false)
const active = ref(props.groups[0]?.key || '')
const control = ref<HTMLElement>()
const popover = ref<HTMLElement>()
const button = ref<HTMLButtonElement>()
const count = computed(() => props.groups.filter(group => props.modelValue[group.key]?.length).length)
const group = computed(() => props.groups.find(item => item.key === active.value) || props.groups[0])

watch(() => props.groups, () => { if (!props.groups.some(item => item.key === active.value)) active.value = props.groups[0]?.key || '' })
async function toggle() {
  open.value = !open.value
  if (open.value) { await nextTick(); popover.value?.querySelector('input')?.focus() }
}
function update(key: string, values: string[]) { emit('update:modelValue', { ...props.modelValue, [key]: values }) }
function clear() { emit('update:modelValue', Object.fromEntries(Object.keys(props.modelValue).map(key => [key, []]))) }
function outside(event: MouseEvent) {
  if (open.value && control.value && !event.composedPath().includes(control.value)) open.value = false
}
function escape(event: KeyboardEvent) {
  if (event.key === 'Escape' && open.value) { event.stopPropagation(); open.value = false; button.value?.focus() }
}
onMounted(() => { document.addEventListener('click', outside); document.addEventListener('keydown', escape) })
onUnmounted(() => { document.removeEventListener('click', outside); document.removeEventListener('keydown', escape) })
</script>

<template>
  <div ref="control" class="filter-control">
    <button ref="button" type="button" class="secondary filter-button" :class="{ active: count }" :aria-label="`筛选${label}`" :aria-expanded="open" :aria-controls="id" @click="toggle">
      <svg class="filter-icon" viewBox="0 0 24 24" aria-hidden="true"><path fill="currentColor" d="M3 4h18l-7 8v6l-4 2v-8L3 4Z" /></svg>
      <span v-if="count" class="filter-count">{{ count }}</span>
    </button>
    <div v-if="open" :id="id" ref="popover" class="filter-popover" role="dialog" :aria-label="`${label}筛选`">
      <div class="filter-popover-layout">
        <nav class="filter-category-list" aria-label="筛选分类">
          <button v-for="item in groups" :key="item.key" type="button" class="filter-category" :class="{ active: group?.key === item.key }" :aria-pressed="group?.key === item.key" @click="active = item.key">
            <span>{{ item.label }}</span><span v-if="modelValue[item.key]?.length" class="filter-category-dot" aria-hidden="true"></span>
          </button>
        </nav>
        <div class="filter-options-pane">
          <ProductFilterGroup v-if="group" :key="group.key" :label="group.label" :all-label="group.allLabel" :options="group.options" :multiple="group.multiple" :model-value="modelValue[group.key] || []" @update:model-value="update(group.key, $event)" />
        </div>
      </div>
      <div class="filter-popover-actions"><button type="button" class="button-link" :disabled="!count" @click="clear">清除筛选</button><button type="button" class="secondary small" @click="open = false">完成</button></div>
    </div>
  </div>
</template>
