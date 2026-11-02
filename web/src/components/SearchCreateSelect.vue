<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, useId, watch } from 'vue'

import type { SearchOption } from '../types'

const value = defineModel<string>({ required: true })
const props = withDefaults(defineProps<{
  label: string
  options: SearchOption[]
  createLabel: string
  create: (name: string) => Promise<SearchOption>
  searchLabel?: string
  newLabel?: string
  fallback?: string
  allowEmpty?: boolean
  emptyLabel?: string
  maxLength?: number
  loading?: boolean
  error?: string
  fullDetails?: boolean
}>(), { allowEmpty: true, emptyLabel: '未设置', maxLength: 100 })
const emit = defineEmits<{ details: [name: string]; retry: [] }>()
const id = useId()
const root = ref<HTMLElement>()
const searchInput = ref<HTMLInputElement>()
const nameInput = ref<HTMLInputElement>()
const trigger = ref<HTMLButtonElement>()
const open = ref(false)
const adding = ref(false)
const query = ref('')
const name = ref('')
const active = ref(-1)
const busy = ref(false)
const above = ref(false)
const panelHeight = ref(360)
const localError = ref('')
const selected = computed(() => props.options.find(option => option.value === value.value))
const display = computed(() => value.value ? selected.value?.label || props.fallback || value.value : props.emptyLabel)
const filtered = computed(() => {
  const needle = query.value.trim().toLocaleLowerCase()
  return props.options.filter(option => (option.searchText || option.label).toLocaleLowerCase().includes(needle))
})
const rows = computed<(SearchOption & { kind: string })[]>(() => [
  { kind: 'create', value: '', label: props.createLabel },
  ...(props.allowEmpty ? [{ kind: 'empty', value: '', label: props.emptyLabel }] : []),
  ...filtered.value.map(option => ({ ...option, kind: 'option' })),
])
// Keep the search draft when real-time updates change the selected value.
watch(query, () => { active.value = -1; if (!busy.value) adding.value = false })
async function expand() {
  open.value = true
  query.value = ''
  active.value = -1
  localError.value = ''
  await nextTick()
  placePanel()
  searchInput.value?.focus()
}
function close() {
  open.value = false
  adding.value = false
  active.value = -1
}
async function select(selectedValue: string) {
  value.value = selectedValue
  close()
  await nextTick()
  trigger.value?.focus()
}
async function startCreate() {
  adding.value = true
  name.value = query.value.trim()
  localError.value = ''
  active.value = -1
  await nextTick()
  placePanel()
  nameInput.value?.focus()
  nameInput.value?.select()
}
async function cancelCreate() {
  if (busy.value) return
  adding.value = false
  localError.value = ''
  active.value = 0
  await nextTick()
  searchInput.value?.focus()
}
async function createOption() {
  if (!name.value.trim() || busy.value) return
  busy.value = true
  localError.value = ''
  try {
    const option = await props.create(name.value.trim())
    await select(option.value)
  } catch (e) {
    localError.value = (e as Error).message
  } finally {
    busy.value = false
    if (localError.value && adding.value) { await nextTick(); nameInput.value?.focus() }
  }
}
function activate(index: number) {
  const row = rows.value[index]
  if (!row) return
  if (row.kind === 'create') void startCreate()
  else void select(row.value)
}
async function move(delta: number) {
  active.value = active.value < 0 ? (delta > 0 ? 0 : rows.value.length - 1) : (active.value + delta + rows.value.length) % rows.value.length
  await nextTick()
  document.getElementById(`${id}-option-${active.value}`)?.scrollIntoView({ block: 'nearest' })
}
function keydown(event: KeyboardEvent) {
  if (event.isComposing || event.keyCode === 229) return
  if (!['ArrowDown','ArrowUp','Enter','Escape'].includes(event.key)) return
  event.preventDefault()
  event.stopPropagation()
  if (event.key === 'ArrowDown') void move(1)
  else if (event.key === 'ArrowUp') void move(-1)
  else if (event.key === 'Enter') activate(active.value < 0 ? 0 : active.value)
  else { close(); void nextTick(() => trigger.value?.focus()) }
}
function rootKeydown(event: KeyboardEvent) {
  if (!open.value || event.key !== 'Escape' || event.isComposing) return
  event.preventDefault()
  event.stopPropagation()
  if (busy.value) return
  if (adding.value) void cancelCreate()
  else { close(); void nextTick(() => trigger.value?.focus()) }
}
function nameKeydown(event: KeyboardEvent) {
  if (event.isComposing || event.keyCode === 229) return
  if (event.key === 'Enter' || event.key === 'Escape') {
    event.preventDefault()
    event.stopPropagation()
    if (event.key === 'Enter') void createOption()
    else void cancelCreate()
  }
}
function details() {
  const initial = name.value
  close()
  emit('details', initial)
}
function placePanel() {
  if (!open.value || !root.value) return
  const rect = root.value.getBoundingClientRect()
  const modal = root.value.closest('.modal')?.getBoundingClientRect()
  const viewportBottom = window.visualViewport ? window.visualViewport.height + window.visualViewport.offsetTop : window.innerHeight
  const bottom = Math.min(viewportBottom - 12, modal ? modal.bottom - 12 : Infinity)
  const top = Math.max(12, modal ? modal.top + 12 : 0)
  const below = bottom - rect.bottom
  const availableAbove = rect.top - top
  above.value = below < 240 && availableAbove > below
  panelHeight.value = Math.max(140, Math.min(360, (above.value ? availableAbove : below) - 8))
}
function outside(event: MouseEvent) {
  if (!busy.value && root.value && !event.composedPath().includes(root.value)) close()
}
function focusOut(event: FocusEvent) {
  if (!busy.value && event.relatedTarget && !root.value?.contains(event.relatedTarget as Node)) close()
}
onMounted(() => { document.addEventListener('click', outside); window.addEventListener('resize',placePanel); window.visualViewport?.addEventListener('resize',placePanel) })
onUnmounted(() => { document.removeEventListener('click', outside); window.removeEventListener('resize',placePanel); window.visualViewport?.removeEventListener('resize',placePanel) })
defineExpose({ select })
</script>

