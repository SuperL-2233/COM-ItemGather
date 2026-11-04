import { onUnmounted, ref } from 'vue'

// Cancel as soon as the input changes so a previous response cannot replace
// results during the debounce interval. Keep the displayed results until success.
export function useLiveSearch<T>(fetchResults: (signal: AbortSignal) => Promise<T>, apply: (result: T) => void) {
  const loading = ref(false)
  const error = ref('')
  let controller: AbortController | undefined
  let timer: ReturnType<typeof setTimeout> | undefined
  let version = 0
  let disposed = false

  function cancel() {
    version++
    clearTimeout(timer)
    timer = undefined
    controller?.abort()
    loading.value = false
  }
  async function run() {
    cancel()
    if (disposed) return
    const current = version
    controller = new AbortController()
    loading.value = true
    error.value = ''
    try {
      const result = await fetchResults(controller.signal)
      if (!disposed && current === version) apply(result)
    } catch (cause) {
      if (!disposed && current === version && (cause as Error).name !== 'AbortError') error.value = (cause as Error).message
    } finally {
      if (current === version) loading.value = false
    }
  }
  function schedule() { cancel(); loading.value = true; error.value = ''; timer = setTimeout(() => void run(), 250) }
  // A remote update must not submit a half-typed query or bypass its debounce.
  function refresh() { if (timer !== undefined) return false; schedule(); return true }
  onUnmounted(() => { disposed = true; cancel() })
  return { loading, error, run, schedule, refresh, cancel }
}
