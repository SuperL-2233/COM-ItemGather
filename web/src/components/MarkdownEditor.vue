<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import Vditor from 'vditor'
import DOMPurify from 'dompurify'
import vditorPackage from 'vditor/package.json'
import 'vditor/dist/index.css'

const props = defineProps<{ modelValue: string; disabled?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [value: string]; save: []; error: [message: string] }>()
const host = ref<HTMLDivElement>()
const ready = ref(false)
const outlineOpen = ref(window.innerWidth >= 760)
const cdn = `/vendor/vditor-${vditorPackage.version}`
let editor: Vditor | undefined
let disposed = false
let edited = false
let updating = false
let observer: MutationObserver | undefined
let outlineObserver: MutationObserver | undefined

function loadIcons() {
  return new Promise<void>((resolve, reject) => {
    if (document.getElementById('vditorIconScript')) { resolve(); return }
    const script = document.createElement('script')
    script.src = `${cdn}/dist/js/icons/ant.js`
    script.onload = () => { script.id = 'vditorIconScript'; resolve() }
    script.onerror = () => { script.remove(); reject(new Error('编辑器资源加载失败，请刷新重试')) }
    document.head.appendChild(script)
  })
}
function applyDisabled() {
  if (!ready.value || !editor) return
  if (props.disabled) editor.disabled(); else editor.enable()
}
function getValue() { return ready.value && edited && editor ? editor.getValue() : props.modelValue }
function recordEdit(event: Event) {
  if (event.isTrusted && !updating && !props.disabled) edited = true
}
function shortcut(event: KeyboardEvent) {
  if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 's') { event.preventDefault(); emit('save') }
}
defineExpose({ getValue })

watch(() => props.disabled, applyDisabled)
watch(() => props.modelValue, value => {
  if (!ready.value || !editor || editor.getValue() === value) return
  updating = true
  editor.setValue(value, true)
  edited = false
  updating = false
})
onMounted(async () => {
  try {
    await loadIcons()
    if (disposed || !host.value) return
    const dark = document.documentElement.dataset.theme === 'dark'
    editor = new Vditor(host.value, {
      value: props.modelValue, mode: 'ir', lang: 'zh_CN', cdn,
      theme: dark ? 'dark' : 'classic', height: 'min(76dvh, 950px)', minHeight: 420,
      placeholder: '从 GitHub 读取 README，或在这里开始编写…',
      cache: { enable: false }, outline: { enable: true, position: 'left' },
      undoDelay: 150,
      toolbar: ['headings', 'bold', 'italic', 'strike', '|', 'list', 'ordered-list', 'check', 'quote', '|', 'link', 'table', 'code', '|', 'undo', 'redo', '|', 'edit-mode', 'fullscreen'],
      toolbarConfig: { pin: false },
      preview: {
        delay: 150, maxWidth: 900,
        markdown: { sanitize: true, autoSpace: false, fixTermTypo: false },
        theme: { current: dark ? 'dark' : 'light', path: `${cdn}/dist/css/content-theme` },
        transform: html => DOMPurify.sanitize(html, { FORBID_TAGS: ['style', 'iframe', 'object', 'embed'], FORBID_ATTR: ['style'] }),
      },
      after: () => {
        if (disposed) { editor?.destroy(); return }
        ready.value = true
        host.value?.querySelectorAll('[contenteditable], textarea.vditor-sv').forEach(element => {
          element.setAttribute('role', 'textbox'); element.setAttribute('aria-label', 'README Markdown 编辑器'); element.setAttribute('aria-multiline', 'true')
        })
        const outline = host.value?.querySelector('.vditor-outline')
        outline?.setAttribute('role', 'navigation'); outline?.setAttribute('aria-label', '章节导航')
        if (outline) {
          const decorate = () => outline.querySelectorAll('[data-target-id]').forEach(item => { item.setAttribute('role', 'button'); item.setAttribute('tabindex', '0') })
          decorate()
          outlineObserver = new MutationObserver(decorate)
          outlineObserver.observe(outline, { childList: true, subtree: true })
          outline.addEventListener('keydown', event => {
            const key = (event as KeyboardEvent).key
            if (key === 'Enter' || key === ' ') { event.preventDefault(); (event.target as HTMLElement).closest<HTMLElement>('[data-target-id]')?.click() }
          })
        }
        applyDisabled()
      },
      input: value => { if (!disposed && !updating) { edited = true; emit('update:modelValue', value) } },
    })
    host.value.addEventListener('input', recordEdit, true)
    host.value.addEventListener('keydown', shortcut)
    observer = new MutationObserver(() => {
      const isDark = document.documentElement.dataset.theme === 'dark'
      if (ready.value) editor?.setTheme(isDark ? 'dark' : 'classic', isDark ? 'dark' : 'light', isDark ? 'github-dark' : 'github', `${cdn}/dist/css/content-theme`)
    })
    observer.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] })
  } catch (error) { emit('error', (error as Error).message) }
})
onBeforeUnmount(() => {
  disposed = true; observer?.disconnect(); outlineObserver?.disconnect()
  host.value?.removeEventListener('input', recordEdit, true)
  host.value?.removeEventListener('keydown', shortcut)
  if (ready.value) editor?.destroy()
})
</script>

<template>
  <div class="markdown-editor" :class="{ 'show-outline': outlineOpen }">
    <div class="markdown-editor-caption"><button class="text-button" :aria-expanded="outlineOpen" @click="outlineOpen = !outlineOpen">☰ 章节导航</button><span>直接在正文中编辑 · Ctrl / ⌘ S 保存</span></div>
    <div ref="host" :aria-busy="!ready" />
    <p v-if="!ready" class="muted editor-loading">正在加载编辑器…</p>
  </div>
</template>