<template>
  <div ref="root" class="search-create-select" @focusout="focusOut" @keydown="rootKeydown">
    <button v-if="!open" ref="trigger" type="button" class="select-trigger" :aria-label="label" aria-haspopup="listbox" :aria-expanded="false" @click="expand" @keydown.down.prevent="expand" @keydown.up.prevent="expand">
      <span>{{ display }}</span><span aria-hidden="true">⌄</span>
    </button>
    <input v-else ref="searchInput" v-model="query" class="select-search-input" role="combobox" :aria-label="searchLabel || `搜索${label}`" aria-autocomplete="list" aria-haspopup="listbox" :aria-expanded="true" :aria-controls="`${id}-options`" :aria-activedescendant="active >= 0 ? `${id}-option-${active}` : undefined" :placeholder="display" autocomplete="off" @keydown="keydown" />
    <div v-if="open" class="search-select-panel" :class="{ 'open-above': above }" :style="{ maxHeight: `${panelHeight}px` }">
      <div v-if="adding" class="inline-create-row">
        <label :for="`${id}-new`">{{ newLabel || `新${label}` }}</label>
        <input :id="`${id}-new`" ref="nameInput" v-model="name" :aria-label="newLabel || `新${label}`" :maxlength="maxLength" :disabled="busy" placeholder="输入新名称，按 Enter 确认" @keydown="nameKeydown" />
        <div class="inline-controls">
          <button type="button" class="secondary small" :disabled="busy || !name.trim()" @click="createOption">{{ busy ? '创建中…' : '确认' }}</button>
          <button type="button" class="text-button" :disabled="busy" @click="cancelCreate">取消</button>
          <button v-if="fullDetails" type="button" class="button-link" :disabled="busy" @click="details">填写完整公司资料</button>
        </div>
      </div>
      <div :id="`${id}-options`" class="choice-list" role="listbox" :aria-label="`${label}选项`">
        <template v-for="(row,index) in rows" :key="`${row.kind}:${row.value}`">
          <button v-if="row.kind !== 'create' || !adding" :id="`${id}-option-${index}`" type="button" role="option" tabindex="-1" class="choice-row" :class="{ 'keyboard-active': active === index, 'create-option': row.kind === 'create' }" :aria-selected="row.kind !== 'create' && value === row.value" :disabled="busy" @click="activate(index)" @mouseenter="active = index">
            <template v-if="row.kind === 'create'"><span>{{ createLabel }}</span></template>
            <template v-else-if="row.kind === 'empty'"><span>{{ emptyLabel }}</span></template>
            <template v-else>
              <img v-if="row.image" :src="row.image" alt="" class="company-logo-mini" />
              <span><strong>{{ row.label }}</strong><small v-if="row.description">{{ row.description }}</small></span>
              <span v-if="value === row.value" aria-hidden="true">✓</span>
            </template>
          </button>
        </template>
        <p v-if="loading" class="muted choice-hint">读取选项中…</p>
        <p v-else-if="!filtered.length" class="muted choice-hint">没有匹配选项，可通过“＋”创建。</p>
      </div>
      <p v-if="localError || error" class="error-text choice-hint" role="alert">{{ localError || error }} <button v-if="error" type="button" class="button-link" @click="emit('retry')">重试</button></p>
    </div>
  </div>
</template>
