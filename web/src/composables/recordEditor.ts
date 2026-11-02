import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { onBeforeRouteLeave } from 'vue-router'
import { api, ApiError } from '../api'
import { useEvents } from '../events'
import { toast } from '../utils'

export function useRecordEditor<T extends { version: number }>(endpoint: string, fields: (record: T) => Record<string, unknown>, normalize: (record: T) => T = value => value) {
  const record = ref<T>()
  let requestID = 0
  const loading = ref(true); const saving = ref(false); const error = ref(''); const remoteNotice = ref(false); const saved = ref('')
  const conflict = ref<{ current: T; submitted: Record<string, unknown> }>()
  const payload = computed(() => record.value ? fields(record.value as T) : {})
  const key = () => JSON.stringify(payload.value)
  const dirty = computed(() => !!record.value && key() !== saved.value)
  function apply(value: T) { requestID++; record.value = normalize(value); saved.value = key(); remoteNotice.value = false }
  async function load(force = false) { const request = ++requestID; try { const latest = await api<T>(endpoint); if (request !== requestID || (record.value && latest.version < record.value.version)) return; if (dirty.value && !force) { if (latest.version !== record.value?.version) remoteNotice.value = true } else apply(latest) } catch (e) { error.value = (e as Error).message } finally { loading.value = false } }
  async function save() { if (!record.value || saving.value) return; saving.value = true; error.value = ''; try { apply(await api<T>(endpoint, { method: 'PATCH', body: JSON.stringify({ ...payload.value, version: record.value.version }) })); toast('已保存') } catch (e) { if (e instanceof ApiError && e.status === 409 && e.details?.current) conflict.value = { current: e.details.current, submitted: payload.value }; else error.value = (e as Error).message } finally { saving.value = false } }
  function useServer() { if (conflict.value) apply(conflict.value.current as T); conflict.value = undefined }
  async function mergeLatest() { if (!record.value || !conflict.value) return; record.value.version = conflict.value.current.version; conflict.value = undefined; await save() }
  function beforeUnload(event: BeforeUnloadEvent) { if (dirty.value) { event.preventDefault(); event.returnValue = '' } }
  onBeforeRouteLeave(() => !dirty.value || confirm('还有未保存修改，确定离开？'))
  onMounted(() => { void load(); window.addEventListener('beforeunload', beforeUnload) }); onBeforeUnmount(() => window.removeEventListener('beforeunload', beforeUnload)); useEvents(() => void load())
  return { record, loading, saving, error, remoteNotice, dirty, conflict, apply, load, save, useServer, mergeLatest }
}
