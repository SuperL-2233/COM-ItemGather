<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { api } from '../api'
import { useLiveSearch } from '../composables/useLiveSearch'
import { useMindMapViewport } from '../composables/mindMapViewport'
import { useEvents } from '../events'
import type { ItemMindMap, MindMapCompany, MindMapItem } from '../types'
import { companyTitle, itemPath, productTitle } from '../utils'
import StatusBadge from '../components/StatusBadge.vue'

type Branch = { id: string; title: string; company?: MindMapCompany; items: MindMapItem[]; total: number }
const unassigned = '__unassigned__'
const canvasWidth = 1016
const rowHeight = 108
const snapshot = ref<ItemMindMap>({ companies: [], items: [], statuses: [] })
const ready = ref(false)
const query = ref('')
const companyID = ref('')
const category = ref<string | null>(null)
const status = ref('')
const collapsed = ref(new Set<string>())
const { viewport, zoom, dragging, setZoom, resetView, wheel, startDrag, moveDrag, endDrag, startTouch, moveTouch, endTouch, cancelTouch, guardClick } = useMindMapViewport(canvasWidth)
const online = ref(navigator.onLine)
const lastSync = ref('')
let fallbackTimer: ReturnType<typeof setInterval> | undefined

const { loading, error, run: load, refresh } = useLiveSearch(
  signal => api<ItemMindMap>('/item-mind-map', { signal }),
  result => {
    snapshot.value = result
    ready.value = true
    lastSync.value = new Date().toLocaleTimeString('zh-CN', { hour12: false })
    if (companyID.value && companyID.value !== unassigned && !result.companies.some(company => company.id === companyID.value)) companyID.value = ''
    if (status.value && !result.statuses.some(option => option.value === status.value)) status.value = ''
    if (category.value !== null && !result.items.some(item => item.category === category.value)) category.value = null
    const ids = new Set([unassigned, ...result.companies.map(company => company.id)])
    collapsed.value = new Set([...collapsed.value].filter(id => ids.has(id)))
  },
)

const allBranches = computed(() => {
  const branches: Branch[] = snapshot.value.companies.map(company => ({ id: company.id, title: companyTitle(company), company, items: [], total: 0 }))
  const byID = new Map(branches.map(branch => [branch.id, branch]))
  const loose: Branch = { id: unassigned, title: '未归属公司', items: [], total: 0 }
  for (const item of snapshot.value.items) {
    const branch = byID.get(item.company_id) || loose
    branch.items.push(item)
    branch.total++
  }
  if (loose.items.length) branches.push(loose)
  return branches
})

const categories = computed(() => [...new Set(snapshot.value.items.map(item => item.category))].sort((left, right) => left.localeCompare(right, 'zh-CN', { numeric: true })))
const filtered = computed(() => {
  const term = query.value.trim().toLocaleLowerCase()
  const matches = (values: string[]) => values.some(value => value.toLocaleLowerCase().includes(term))
  return allBranches.value.flatMap(branch => {
    if (companyID.value && branch.id !== companyID.value) return []
    const companyMatches = !term || matches([branch.title, branch.company?.english_name || '', branch.company?.aliases || ''])
    const items = branch.items.filter(item =>
      (category.value === null || item.category === category.value) &&
      (!status.value || item.development_status === status.value) &&
      (companyMatches || matches([item.chinese_name, item.english_name, item.category])),
    )
    if (!items.length && (category.value !== null || status.value || !companyMatches || (term && branch.items.length))) return []
    return [{ ...branch, items }]
  })
})
const itemCount = computed(() => filtered.value.reduce((total, branch) => total + branch.items.length, 0))
const hasFilters = computed(() => !!(query.value || companyID.value || category.value !== null || status.value))
const allCollapsed = computed(() => filtered.value.filter(branch => branch.items.length).every(branch => collapsed.value.has(branch.id)))
const layout = computed(() => {
  let top = 32
  const branches = filtered.value.map(branch => {
    const folded = collapsed.value.has(branch.id)
    const height = Math.max(94, folded ? 94 : branch.items.length * rowHeight - 14)
    const placed = { ...branch, top, height, folded }
    top += height + 32
    return placed
  })
  return { branches, height: Math.max(340, top) }
})
const syncLabel = computed(() => !online.value ? '离线，显示上次数据' : error.value ? '同步失败' : loading.value ? '正在同步…' : `已自动同步 · ${lastSync.value}`)

