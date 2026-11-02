<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api, ApiError } from '../api'
import { useEvents } from '../events'
import type { ChoiceOption, OptionState } from '../types'
import { toast } from '../utils'
import StatusBadge from '../components/StatusBadge.vue'
import StatusDeleteDialog from '../components/StatusDeleteDialog.vue'
import CategoryDeleteDialog from '../components/CategoryDeleteDialog.vue'

const route = useRoute()
const router = useRouter()
const kind = computed(() => route.query.kind === 'categories' ? 'categories' : 'statuses')
const noun = computed(() => kind.value === 'statuses' ? '制作状态' : '商品分类')
const state = ref<OptionState>({ items: [], revision: '' })
const loading = ref(true)
const busy = ref(false)
const error = ref('')
const name = ref('')
const newColor = ref('#7c3aed')
const deleting = ref<ChoiceOption>()
const dragged = ref('')
const dropTarget = ref('')
const palette = [
  { color: '#7c3aed', label: '紫色' }, { color: '#2563eb', label: '蓝色' },
  { color: '#0891b2', label: '青色' }, { color: '#16845b', label: '绿色' },
  { color: '#b77912', label: '黄色' }, { color: '#d75531', label: '橙色' },
  { color: '#d14362', label: '红色' }, { color: '#6d7169', label: '灰色' },
]
let requestID = 0
async function load() {
  const request = ++requestID
  const currentKind = kind.value
  try {
    const result = await api<OptionState>(`/product-options/${currentKind}`)
    if (request === requestID && currentKind === kind.value) state.value = result
  } catch (e) { if (request === requestID) error.value = (e as Error).message }
  finally { if (request === requestID) loading.value = false }
}
async function mutate(action: () => Promise<OptionState>, message: string) {
  if (busy.value) return false
  const currentKind = kind.value
  busy.value = true
  requestID++
  error.value = ''
  try {
    const result = await action()
    if (currentKind !== kind.value) { void load(); return false }
    state.value = result
    toast(message)
    return true
  } catch (e) {
    if (currentKind !== kind.value) { void load(); return false }
    if (e instanceof ApiError && e.code === 'option_changed' && e.details?.items) state.value = e.details as OptionState
    else await load()
    error.value = (e as Error).message
    return false
  } finally { busy.value = false }
}
async function add() {
  if (!name.value.trim()) return
  const data = { name: name.value.trim(), ...(kind.value === 'statuses' ? { color: newColor.value } : {}) }
  if (await mutate(() => api<OptionState>(`/product-options/${kind.value}`, { method: 'POST', body: JSON.stringify(data) }), `已添加${noun.value}`)) name.value = ''
}
async function createForDialog(label: string) {
  const result = await api<OptionState & { option: ChoiceOption }>(`/product-options/${kind.value}`, { method: 'POST', body: JSON.stringify({ name: label }) })
  requestID++
  state.value = result
  return result.option
}
async function colorChanged(option: ChoiceOption, event: Event) {
  const input = event.target as HTMLInputElement
  const color = input.value
  await mutate(() => api<OptionState>(`/product-options/statuses/${encodeURIComponent(option.value)}/color`, { method: 'PATCH', body: JSON.stringify({ color, version: option.version }) }), '颜色已保存')
  input.value = state.value.items.find(item => item.value === option.value)?.color || option.color || '#7c3aed'
}
async function reorder(from: number, to: number) {
  if (busy.value || from < 0 || to < 0 || to >= state.value.items.length || from === to) return
  const items = [...state.value.items]
  items.splice(to, 0, items.splice(from, 1)[0])
  const revision = state.value.revision
  state.value = { ...state.value, items }
  await mutate(() => api<OptionState>(`/product-options/${kind.value}/order`, { method: 'PATCH', body: JSON.stringify({ order: items.map(item => item.value), revision }) }), '顺序已保存')
}
async function move(value: string, direction: number) {
  const index = state.value.items.findIndex(item => item.value === value)
  await reorder(index, index + direction)
  await nextTick()
  document.getElementById(`option-handle-${encodeURIComponent(value)}`)?.focus()
}
function dragStart(event: DragEvent, value: string) {
  if (busy.value) { event.preventDefault(); return }
  dragged.value = value
  if (event.dataTransfer) { event.dataTransfer.effectAllowed = 'move'; event.dataTransfer.setData('text/plain', value) }
}
function dragEnd() { dragged.value = ''; dropTarget.value = '' }
function drop(value: string) {
  if (!dragged.value) return
  const from = state.value.items.findIndex(item => item.value === dragged.value)
  const to = state.value.items.findIndex(item => item.value === value)
  dragEnd()
  void reorder(from, to)
}
function deleted() { deleting.value = undefined; error.value = ''; void load(); toast(`已删除${noun.value}`) }
function cancelDelete() {
  const value = deleting.value?.value
  deleting.value = undefined
  if (value) void nextTick(() => document.getElementById(`option-delete-${encodeURIComponent(value)}`)?.focus())
}
function changeKind(value: string) { if (!busy.value) void router.replace({ query: { kind: value } }) }
async function tabKeydown(event: KeyboardEvent) {
  if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return
  event.preventDefault()
  if (busy.value) return
  const value = event.key === 'Home' ? 'statuses' : event.key === 'End' ? 'categories' : kind.value === 'statuses' ? 'categories' : 'statuses'
  await router.replace({ query: { kind: value } })
  await nextTick()
  document.getElementById(`option-tab-${value}`)?.focus()
}
watch(kind, () => { state.value = { items: [], revision: '' }; name.value = ''; deleting.value = undefined; error.value = ''; loading.value = true; void load() })
onMounted(load)
useEvents(() => { if (!busy.value && !dragged.value) void load() })
</script>

