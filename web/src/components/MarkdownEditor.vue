<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Compartment, EditorState } from '@codemirror/state'
import { EditorView, drawSelection, highlightActiveLine, keymap, lineNumbers } from '@codemirror/view'
import { defaultKeymap, history, historyKeymap, indentWithTab, redo, undo } from '@codemirror/commands'
import { HighlightStyle, syntaxHighlighting } from '@codemirror/language'
import { markdown } from '@codemirror/lang-markdown'
import { GFM } from '@lezer/markdown'
import { tags } from '@lezer/highlight'
import { normalizeLineEndings, sourceDocument } from '../markdown/source'
import { editorFocus, headings, livePreview, sourceMode } from '../markdown/preview'
import type { MarkdownHeading } from '../markdown/preview'
import '../markdown/editor.css'

const props = defineProps<{ modelValue: string; disabled?: boolean; sourceOnly?: boolean; label?: string }>()
const emit = defineEmits<{ 'update:modelValue': [value: string]; save: []; error: [message: string] }>()
const host = ref<HTMLDivElement>()
const outline = ref<MarkdownHeading[]>([])
const outlineOpen = ref(!props.sourceOnly && window.innerWidth >= 760)
const showSource = ref(!!props.sourceOnly)
const fullscreen = ref(false)
const ready = ref(false)
const editable = new Compartment()
const presentation = new Compartment()
let view: EditorView | undefined
let source: ReturnType<typeof sourceDocument> | undefined
let previousOverflow = ''
let resumeFocus = false
const colors = HighlightStyle.define([
  { tag: tags.strong, fontWeight: '700' }, { tag: tags.emphasis, fontStyle: 'italic' },
  { tag: tags.strikethrough, textDecoration: 'line-through' },
  { tag: [tags.processingInstruction, tags.meta], color: 'var(--muted)' },
  { tag: [tags.link, tags.url], color: 'var(--accent)' },
  { tag: tags.monospace, fontFamily: 'ui-monospace, SFMono-Regular, Consolas, monospace' },
])
function getValue() { return view && source ? view.state.field(source.field).text : props.modelValue }
defineExpose({ getValue })
function createState(text: string) {
  source = sourceDocument(text)
  return EditorState.create({
    doc: normalizeLineEndings(text),
    extensions: [
      source.extension, history(), drawSelection(), highlightActiveLine(),
      markdown({ extensions: GFM }), syntaxHighlighting(colors), EditorView.lineWrapping,
      editable.of([EditorState.readOnly.of(!!props.disabled), EditorView.editable.of(!props.disabled)]),
      presentation.of(showSource.value ? lineNumbers() : []), livePreview,
      keymap.of([
        { key: 'Mod-s', run: () => { emit('save'); return true } },
        { key: 'Mod-z', run: undo, shift: redo, preventDefault: true },
        ...defaultKeymap, ...historyKeymap, indentWithTab,
      ]),
      EditorView.contentAttributes.of({ 'aria-label': props.label || 'README Markdown 编辑器', spellcheck: 'false' }),
      EditorView.focusChangeEffect.of((_state, focusing) => editorFocus.of(focusing)),
      EditorView.updateListener.of(update => {
        if (update.docChanged && source) {
          outline.value = headings(update.state)
          emit('update:modelValue', update.state.field(source.field).text)
        }
      }),
    ],
  })
}
function switchMode(value: boolean) {
  showSource.value = value
  view?.dispatch({ effects: [sourceMode.of(value), presentation.reconfigure(value ? lineNumbers() : [])] })
}
function toggleFullscreen() {
  fullscreen.value = !fullscreen.value
  if (fullscreen.value) { previousOverflow = document.body.style.overflow; document.body.style.overflow = 'hidden' }
  else document.body.style.overflow = previousOverflow
  void nextTick(() => view?.requestMeasure())
}
function jump(heading: MarkdownHeading) {
  if (!view) return
  view.dispatch({ selection: { anchor: heading.from }, effects: EditorView.scrollIntoView(heading.from, { y: 'start' }) })
  view.focus()
}
function wrap(before: string, after = before, placeholder = '文字') {
  if (!view || props.disabled) return
  const { from, to } = view.state.selection.main
  const selected = view.state.sliceDoc(from, to) || placeholder
  view.dispatch({ changes: { from, to, insert: before + selected + after }, selection: { anchor: from + before.length, head: from + before.length + selected.length }, scrollIntoView: true })
  view.focus()
}
function prefixLines(prefix: string) {
  if (!view || props.disabled) return
  const { from, to } = view.state.selection.main
  const first = view.state.doc.lineAt(from).number; const last = view.state.doc.lineAt(to).number
  const changes = []
  for (let number = first; number <= last; number++) {
    const line = view.state.doc.line(number)
    const existing = prefix.startsWith('#') ? /^ {0,3}#{1,6} /.exec(line.text)?.[0] || '' : line.text.startsWith(prefix) ? prefix : ''
    changes.push({ from: line.from, to: line.from + existing.length, insert: existing === prefix ? '' : prefix })
  }
  view.dispatch({ changes, scrollIntoView: true }); view.focus()
}
function historyAction(action: typeof undo) { if (view && !props.disabled) { action(view); view.focus() } }
watch(() => props.disabled, value => {
  if (!view) return
  if (value) resumeFocus = view.hasFocus
  view.dispatch({ effects: editable.reconfigure([EditorState.readOnly.of(!!value), EditorView.editable.of(!value)]) })
  if (!value && resumeFocus && (document.activeElement === document.body || document.activeElement === view.contentDOM)) view.focus()
})
watch(() => props.modelValue, value => {
  if (!view || value === getValue()) return
  view.setState(createState(value))
  if (showSource.value) view.dispatch({ effects: sourceMode.of(true) })
  outline.value = headings(view.state)
})
watch(outlineOpen, () => { void nextTick(() => view?.requestMeasure()) })
onMounted(() => {
  try {
    view = new EditorView({ state: createState(props.modelValue), parent: host.value })
    if (showSource.value) view.dispatch({ effects: sourceMode.of(true) })
    outline.value = headings(view.state); ready.value = true
  } catch (error) { emit('error', (error as Error).message) }
})
onBeforeUnmount(() => { view?.destroy(); if (fullscreen.value) document.body.style.overflow = previousOverflow })
</script>

