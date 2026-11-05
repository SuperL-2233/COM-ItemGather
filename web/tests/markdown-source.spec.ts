import { test, expect } from '@playwright/test'
import { EditorState, Transaction } from '@codemirror/state'
import { history, isolateHistory, redo, undo } from '@codemirror/commands'
import { normalizeLineEndings, sourceDocument } from '../src/markdown/source'

for (const newline of ['\n', '\r\n', '\r']) {
  test(`原文局部编辑保留空行、表格、尾随空格及 ${JSON.stringify(newline)} 换行`, () => {
    const original = ['\ufeff# 标题', '', '', '段落保留尾随空格  ', '', '', '', '| 名称 |价格|', '|:-----|---:|', '|苹果|  10 |', '', '只改这里', '', ''].join(newline)
    const source = sourceDocument(original)
    let state = EditorState.create({ doc: normalizeLineEndings(original), extensions: [source.extension, history()] })
    const from = state.doc.toString().indexOf('只改这里')
    state = state.update({ changes: { from, to: from + 4, insert: '这一处已改' } }).state
    expect(state.field(source.field).text).toBe(original.replace('只改这里', '这一处已改'))
    expect(undo({ state, dispatch: transaction => { state = transaction.state } })).toBeTruthy()
    expect(state.field(source.field).text).toBe(original)
    expect(redo({ state, dispatch: transaction => { state = transaction.state } })).toBeTruthy()
    expect(state.field(source.field).text).toBe(original.replace('只改这里', '这一处已改'))
  })
}

test('混合换行的多处编辑、删除和撤销逐字保留未改内容', () => {
  const original = '# 标题\r\n\r\n\n前段\r\n\n\r\n| A| B  |\n|---|---:|\r\n| a |  b |\n\r\n后段\r'
  const source = sourceDocument(original)
  let state = EditorState.create({ doc: normalizeLineEndings(original), extensions: [source.extension, history()] })
  const text = state.doc.toString()
  state = state.update({ changes: [
    { from: text.indexOf('前段'), to: text.indexOf('前段') + 2, insert: '前段修改' },
    { from: text.indexOf('后段'), to: text.indexOf('后段') + 2, insert: '后段修改' },
  ], annotations: isolateHistory.of('full') }).state
  const modified = original.replace('前段', '前段修改').replace('后段', '后段修改')
  expect(state.field(source.field).text).toBe(modified)
  const from = state.doc.toString().indexOf('修改')
  state = state.update({ changes: { from, to: from + 2 }, annotations: isolateHistory.of('full') }).state
  expect(state.field(source.field).text).toBe(modified.replace('前段修改', '前段'))
  undo({ state, dispatch: transaction => { state = transaction.state } })
  expect(state.field(source.field).text).toBe(modified)
  undo({ state, dispatch: transaction => { state = transaction.state } })
  expect(state.field(source.field).text).toBe(original)
})

test('组合输入的撤销和重做保留混合换行与文件末尾格式', () => {
  const original = '前段\r\n\n\r\n后段'
  const source = sourceDocument(original)
  let state = EditorState.create({ doc: normalizeLineEndings(original), extensions: [source.extension, history()] })
  for (const text of ['新增', '文字']) state = state.update({ changes: { from: state.doc.length, insert: text }, annotations: Transaction.userEvent.of('input.type') }).state
  expect(state.field(source.field).text).toBe(original + '新增文字')
  undo({ state, dispatch: transaction => { state = transaction.state } })
  expect(state.field(source.field).text).toBe(original)
  redo({ state, dispatch: transaction => { state = transaction.state } })
  expect(state.field(source.field).text).toBe(original + '新增文字')
})

test('跨行删除和插入后，撤销重做恢复每一处原始换行', () => {
  const original = '首行\r\n\n中间\r\n\r末行\n\n'
  const source = sourceDocument(original)
  let state = EditorState.create({ doc: normalizeLineEndings(original), extensions: [source.extension, history()] })
  const from = state.doc.line(2).from
  const to = state.doc.line(5).from
  state = state.update({ changes: { from, to, insert: '新段落\n\n' } }).state
  const changed = '首行\r\n新段落\r\n\r\n末行\n\n'
  expect(state.field(source.field).text).toBe(changed)
  undo({ state, dispatch: transaction => { state = transaction.state } })
  expect(state.field(source.field).text).toBe(original)
  redo({ state, dispatch: transaction => { state = transaction.state } })
  expect(state.field(source.field).text).toBe(changed)
})