function toggle(id: string) {
  if (collapsed.value.has(id)) collapsed.value.delete(id)
  else collapsed.value.add(id)
}
function toggleAll() {
  const fold = !allCollapsed.value
  for (const branch of filtered.value) {
    if (fold) collapsed.value.add(branch.id)
    else collapsed.value.delete(branch.id)
  }
}
function clearFilters() { query.value = ''; companyID.value = ''; category.value = null; status.value = '' }
function rootEdge(top: number) { return `M 224 79 H 264 V ${top + 47} H 304` }
function itemEdge(top: number, index: number) { return `M 552 ${top + 47} H 596 V ${top + index * rowHeight + 47} H 640` }

function refreshVisible() { if (!document.hidden && !loading.value && online.value) void load() }
function networkChanged() { online.value = navigator.onLine; if (online.value) refreshVisible() }

watch([query, companyID, category, status], () => { viewport.value?.scrollTo({ left: 0, top: 0 }) })
watch(query, value => { if (value.trim()) collapsed.value.clear() })
onMounted(() => {
  void load()
  // Events update immediately; a periodic snapshot also picks up database edits
  // made outside the application and recovers from a missed notification.
  fallbackTimer = setInterval(refreshVisible, 15000)
  window.addEventListener('online', networkChanged)
  window.addEventListener('offline', networkChanged)
  window.addEventListener('focus', refreshVisible)
  document.addEventListener('visibilitychange', refreshVisible)
})
onUnmounted(() => {
  clearInterval(fallbackTimer)
  window.removeEventListener('online', networkChanged)
  window.removeEventListener('offline', networkChanged)
  window.removeEventListener('focus', refreshVisible)
  document.removeEventListener('visibilitychange', refreshVisible)
})
useEvents(refresh)
</script>

