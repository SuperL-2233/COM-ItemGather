<script setup lang="ts">
import { computed, ref } from 'vue'

interface FilterOption { value: string; label: string; color?: string }

const props = defineProps<{
  label: string
  allLabel: string
  modelValue: string[]
  options: (string | FilterOption)[]
  multiple?: boolean
}>()
const emit = defineEmits<{ 'update:modelValue': [value: string[]] }>()
const search = ref('')
const value = (option: string | FilterOption) => typeof option === 'string' ? option : option.value
const optionLabel = (option: string | FilterOption) => typeof option === 'string' ? option : option.label
const optionColor = (option: string | FilterOption) => typeof option === 'string' ? undefined : option.color
const filteredOptions = computed(() => {
  const query = search.value.trim().toLocaleLowerCase()
  return props.options.filter(option => !query || optionLabel(option).toLocaleLowerCase().includes(query))
})
function toggle(option: string | FilterOption) {
  const selected = value(option)
  emit('update:modelValue', props.modelValue.includes(selected) ? props.modelValue.filter(item => item !== selected) : props.multiple === false ? [selected] : [...props.modelValue, selected])
}
</script>

<template>
  <section class="filter-group">
    <h3>{{ label }}</h3>
    <input v-model="search" type="search" :aria-label="`搜索${label}选项`" :placeholder="`搜索${label}选项`" @keydown.enter.stop.prevent />
    <div class="filter-option-list">
      <button type="button" class="filter-option" :class="{ active: modelValue.length === 0 }" :aria-pressed="modelValue.length === 0" @click="emit('update:modelValue', [])"><span>{{ allLabel }}</span><span v-if="modelValue.length === 0" aria-hidden="true">✓</span></button>
      <button v-for="option in filteredOptions" :key="value(option)" type="button" class="filter-option" :class="{ active: modelValue.includes(value(option)) }" :aria-pressed="modelValue.includes(value(option))" @click="toggle(option)"><span class="filter-option-label"><span v-if="optionColor(option)" class="filter-color-dot" :style="{ backgroundColor: optionColor(option) }" aria-hidden="true"></span>{{ optionLabel(option) }}</span><span v-if="modelValue.includes(value(option))" aria-hidden="true">✓</span></button>
      <p v-if="!filteredOptions.length" class="muted filter-no-options">没有匹配选项</p>
    </div>
  </section>
</template>

<style scoped>
.filter-option-label{display:flex;align-items:center;gap:.5rem;min-width:0}
.filter-color-dot{width:8px;height:8px;flex:none;border-radius:50%;box-shadow:0 0 0 1px color-mix(in srgb,var(--text) 15%,transparent)}
</style>
