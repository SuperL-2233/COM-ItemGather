<script setup lang="ts">
import { computed } from 'vue'
import { Marked } from 'marked'
import DOMPurify from 'dompurify'

const props = defineProps<{ content: string }>()
const markdown = new Marked({ gfm: true, breaks: true, async: false })
markdown.use({ renderer: { checkbox: ({ checked }) => `<span class="issue-task-marker" role="checkbox" aria-checked="${checked}" aria-disabled="true" aria-label="${checked ? '已完成' : '未完成'}">${checked ? '☑' : '☐'}</span> ` } })
const html = computed(() => {
  const element = document.createElement('div')
  element.innerHTML = DOMPurify.sanitize(markdown.parse(props.content) as string, {
    FORBID_TAGS: ['style', 'iframe', 'object', 'embed', 'form', 'input', 'textarea', 'select', 'button'], FORBID_ATTR: ['style'],
  })
  element.querySelectorAll('a').forEach(link => { link.target = '_blank'; link.rel = 'noopener noreferrer' })
  return element.innerHTML
})
</script>

<template><div v-if="content" class="issue-markdown" v-html="html" /><p v-else class="muted issue-no-description">暂无说明</p></template>

<style>
.issue-markdown{font-size:.95rem;line-height:1.85;overflow-wrap:anywhere;min-width:0}
.issue-markdown>:last-child{margin-bottom:0}
.issue-markdown h1{font-size:1.5rem}.issue-markdown h2{font-size:1.25rem}.issue-markdown h3{font-size:1.1rem}
.issue-markdown pre{max-height:450px;white-space:pre;overflow:auto;font-size:.83rem;line-height:1.65}
.issue-markdown code{font-family:ui-monospace,SFMono-Regular,Consolas,monospace;font-size:.88em}
.issue-markdown :not(pre)>code{padding:.15rem .3rem;background:var(--surface-soft);border-radius:4px}
.issue-markdown blockquote{border-left:3px solid var(--border);margin:1rem 0;padding:.3rem 1rem;color:var(--muted)}
.issue-markdown table{display:block;max-width:100%;overflow:auto;border-collapse:collapse;margin:1rem 0}
.issue-markdown th,.issue-markdown td{border:1px solid var(--border);padding:.4rem .7rem}
.issue-markdown th{background:var(--surface-soft)}.issue-markdown img{max-width:100%;height:auto}
.issue-markdown hr{border:0;border-top:1px solid var(--border);margin:1rem 0}
.issue-markdown ul,.issue-markdown ol{padding-left:1.5rem}.issue-no-description{font-size:.9rem;margin:0}
.issue-task-marker{display:inline-block;color:var(--accent);font-size:1.05em;line-height:1}
</style>
