import { onMounted, onUnmounted } from 'vue'

export function useEvents(sync: () => void) {
  let source: EventSource | undefined
  let timer: ReturnType<typeof setTimeout> | undefined
  const schedule = () => { clearTimeout(timer); timer = setTimeout(sync, 150) }
  onMounted(() => {
    source = new EventSource('/api/events')
    source.onopen = schedule
    source.addEventListener('change', schedule)
  })
  onUnmounted(() => { clearTimeout(timer); source?.close() })
}
