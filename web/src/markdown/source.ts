import { StateEffect, StateField } from '@codemirror/state'
import type { ChangeSet, Extension, Text } from '@codemirror/state'
import { invertedEffects } from '@codemirror/commands'

export const normalizeLineEndings = (source: string) => source.replace(/\r\n?|\n/g, '\n')

/** Map CodeMirror's LF-based positions to the original, unformatted text. */
export class MarkdownSource {
  readonly text: string
  readonly newline: string
  readonly endings: readonly string[]
  private readonly crlf: number[] = []

  constructor(text: string) {
    this.text = text
    this.newline = text.match(/\r\n|\r|\n/)?.[0] || '\n'
    this.endings = text.match(/\r\n|\r|\n/g) || []
    let extra = 0
    for (let index = text.indexOf('\r\n'); index >= 0; index = text.indexOf('\r\n', index + 2)) {
      this.crlf.push(index - extra++)
    }
  }

  offset(position: number) {
    let left = 0; let right = this.crlf.length
    while (left < right) { const middle = (left + right) >>> 1; if (this.crlf[middle] < position) left = middle + 1; else right = middle }
    return position + left
  }

  apply(changes: ChangeSet) {
    const parts: string[] = []
    let cursor = 0
    changes.iterChanges((from, to, _fromNew, _toNew, inserted) => {
      const start = this.offset(from); const end = this.offset(to)
      parts.push(this.text.slice(cursor, start), inserted.toString().replace(/\n/g, this.newline))
      cursor = end
    })
    parts.push(this.text.slice(cursor))
    return new MarkdownSource(parts.join(''))
  }
}

// Undo stores only newline metadata plus CodeMirror's shared text tree, rather
// than retaining a full raw document copy for every keystroke.
const restoreSource = StateEffect.define<{ doc: Text; endings: readonly string[] }>()
export function sourceDocument(initial: string): { field: StateField<MarkdownSource>; extension: Extension } {
  const field = StateField.define<MarkdownSource>({
    create: () => new MarkdownSource(initial),
    update(value, transaction) {
      for (const effect of transaction.effects) {
        if (effect.is(restoreSource) && effect.value.doc.eq(transaction.newDoc)) {
          const lines = transaction.newDoc.toString().split('\n')
          return new MarkdownSource(lines.map((line, index) => line + (effect.value.endings[index] || '')).join(''))
        }
      }
      return transaction.docChanged ? value.apply(transaction.changes) : value
    },
  })
  return { field, extension: [field, invertedEffects.of(transaction => transaction.docChanged ? [restoreSource.of({ doc: transaction.startState.doc, endings: transaction.startState.field(field).endings })] : [])] }
}
