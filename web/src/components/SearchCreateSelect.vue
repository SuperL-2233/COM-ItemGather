<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, useId, watch } from 'vue'

import type { SearchOption } from '../types'

const value = defineModel<string>({ required: true })
const props = withDefaults(defineProps<{
  label: string
  options: SearchOption[]
  createLabel: string
  create?: (name: string) => Promise<SearchOption>
  searchLabel?: string
  newLabel?: string
  fallback?: string
  allowEmpty?: boolean
  emptyLabel?: string
  maxLength?: number
  loading?: boolean
  error?: string
  fullDetails?: boolean
  allowRemove?: boolean
  removeNoun?: string
  allowCreate?: boolean
  compact?: boolean
}>(), { allowEmpty: true, emptyLabel: '未设置', maxLength: 100, removeNoun: '分类', allowCreate: true })
const emit = defineEmits<{ details: [name: string]; retry: []; remove: [option: SearchOption] }>()
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
  ...(props.allowCreate ? [{ kind: 'create', value: '', label: props.createLabel }] : []),
  ...(props.allowEmpty && (!props.compact || !query.value.trim()) ? [{ kind: 'empty', value: '', label: props.emptyLabel }] : []),
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
  if (!props.allowCreate || !props.create) return
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
  if (!name.value.trim() || busy.value || !props.create) return
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
function removeOption(option: SearchOption) {
  if (busy.value) return
  close()
  emit('remove',option)
}
function keydown(event: KeyboardEvent) {
  if (event.isComposing || event.keyCode === 229) return
  if (event.key === 'Delete' && event.shiftKey && props.allowRemove && rows.value[active.value]?.kind === 'option') { event.preventDefault(); event.stopPropagation(); removeOption(rows.value[active.value]); return }
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
defineExpose({ select, focus: () => trigger.value?.focus() })
</script>

<template>
  <div ref="root" class="search-create-select" :class="{ 'compact-select': compact }" @focusout="focusOut" @keydown="rootKeydown">
    <button v-if="!open" ref="trigger" type="button" class="select-trigger" :aria-label="label" :title="display" aria-haspopup="listbox" :aria-expanded="false" @click="expand" @keydown.down.prevent="expand" @keydown.up.prevent="expand">
      <span class="choice-selected"><span v-if="selected?.color" class="choice-color-dot" :style="{ backgroundColor: selected.color }" aria-hidden="true"></span><span>{{ display }}</span></span><svg class="select-chevron" viewBox="0 0 20 20" aria-hidden="true"><path d="m6 8 4 4 4-4" /></svg>
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
          <div v-if="row.kind !== 'create' || !adding" class="choice-option-row" role="presentation">
          <button :id="`${id}-option-${index}`" type="button" role="option" tabindex="-1" class="choice-row" :class="{ 'keyboard-active': active === index, 'create-option': row.kind === 'create' }" :aria-selected="row.kind !== 'create' && value === row.value" :disabled="busy" @click="activate(index)" @mouseenter="active = index">
            <template v-if="row.kind === 'create'"><span>{{ createLabel }}</span></template>
            <template v-else-if="row.kind === 'empty'"><span>{{ emptyLabel }}</span></template>
            <template v-else>
              <span v-if="row.color" class="choice-color-dot" :style="{ backgroundColor: row.color }" aria-hidden="true"></span>
              <img v-if="row.image" :src="row.image" alt="" class="company-logo-mini" />
              <span :title="row.label"><strong>{{ row.label }}</strong><small v-if="row.description">{{ row.description }}</small></span>
              <span v-if="value === row.value" aria-hidden="true">✓</span>
            </template>
          </button>
          <button v-if="allowRemove && row.kind === 'option'" type="button" tabindex="-1" class="choice-remove" :aria-label="`删除${removeNoun}“${row.label}”`" :title="`删除${removeNoun}；也可选中后按 Shift+Delete`" :disabled="busy" @click.stop="removeOption(row)">删除</button>
          </div>
        </template>
        <p v-if="loading" class="muted choice-hint">读取选项中…</p>
        <p v-else-if="!filtered.length" class="muted choice-hint">{{ allowCreate ? '没有匹配选项，可通过“＋”创建。' : '没有匹配选项。' }}</p>
      </div>
      <p v-if="allowRemove && filtered.length" class="muted choice-hint">选中{{ removeNoun }}后也可按 Shift+Delete 删除。</p>
      <p v-if="localError || error" class="error-text choice-hint" role="alert">{{ localError || error }} <button v-if="error" type="button" class="button-link" @click="emit('retry')">重试</button></p>
      <div v-if="$slots.footer" class="choice-footer"><slot name="footer" /></div>
    </div>
  </div>
</template>

<style scoped>
.choice-selected{display:flex;align-items:center;gap:.5rem;min-width:0;flex:1}
.choice-selected>span:last-child{min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.select-chevron{width:1rem;height:1rem;flex:none;fill:none;stroke:var(--menu-indicator);stroke-width:2;stroke-linecap:round;stroke-linejoin:round}
.choice-color-dot{width:8px;height:8px;flex:none!important;border-radius:50%;background:var(--muted);box-shadow:0 0 0 1px color-mix(in srgb,var(--text) 15%,transparent)}
.choice-row>.choice-color-dot+span{flex:1;min-width:0}
.choice-footer{border-top:1px solid var(--menu-divider);background:var(--menu-section);border-radius:0 0 4px 4px;padding:.65rem .55rem .4rem;margin-top:.3rem;font-size:.78rem}
.compact-select .search-select-panel{padding:.3rem}
.compact-select .choice-list{max-height:220px;overflow-y:auto}
.compact-select .choice-option-row{margin-bottom:.1rem}
.compact-select .choice-row{border:0;border-radius:5px;min-height:2.1rem;padding:.4rem .55rem;gap:.55rem;font-size:.86rem}
.compact-select .choice-row strong{font-weight:500;line-height:1.4;display:-webkit-box;-webkit-line-clamp:2;-webkit-box-orient:vertical;overflow:hidden}
.compact-select .select-trigger{padding:.55rem .7rem}
</style>