<template>
  <section class="option-settings">
    <div class="page-heading"><div><RouterLink to="/settings">← 返回设置</RouterLink><h1>选项管理</h1><p>管理团队共用的制作状态和商品分类。</p></div></div>
    <div class="option-tabs" role="tablist" aria-label="选项类型">
      <button v-for="tab in [{ value: 'statuses', label: '制作状态' }, { value: 'categories', label: '商品分类' }]" :id="`option-tab-${tab.value}`" :key="tab.value" type="button" role="tab" :aria-selected="kind === tab.value" :tabindex="kind === tab.value ? 0 : -1" aria-controls="option-settings-panel" :disabled="busy" @click="changeKind(tab.value)" @keydown="tabKeydown">{{ tab.label }}</button>
    </div>
    <section id="option-settings-panel" class="panel options-panel" role="tabpanel" :aria-labelledby="`option-tab-${kind}`">
      <header class="options-heading"><div><h2>{{ noun }}</h2><p>拖动调整顺序，也可使用上下移动按钮。更改会同步给所有成员。</p></div><span class="option-total">{{ state.items.length }} 项</span></header>
      <form class="option-add-form" @submit.prevent="add">
        <div class="option-add-row"><label :for="`new-${kind}`" class="option-name-input"><span>新{{ noun }}</span><input :id="`new-${kind}`" v-model="name" :maxlength="kind === 'statuses' ? 80 : 100" :placeholder="kind === 'statuses' ? '例如：等待美术确认' : '例如：饮料、零食'" :disabled="busy" /></label><button type="submit" class="primary" :disabled="busy || loading || !name.trim()">添加{{ noun }}</button></div>
        <div v-if="kind === 'statuses'" class="new-status-colors"><span class="muted">颜色</span><div class="color-presets" role="group" aria-label="新制作状态颜色预设"><button v-for="color in palette" :key="color.color" type="button" class="color-preset" :style="{ '--swatch': color.color }" :aria-label="color.label" :aria-pressed="newColor === color.color" :disabled="busy" @click="newColor = color.color"><span v-if="newColor === color.color" aria-hidden="true">✓</span></button></div><label class="custom-color"><span>自定义</span><input v-model="newColor" type="color" aria-label="新制作状态颜色" :disabled="busy" /></label><StatusBadge class="new-status-preview" :label="name || '状态预览'" :color="newColor" /></div>
      </form>
      <p v-if="error" class="error-text option-error" role="alert">{{ error }}</p>
      <p v-if="loading" class="muted options-empty" role="status">读取选项中…</p>
      <ol v-else-if="state.items.length" class="option-list" :aria-label="`${noun}顺序`" :aria-busy="busy">
        <li v-for="(option, index) in state.items" :key="option.value" class="option-row" :class="{ 'category-row': kind === 'categories', 'drop-target': dragged && dropTarget === option.value && dragged !== option.value, dragging: dragged === option.value }" :data-value="option.value" @dragover.prevent="dropTarget = option.value" @drop.prevent="drop(option.value)">
          <div class="option-order"><button :id="`option-handle-${encodeURIComponent(option.value)}`" type="button" class="drag-handle" draggable="true" :aria-label="`拖动${noun}“${option.label}”排序`" :disabled="busy" title="拖动排序，也可使用右侧上下移动按钮" @dragstart="dragStart($event, option.value)" @dragend="dragEnd" @keydown.up.prevent="move(option.value, -1)" @keydown.down.prevent="move(option.value, 1)"><svg viewBox="0 0 16 20" aria-hidden="true"><circle v-for="point in [[5,5],[11,5],[5,10],[11,10],[5,15],[11,15]]" :key="point.join(',')" :cx="point[0]" :cy="point[1]" r="1.2" /></svg></button><span>{{ index + 1 }}</span></div>
          <div class="option-name"><StatusBadge v-if="kind === 'statuses'" :label="option.label" :color="option.color" /><strong v-else>{{ option.label }}</strong></div>
          <label v-if="kind === 'statuses'" class="option-color"><span>颜色</span><input type="color" :value="option.color" :aria-label="`${option.label}的颜色`" :disabled="busy" @change="colorChanged(option, $event)" /></label>
          <div class="option-actions"><button type="button" class="secondary small" :aria-label="`上移${noun}“${option.label}”`" :disabled="busy || index === 0" @click="move(option.value, -1)">↑</button><button type="button" class="secondary small" :aria-label="`下移${noun}“${option.label}”`" :disabled="busy || index === state.items.length - 1" @click="move(option.value, 1)">↓</button><button :id="`option-delete-${encodeURIComponent(option.value)}`" type="button" class="text-button danger-text" :aria-label="`删除${kind === 'statuses' ? '制作状态' : '分类'}“${option.label}”`" :disabled="busy" @click="deleting = option">删除</button></div>
        </li>
      </ol>
      <p v-else class="muted options-empty">还没有{{ noun }}，可以在上方添加。</p>
    </section>
    <StatusDeleteDialog v-if="deleting && kind === 'statuses'" :option="deleting" :options="state.items" :create="createForDialog" @deleted="deleted" @cancel="cancelDelete" />
    <CategoryDeleteDialog v-else-if="deleting" :name="deleting.value" :options="state.items" :create="createForDialog" @deleted="deleted" @cancel="cancelDelete" />
  </section>