<template>
  <div class="markdown-editor source-editor" :class="{ 'show-outline': outlineOpen, 'is-fullscreen': fullscreen, 'show-source': showSource, 'source-only': sourceOnly }">
    <div v-if="!sourceOnly" class="markdown-editor-caption"><button class="text-button" :aria-expanded="outlineOpen" @click="outlineOpen = !outlineOpen">☰ 章节导航</button><span>Ctrl / ⌘ S 保存</span><div class="markdown-mode" role="group" aria-label="编辑模式"><button :aria-pressed="!showSource" @click="switchMode(false)">即时预览</button><button :aria-pressed="showSource" @click="switchMode(true)">源码</button></div><button class="text-button" aria-label="全屏" :aria-pressed="fullscreen" @click="toggleFullscreen">{{ fullscreen ? '退出全屏' : '全屏' }}</button></div>
    <div v-if="!sourceOnly" class="markdown-format-toolbar" role="toolbar" aria-label="Markdown 格式">
      <button aria-label="二级标题" title="二级标题" :disabled="disabled" @click="prefixLines('## ')">H₂</button><button aria-label="粗体" title="粗体" :disabled="disabled" @click="wrap('**')"><b>B</b></button><button aria-label="斜体" title="斜体" :disabled="disabled" @click="wrap('*')"><i>I</i></button><button aria-label="删除线" title="删除线" :disabled="disabled" @click="wrap('~~')"><s>S</s></button><span class="markdown-toolbar-separator" /><button aria-label="无序列表" title="无序列表" :disabled="disabled" @click="prefixLines('- ')">☷</button><button aria-label="有序列表" title="有序列表" :disabled="disabled" @click="prefixLines('1. ')">1.</button><button aria-label="任务列表" title="任务列表" :disabled="disabled" @click="prefixLines('- [ ] ')">☑</button><button aria-label="引用" title="引用" :disabled="disabled" @click="prefixLines('> ')">❯</button><span class="markdown-toolbar-separator" /><button aria-label="链接" title="链接" :disabled="disabled" @click="wrap('[', '](https://)', '链接文字')">↗</button><button aria-label="插入表格" title="插入表格" :disabled="disabled" @click="wrap('\n| 列一 | 列二 |\n| --- | --- |\n| ', ' |  |\n', '内容')">▦</button><button aria-label="代码块" title="代码块" :disabled="disabled" @click="wrap('\n```\n', '\n```\n', '代码')">&lt;/&gt;</button><span class="markdown-toolbar-separator" /><button aria-label="撤销" title="撤销" :disabled="disabled" @click="historyAction(undo)">↶</button><button aria-label="重做" title="重做" :disabled="disabled" @click="historyAction(redo)">↷</button>
    </div>
    <div class="markdown-editor-main" :aria-busy="!ready">
      <nav v-show="outlineOpen" class="markdown-outline" aria-label="章节导航"><div class="markdown-outline-title">大纲</div><button v-for="(heading, index) in outline" :key="index" :style="{ paddingLeft: `${.9 + (heading.level - 1) * .65}rem` }" @click="jump(heading)">{{ heading.text }}</button><p v-if="!outline.length">添加标题后将在这里显示章节</p></nav>
      <div ref="host" class="markdown-editor-host" />
    </div>
  </div>
</template>