<template>
  <section class="mind-map-page" aria-labelledby="mind-map-title">
    <div class="page-heading mind-map-heading">
      <div>
        <h2 id="mind-map-title">思维导图</h2>
        <p>从公司看到旗下商品，用边框颜色掌握制作状态。点击节点名称可查看或编辑详情。</p>
      </div>
      <span class="mind-map-sync" :class="{ 'sync-error': !!error || !online }" role="status"><span aria-hidden="true"></span>{{ syncLabel }}</span>
    </div>

    <form class="mind-map-filters" aria-label="导图筛选" @submit.prevent>
      <label class="mind-map-search">搜索<input v-model="query" type="search" aria-label="搜索导图" placeholder="公司、别名、商品名称或商品分类" /></label>
      <label>所属公司<select v-model="companyID" aria-label="导图所属公司"><option value="">全部公司与未归属商品</option><option v-for="company in snapshot.companies" :key="company.id" :value="company.id">{{ companyTitle(company) }}</option><option :value="unassigned">未归属公司</option></select></label>
      <label>商品分类<select v-model="category" aria-label="导图商品分类"><option :value="null">全部商品分类</option><option v-for="value in categories" :key="value" :value="value">{{ value || '未分类' }}</option></select></label>
      <label>制作状态<select v-model="status" aria-label="导图制作状态"><option value="">全部状态</option><option v-for="option in snapshot.statuses" :key="option.value" :value="option.value">{{ option.label }}</option></select></label>
      <button v-if="hasFilters" type="button" class="text-button" @click="clearFilters">清除筛选</button>
    </form>

    <p v-if="error" class="error-text mind-map-error" role="alert">{{ ready ? '自动同步失败，当前保留上次成功读取的导图。' : '导图读取失败。' }}{{ error }} <button class="button-link" @click="load">重试</button></p>
    <div v-if="!ready && loading" class="empty-state" aria-busy="true">正在读取公司与商品关系…</div>
    <div v-else-if="ready && !snapshot.companies.length && !snapshot.items.length" class="empty-state">还没有公司或商品。<RouterLink to="/companies">创建公司</RouterLink>或<RouterLink to="/products">添加商品</RouterLink>后，这里会自动生成导图。</div>
    <template v-else-if="ready">
      <div class="mind-map-board">
        <div class="mind-map-controls">
          <span class="mind-map-count">{{ itemCount }} 件商品 · {{ filtered.length }} 个分支<span v-if="hasFilters">（已筛选）</span></span>
          <div class="mind-map-buttons" role="group" aria-label="导图视图控制">
            <button type="button" class="secondary small" :disabled="!itemCount" @click="toggleAll">{{ allCollapsed ? '全部展开' : '全部折叠' }}</button>
            <button type="button" class="secondary small zoom-button" aria-label="缩小导图" :disabled="zoom <= .25" @click="setZoom(zoom - .15)">−</button>
            <output class="mind-map-zoom" aria-label="导图缩放比例">{{ Math.round(zoom * 100) }}%</output>
            <button type="button" class="secondary small zoom-button" aria-label="放大导图" :disabled="zoom >= 1.75" @click="setZoom(zoom + .15)">＋</button>
            <button type="button" class="secondary small" @click="resetView(true)">适应宽度</button>
            <button type="button" class="text-button" @click="resetView()">重置视图</button>
          </div>
        </div>
        <div v-if="!filtered.length" class="empty-state">没有符合条件的公司或商品。<button class="button-link" @click="clearFilters">清除筛选</button></div>
        <div v-else ref="viewport" class="mind-map-viewport" :class="{ dragging }" tabindex="0" role="region" aria-label="公司与商品思维导图" aria-describedby="mind-map-help" @pointerdown="startDrag" @pointermove="moveDrag" @pointerup="endDrag" @pointercancel="endDrag" @lostpointercapture="endDrag" @wheel="wheel" @touchstart="startTouch" @touchmove="moveTouch" @touchend="endTouch" @touchcancel="cancelTouch" @click.capture="guardClick">
          <div class="mind-map-stage" :style="{ width: `${canvasWidth * zoom}px`, height: `${layout.height * zoom}px` }">
            <div class="mind-map-canvas" :style="{ width: `${canvasWidth}px`, height: `${layout.height}px`, transform: `scale(${zoom})` }">
              <svg class="mind-map-edges" :width="canvasWidth" :height="layout.height" aria-hidden="true">
                <g v-for="branch in layout.branches" :key="branch.id">
                  <path :d="rootEdge(branch.top)" class="company-edge" />
                  <template v-if="!branch.folded"><path v-for="(item, index) in branch.items" :key="item.id" :d="itemEdge(branch.top, index)" class="item-edge" /></template>
                </g>
              </svg>
              <div class="mind-map-root">
                <span class="mind-map-eyebrow">商品关系</span>
                <strong>公司与商品</strong>
                <small>{{ snapshot.companies.length }} 家公司 · {{ snapshot.items.length }} 件商品</small>
              </div>
              <section v-for="branch in layout.branches" :key="branch.id" class="mind-map-branch" :aria-label="`${branch.title}分支`" :data-company-id="branch.id" :style="{ top: `${branch.top}px`, height: `${branch.height}px` }">
                <div class="mind-map-company" :class="{ unassigned: !branch.company }">
                  <button v-if="branch.items.length" type="button" class="mind-map-toggle" :aria-label="`${branch.folded ? '展开' : '折叠'}${branch.title}`" :aria-expanded="!branch.folded" @click="toggle(branch.id)">{{ branch.folded ? '＋' : '−' }}</button>
                  <span v-else class="mind-map-toggle empty" aria-hidden="true">○</span>
                  <div class="mind-map-company-info">
                    <RouterLink v-if="branch.company" :to="`/companies/${branch.id}`" :title="branch.title" :aria-label="branch.title" class="mind-map-node-title">{{ branch.title }}</RouterLink>
                    <strong v-else class="mind-map-node-title">{{ branch.title }}</strong>
                    <small v-if="branch.company?.chinese_name && branch.company.english_name" :title="branch.company.english_name">{{ branch.company.english_name }}</small>
                    <small>{{ branch.items.length ? `${branch.items.length} 件商品` : '暂无关联商品' }}<span v-if="branch.total !== branch.items.length"> / 共 {{ branch.total }} 件</span><span v-if="branch.folded"> · 已折叠</span></small>
                  </div>
                </div>
                <template v-if="!branch.folded">
                  <article v-for="(item, index) in branch.items" :key="item.id" class="mind-map-item" :data-item-id="item.id" :aria-label="`${productTitle(item)}，${item.development_status_label}`" :style="{ top: `${index * rowHeight}px`, '--item-status-color': item.development_status_color }">
                    <div class="mind-map-item-heading"><span class="mind-map-item-category" :title="item.category || '未分类'" :aria-label="`商品分类：${item.category || '未分类'}`">{{ item.category || '未分类' }}</span><RouterLink :to="itemPath(item)" :title="productTitle(item)" :aria-label="productTitle(item)" class="mind-map-node-title">{{ productTitle(item) }}</RouterLink></div>
                    <small v-if="item.chinese_name && item.english_name" class="mind-map-item-english" :title="item.english_name">{{ item.english_name }}</small>
                    <small v-if="item.company_deleted" class="mind-map-item-note">原公司已删除</small>
                  </article>
                </template>
              </section>
            </div>
          </div>
        </div>
        <div id="mind-map-help" class="mind-map-help"><span>手机单指拖动、双指缩放；电脑可拖动、滚动或按 Ctrl / ⌘ + 滚轮缩放。</span><span>边框颜色表示制作状态，对照下方图例。</span></div>
      </div>
      <div class="mind-map-legend" aria-label="制作状态图例"><span>制作状态</span><StatusBadge v-for="option in snapshot.statuses" :key="option.value" :label="option.label" :color="option.color" /></div>
    </template>
  </section>
