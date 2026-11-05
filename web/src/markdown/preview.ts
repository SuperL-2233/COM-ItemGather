import { StateEffect, StateField } from '@codemirror/state'
import type { EditorState, Range } from '@codemirror/state'
import { Decoration, EditorView, WidgetType } from '@codemirror/view'
import type { DecorationSet } from '@codemirror/view'
import { syntaxTree } from '@codemirror/language'
import { Marked } from 'marked'
import DOMPurify from 'dompurify'

const renderer = new Marked({ gfm: true, breaks: false, async: false })
export const sourceMode = StateEffect.define<boolean>()
export const editorFocus = StateEffect.define<boolean>()
const sourceModeField = StateField.define({ create: () => false, update: (value, transaction) => transaction.effects.reduce((current, effect) => effect.is(sourceMode) ? effect.value : current, value) })
const focusField = StateField.define({ create: () => false, update: (value, transaction) => transaction.effects.reduce((current, effect) => effect.is(editorFocus) ? effect.value : current, value) })

class MarkdownWidget extends WidgetType {
  constructor(readonly source: string, readonly block: boolean) { super() }
  eq(other: MarkdownWidget) { return other.source === this.source && other.block === this.block }
  toDOM(view: EditorView) {
    const element = document.createElement(this.block ? 'div' : 'span')
    element.className = 'markdown-rendered' + (this.block ? ' markdown-block-preview' : '')
    const html = this.block ? renderer.parse(this.source) : renderer.parseInline(this.source)
    element.innerHTML = DOMPurify.sanitize(html as string, { FORBID_TAGS: ['style', 'iframe', 'object', 'embed', 'form'], FORBID_ATTR: ['style'] })
    element.querySelectorAll('a').forEach(link => { link.target = '_blank'; link.rel = 'noopener noreferrer' })
    element.setAttribute('contenteditable', 'false')
    element.setAttribute('tabindex', '0')
    element.setAttribute('aria-label', '编辑这段 Markdown')
    const edit = () => {
      const position = view.posAtDOM(element)
      view.dispatch({ selection: { anchor: position }, effects: editorFocus.of(true), scrollIntoView: true })
      view.focus()
    }
    element.addEventListener('mousedown', event => {
      const mouse = event as MouseEvent
      if ((mouse.ctrlKey || mouse.metaKey) && (event.target as Element).closest('a')) return
      event.preventDefault(); event.stopPropagation(); edit()
    })
    element.addEventListener('keydown', event => { if ((event as KeyboardEvent).key === 'Enter') { event.preventDefault(); edit() } })
    return element
  }
  ignoreEvent() { return true }
}

function decorations(state: EditorState): DecorationSet {
  if (state.field(sourceModeField)) return Decoration.none
  const focused = state.field(focusField)
  const active = state.selection.ranges.map(range => ({ from: state.doc.lineAt(range.from).from, to: state.doc.lineAt(range.to).to }))
  const editing = (from: number, to: number) => focused && active.some(range => range.from <= to && range.to >= from)
  const ranges: Range<Decoration>[] = []
  syntaxTree(state).iterate({
    enter(node) {
      const { name, from, to } = node
      if (['Table', 'FencedCode', 'CodeBlock', 'HTMLBlock', 'HorizontalRule'].includes(name)) {
        const first = state.doc.lineAt(from).from; const last = state.doc.lineAt(to).to
        if (!editing(first, last)) {
          ranges.push(Decoration.replace({ widget: new MarkdownWidget(state.doc.sliceString(first, last), true), block: true }).range(first, last))
          return false
        }
      }
      const heading = /^(?:ATX|Setext)Heading([1-6])$/.exec(name)
      if (heading) ranges.push(Decoration.line({ class: `markdown-heading markdown-h${heading[1]}`, attributes: { role: 'heading', 'aria-level': heading[1] } }).range(state.doc.lineAt(from).from))
      if (editing(from, to)) return
      if (['Image', 'Link', 'Autolink'].includes(name)) {
        ranges.push(Decoration.replace({ widget: new MarkdownWidget(state.doc.sliceString(from, to), false) }).range(from, to))
        return false
      }
      if (['HeaderMark', 'EmphasisMark', 'StrikethroughMark', 'CodeMark'].includes(name)) {
        const end = name === 'HeaderMark' && state.doc.sliceString(to, to + 1) === ' ' ? to + 1 : to
        ranges.push(Decoration.replace({}).range(from, end))
      }
    },
  })
  return Decoration.set(ranges, true)
}

const previewField = StateField.define<DecorationSet>({ create: decorations, update: (_value, transaction) => decorations(transaction.state), provide: field => EditorView.decorations.from(field) })
export const livePreview = [sourceModeField, focusField, previewField]

export interface MarkdownHeading { from: number; level: number; text: string }
export function headings(state: EditorState): MarkdownHeading[] {
  const result: MarkdownHeading[] = []
  let offset = 0
  // A separate read-only parse includes headings outside the editor viewport.
  // No rendered or parsed content is ever written back into the document.
  for (const token of renderer.lexer(state.doc.toString())) {
    if (token.type === 'heading') {
      const element = document.createElement('span')
      element.innerHTML = DOMPurify.sanitize(renderer.parseInline(token.text) as string)
      result.push({ from: offset, level: token.depth, text: element.textContent || token.text })
    }
    offset += token.raw.length
  }
  return result
}
