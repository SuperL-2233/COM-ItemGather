import { onMounted, onUnmounted } from 'vue'
import { api } from './api'

// One connection per tab: a recipe page can contain many independently edited cards.
const listeners = new Set<() => void>()
let source: EventSource | undefined
let timer: ReturnType<typeof setTimeout> | undefined
function schedule() { clearTimeout(timer); timer = setTimeout(() => { for (const listener of listeners) listener() }, 150) }
export function useEvents(sync: () => void) {
  onMounted(() => {
    listeners.add(sync)
    if (!source) {
      source = new EventSource('/api/events')
      source.onopen = schedule
      source.addEventListener('change', schedule)
      source.onerror = () => { void api('/auth/me').catch(() => {}) }
    }
  })
  onUnmounted(() => {
    listeners.delete(sync)
    if (!listeners.size) { clearTimeout(timer); source?.close(); source = undefined }
  })
}