</template>

<style scoped>
.mind-map-page{min-width:0}
.mind-map-heading{align-items:center;flex-wrap:wrap;margin-bottom:1rem}
.mind-map-heading h2{font-size:1.35rem;margin-bottom:.45rem}
.mind-map-heading p{font-size:.88rem;line-height:1.6}
.mind-map-sync{display:inline-flex;align-items:center;gap:.45rem;flex:none;font-size:.78rem;color:var(--muted)}
.mind-map-sync>span{width:7px;height:7px;border-radius:50%;background:var(--accent)}
.mind-map-sync.sync-error>span{background:var(--danger)}
.mind-map-filters{display:flex;align-items:end;gap:.7rem;flex-wrap:wrap;margin-bottom:1rem}
.mind-map-filters label{display:flex;flex-direction:column;gap:.35rem;min-width:0;font-size:.78rem;font-weight:600;color:var(--muted)}
.mind-map-filters input,.mind-map-filters select{font-size:.85rem;font-weight:400;color:var(--text);max-width:100%}
.mind-map-filters select{min-width:7rem;width:100%}
.mind-map-filters .mind-map-search{flex:1;min-width:220px}
.mind-map-filters>label:nth-child(2){max-width:230px}
.mind-map-filters>.text-button{min-height:2.6rem;font-size:.82rem}
.mind-map-error{font-size:.85rem;line-height:1.6}
.mind-map-board{border:1px solid var(--border);border-radius:10px;background:var(--surface);overflow:hidden}
.mind-map-controls{display:flex;align-items:center;justify-content:space-between;gap:.75rem;flex-wrap:wrap;padding:.75rem 1rem;border-bottom:1px solid var(--border)}
.mind-map-count{font-size:.82rem;color:var(--muted)}
.mind-map-buttons{display:flex;align-items:center;gap:.4rem;flex-wrap:wrap}
.mind-map-buttons button{font-size:.78rem}
.mind-map-buttons .zoom-button{padding:.3rem .55rem;min-width:2.15rem;font-size:1rem}
.mind-map-zoom{min-width:2.7rem;text-align:center;font-size:.8rem;font-variant-numeric:tabular-nums;color:var(--muted)}
.mind-map-viewport{height:clamp(380px,61vh,720px);width:100%;overflow:auto;position:relative;isolation:isolate;cursor:grab;background-color:var(--surface-soft);background-image:radial-gradient(var(--border) 1px,transparent 1px);background-size:18px 18px;overscroll-behavior:contain;touch-action:none}
.mind-map-viewport.dragging{cursor:grabbing;user-select:none}
.mind-map-viewport:focus-visible{outline-offset:-3px}
.mind-map-stage{position:relative;min-width:100%;min-height:100%;overflow:hidden}
.mind-map-canvas{position:absolute;inset:0 auto auto 0;transform-origin:top left}
.mind-map-edges{position:absolute;inset:0;pointer-events:none;overflow:visible}
.mind-map-edges path{fill:none;stroke-width:1.5;stroke-opacity:.48;stroke-linejoin:round}
.mind-map-edges .company-edge{stroke:var(--accent);stroke-opacity:.4}
.mind-map-edges .item-edge{stroke:var(--muted);stroke-opacity:.3}
.mind-map-root{position:absolute;top:32px;left:24px;width:200px;min-height:94px;display:flex;flex-direction:column;justify-content:center;gap:.4rem;padding:.9rem 1.1rem;border-radius:10px;background:var(--accent);color:var(--surface);box-shadow:0 3px 10px #0001}
.mind-map-root strong{font-size:1.05rem}
.mind-map-root small{font-size:.75rem;opacity:.9}
.mind-map-eyebrow{font-size:.65rem;letter-spacing:.12em;opacity:.8}
.mind-map-branch{position:absolute;left:0;width:100%;pointer-events:none}
.mind-map-company,.mind-map-item{position:absolute;pointer-events:auto;background:var(--surface);border:1px solid var(--border);border-radius:9px;box-shadow:0 2px 5px #00000006}
.mind-map-company{left:304px;top:0;width:248px;height:94px;padding:.8rem;display:flex;align-items:center;gap:.7rem;border-color:color-mix(in srgb,var(--accent) 40%,var(--border))}
.mind-map-company.unassigned{border-style:dashed}
.mind-map-toggle{display:flex;align-items:center;justify-content:center;width:26px;height:26px;flex:none;padding:0;border:1px solid var(--border);border-radius:6px;background:var(--surface-soft);color:var(--accent);font-size:1rem}
.mind-map-toggle.empty{color:var(--muted);border-color:transparent;background:transparent;font-size:.7rem}
.mind-map-company-info{min-width:0;flex:1}
.mind-map-node-title{display:block;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;color:var(--text);font-size:.9rem;font-weight:650;line-height:1.5}
a.mind-map-node-title:hover{color:var(--accent)}
.mind-map-company small{display:block;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;font-size:.72rem;line-height:1.6;color:var(--muted)}
.mind-map-item{left:640px;width:340px;height:94px;border:2px solid var(--item-status-color);padding:.65rem .8rem;display:flex;flex-direction:column;justify-content:center;gap:.35rem}
.mind-map-item-heading{display:flex;align-items:center;gap:.5rem;min-width:0}
.mind-map-item-heading a{min-width:0;flex:1}
.mind-map-item-category{font-size:.65rem;max-width:45%;flex-shrink:0;border-radius:4px;padding:.13rem .3rem;background:var(--surface-soft);color:var(--muted);white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.mind-map-item-english{font-size:.7rem;color:var(--muted);white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.mind-map-item-note{font-size:.7rem;color:var(--muted)}
.mind-map-help{display:flex;justify-content:space-between;gap:.5rem;flex-wrap:wrap;padding:.65rem 1rem;border-top:1px solid var(--border);color:var(--muted);font-size:.72rem;line-height:1.6}
.mind-map-legend{display:flex;align-items:center;gap:.5rem;flex-wrap:wrap;margin-top:1rem;color:var(--muted);font-size:.75rem}
.mind-map-legend>span:first-child{margin-right:.25rem}
@media(max-width:600px){.mind-map-filters{display:grid;grid-template-columns:1fr 1fr}.mind-map-filters .mind-map-search{grid-column:1/-1;min-width:0}.mind-map-filters>label:nth-child(2){max-width:none}.mind-map-filters>label:last-of-type{grid-column:1/-1}.mind-map-filters select{min-width:0}.mind-map-controls{padding:.65rem .7rem}.mind-map-buttons{gap:.3rem}.mind-map-buttons .small{padding:.3rem .5rem}.mind-map-help{padding:.65rem .7rem}.mind-map-viewport{height:56vh;min-height:360px}}
</style>
