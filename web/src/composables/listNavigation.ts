import { computed, watch } from 'vue'
import { onBeforeRouteLeave, useRoute } from 'vue-router'
import { auth } from '../api'
import type { Page } from '../types'

interface Position { left: number; top: number }
interface ListVisit { userID: string; location: string; entry: number; state: unknown; position: Position }
const visits = new Map<string, ListVisit>()
const origins = new Map<string, { path: string; location: string }>()
const entryNumber = () => Number(window.history.state?.position || 0)
const visitKey = (userID: string, location: string, entry: number) => JSON.stringify([userID, location, entry])
const originKey = (userID: string, location: string) => JSON.stringify([userID, location])
let cancelScroll: (() => void) | undefined

export function cancelPendingScroll() { cancelScroll?.() }

watch(() => auth.user?.id, () => {
  visits.clear()
  origins.clear()
  cancelPendingScroll()
}, { flush: 'sync' })

function previousVisit(location: string) {
  const userID = auth.user?.id
  if (!userID) return
  return visits.get(visitKey(userID, location, entryNumber()))
    || [...visits.values()].reverse().find(visit => visit.userID === userID && visit.location === location)
}

// Store only list display state, never an editor or the inspiration composer.
// Cached rows restore page height immediately; normal loads still refresh them.
export function useListNavigation<T>(capture: () => T): T | undefined {
  const route = useRoute()
  const userID = auth.user?.id
  let entry = entryNumber()
  const previous = previousVisit(route.fullPath)
  watch(() => route.fullPath, () => { entry = entryNumber() }, { flush: 'post' })
  onBeforeRouteLeave((to, from) => {
    if (!userID || auth.user?.id !== userID) return
    const key = visitKey(userID, from.fullPath, entry)
    visits.delete(key)
    visits.set(key, { userID, location: from.fullPath, entry, state: JSON.parse(JSON.stringify(capture())), position: { left: window.scrollX, top: window.scrollY } })
    origins.set(originKey(userID, to.fullPath), { path: from.path, location: from.fullPath })
    while (visits.size > 20) visits.delete(visits.keys().next().value!)
    while (origins.size > 20) origins.delete(origins.keys().next().value!)
  })
  return previous ? JSON.parse(JSON.stringify(previous.state)) as T : undefined
}

// A detail's library link must retain the company constraint of its source list.
export function useListBackLocation(fallback: string) {
  const route = useRoute()
  return computed(() => {
    const origin = auth.user && origins.get(originKey(auth.user.id, route.fullPath))
    return origin && origin.path === fallback ? origin.location : fallback
  })
}

export function savedListPosition(location: string) { return previousVisit(location)?.position }

// Back/forward can also return to an async detail page. Wait for enough layout
// height, and stop restoring as soon as the user scrolls or navigates elsewhere.
export function restoreScroll(position: Position): Promise<Position | false> {
  cancelPendingScroll()
  if (document.documentElement.scrollHeight >= position.top + window.innerHeight - 1) return Promise.resolve(position)
  return new Promise(resolve => {
    let done = false
    const finish = (value: Position | false) => {
      if (done) return
      done = true
      observer.disconnect()
      clearTimeout(timer)
      window.removeEventListener('wheel', cancel)
      window.removeEventListener('touchstart', cancel)
      window.removeEventListener('keydown', onKey)
      if (cancelScroll === cancel) cancelScroll = undefined
      resolve(value)
    }
    const cancel = () => finish(false)
    const onKey = (event: KeyboardEvent) => { if (['ArrowUp', 'ArrowDown', 'PageUp', 'PageDown', 'Home', 'End', ' '].includes(event.key)) cancel() }
    const observer = new ResizeObserver(() => {
      if (document.documentElement.scrollHeight >= position.top + window.innerHeight - 1) finish(position)
    })
    const timer = setTimeout(() => finish(position), 5000)
    cancelScroll = cancel
    observer.observe(document.body)
    window.addEventListener('wheel', cancel, { passive: true })
    window.addEventListener('touchstart', cancel, { passive: true })
    window.addEventListener('keydown', onKey)
  })
}

// Refresh the whole displayed range atomically, within the API's 100-row limit.
export async function loadListPages<T extends { id: string }>(count: number, fetchPage: (offset: number, limit: number) => Promise<Page<T>>): Promise<Page<T>> {
  const items = new Map<string, T>()
  let offset = 0
  let result: Page<T>
  do {
    result = await fetchPage(offset, Math.min(100, count - offset))
    for (const item of result.items) items.set(item.id, item)
    offset += result.items.length
  } while (result.items.length && offset < Math.min(count, result.total))
  return { ...result, items: [...items.values()], offset: 0 }
}