</template>

<style scoped>
.option-settings{max-width:860px}
.page-heading h1{margin-top:.5rem}
.option-tabs{display:flex;gap:.4rem;margin-bottom:1rem;border-bottom:1px solid var(--border)}
.option-tabs button{padding:.75rem 1rem;border-bottom:2px solid transparent;background:transparent;color:var(--muted);font-weight:600}
.option-tabs button[aria-selected=true]{color:var(--accent);border-color:var(--accent)}
.options-panel{padding:0;overflow:hidden}
.options-heading{display:flex;align-items:flex-start;justify-content:space-between;gap:1rem;padding:1.3rem 1.3rem 1rem}
.options-heading h2{margin:0 0 .4rem}
.options-heading p{margin:0;color:var(--muted);font-size:.82rem;line-height:1.6}
.option-total{color:var(--muted);font-size:.8rem;white-space:nowrap;padding:.2rem 0}
.option-add-form{padding:0 1.3rem 1.2rem;border-bottom:1px solid var(--border)}
.option-add-row{display:flex;align-items:flex-end;gap:.75rem}
.option-name-input{display:flex;flex-direction:column;gap:.4rem;font-size:.8rem;flex:1;min-width:0}
.option-add-row .primary{flex:none}
.new-status-colors{display:flex;align-items:center;gap:.65rem;flex-wrap:wrap;margin-top:.9rem;font-size:.78rem}
.color-presets{display:flex;gap:.35rem}
.color-preset{display:grid;place-items:center;width:24px;height:24px;border:2px solid var(--surface);border-radius:50%;background:var(--swatch);color:#fff;font-size:.8rem;box-shadow:0 0 0 1px var(--border)}
.color-preset[aria-pressed=true]{box-shadow:0 0 0 2px var(--focus)}
.custom-color,.option-color{display:flex;align-items:center;gap:.4rem;color:var(--muted);font-size:.75rem}
input[type=color]{width:30px;height:30px;min-width:30px;padding:2px;border-radius:6px;cursor:pointer}
.new-status-preview{margin-left:auto}
.option-list{list-style:none;margin:0;padding:0}
.option-row{display:grid;grid-template-columns:48px minmax(0,1fr) auto auto;gap:1rem;align-items:center;padding:1rem 1.3rem;border-bottom:1px solid var(--border);background:var(--surface)}
.option-row:last-child{border-bottom:0}
.option-order{display:flex;align-items:center;gap:.3rem;color:var(--muted);font-size:.72rem;font-variant-numeric:tabular-nums}
.drag-handle{display:grid;place-items:center;padding:0;background:transparent;color:var(--muted);cursor:grab}
.drag-handle svg{width:16px;height:24px;fill:currentColor}
.option-name{min-width:0;overflow-wrap:anywhere;font-size:.9rem}
.option-name>strong{font-weight:500}
.option-name :deep(.status-badge){font-size:.86rem}
.option-actions{display:flex;gap:.35rem;align-items:center}
.option-actions>.secondary{padding:.3rem .55rem}
.option-actions>.text-button{font-size:.8rem;padding:.4rem .3rem;margin-left:.2rem}
.category-row .option-actions{grid-column:3/5;justify-self:end}
.drop-target{box-shadow:inset 0 0 0 2px var(--focus);background:var(--surface-soft)}
.dragging{opacity:.5}
.options-empty{padding:1.3rem;margin:0;font-size:.85rem}
.option-error{padding:.8rem 1.3rem;margin:0;font-size:.85rem;line-height:1.6}
@media(max-width:600px){.options-heading{padding:1rem}.option-add-form{padding:0 1rem 1rem}.option-add-row{align-items:stretch;flex-wrap:wrap}.option-name-input{flex-basis:100%}.option-add-row>.primary{width:100%;min-height:2.75rem}.new-status-colors{gap:.65rem}.new-status-preview{margin-left:0}.option-row{grid-template-columns:32px minmax(0,1fr) auto;gap:.7rem;padding:.85rem 1rem}.option-name{grid-column:2;grid-row:1}.option-color{grid-column:3;grid-row:1}.option-color>span{display:none}.option-actions,.category-row .option-actions{grid-column:2/-1;grid-row:2;justify-self:end}.category-row .option-name{grid-column:2/-1}.option-order{grid-column:1;grid-row:1;gap:.15rem}.option-tabs button{flex:1}.options-heading p{font-size:.78rem}}
</style>
