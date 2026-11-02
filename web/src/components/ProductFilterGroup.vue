<script setup lang="ts">
import { computed, ref } from 'vue'

interface FilterOption { value: string; label: string }

const props = defineProps<{
  label: string
  allLabel: string
  modelValue: string
  options: (string | FilterOption)[]
}>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
const search = ref('')
const value = (option: string | FilterOption) => typeof option === 'string' ? option : option.value
const optionLabel = (option: string | FilterOption) => typeof option === 'string' ? option : option.label
const filteredOptions = computed(() => {
  const query = search.value.trim().toLocaleLowerCase()
  return props.options.filter(option => !query || optionLabel(option).toLocaleLowerCase().includes(query))
})
</script>

<template>
  <section class="filter-group">
    <h3>{{ label }}</h3>
    <input v-model="search" type="search" :aria-label="`搜索${label}选项`" :placeholder="`搜索${label}选项`" @keydown.enter.stop.prevent />
    <div class="filter-option-list">
      <button type="button" class="filter-option" :class="{ active: modelValue === '' }" @click="emit('update:modelValue', '')"><span>{{ allLabel }}</span><span v-if="modelValue === ''" aria-hidden="true">✓</span></button>
      <button v-for="option in filteredOptions" :key="value(option)" type="button" class="filter-option" :class="{ active: modelValue === value(option) }" @click="emit('update:modelValue', value(option))"><span>{{ optionLabel(option) }}</span><span v-if="modelValue === value(option)" aria-hidden="true">✓</span></button>
      <p v-if="!filteredOptions.length" class="muted filter-no-options">没有匹配选项</p>
    </div>
  </section>
</template>
