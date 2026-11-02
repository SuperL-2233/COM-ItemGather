<script setup lang="ts">
import { computed, onMounted, ref, useId } from 'vue'
import { api, ApiError } from '../api'
import type { ChoiceOption, StatusDeletion, StatusUsage } from '../types'
import SearchCreateSelect from './SearchCreateSelect.vue'

const props = defineProps<{ option: ChoiceOption; options: ChoiceOption[]; create: (name: string) => Promise<ChoiceOption> }>()
const emit = defineEmits<{ deleted: [result: StatusDeletion]; cancel: [] }>()
const id = useId()
const usage = ref<StatusUsage>()
const cancelButton = ref<HTMLButtonElement>()
const loading = ref(true)
const busy = ref(false)
const error = ref('')
const replacement = ref('')
const choices = computed(() => props.options.filter(option => option.value !== props.option.value))
const hasReferences = computed(() => (usage.value?.item_count || 0) + (usage.value?.recipe_count || 0) > 0)
const canDelete = computed(() => !!usage.value && !loading.value && !busy.value && choices.value.length > 0 && (!hasReferences.value || choices.value.some(option => option.value === replacement.value)))
async function load() {
  loading.value = true
  error.value = ''
  try { usage.value = await api<StatusUsage>(`/product-options/statuses/usage?${new URLSearchParams({ id: props.option.value })}`) }
  catch (e) { usage.value = undefined; error.value = (e as Error).message }
  finally { loading.value = false }
}
async function remove() {
  if (!canDelete.value || !usage.value) return
  busy.value = true
  error.value = ''
  try {
    emit('deleted', await api<StatusDeletion>('/product-options/statuses', { method: 'DELETE', body: JSON.stringify({ id: props.option.value, token: usage.value.token, replacement: replacement.value }) }))
  } catch (e) {
    if (e instanceof ApiError && e.code === 'status_changed' && e.details?.usage) usage.value = e.details.usage
    if (e instanceof ApiError && e.status === 404) usage.value = undefined
    error.value = (e as Error).message
  } finally { busy.value = false }
}
onMounted(() => { void load(); cancelButton.value?.focus() })
</script>

<template>
  <Teleport to="body">
    <div class="modal-backdrop" @keydown.esc.stop.prevent="!busy && emit('cancel')">
      <form class="modal" role="dialog" aria-modal="true" :aria-labelledby="`${id}-title`" @submit.stop.prevent="remove">
        <h2 :id="`${id}-title`">删除制作状态“{{ option.label }}”</h2>
        <p v-if="loading" class="muted">正在检查制作状态使用情况…</p>
        <template v-else-if="usage">
          <p v-if="hasReferences">此状态用于 {{ usage.item_count }} 件商品／道具、{{ usage.recipe_count }} 套配方（包含回收站中的内容）。</p>
          <p v-else>此状态当前未被使用，确认后会从制作状态选项中移除。</p>
          <p v-if="hasReferences" class="muted">请选择替代状态。商品、道具和配方本身都会保留。</p>
          <p v-if="!choices.length" class="error-text">至少保留一个制作状态，请先新建其他状态。</p>
          <div v-if="hasReferences || !choices.length" class="field">
            <SearchCreateSelect v-model="replacement" label="替代状态" :options="choices" :create="create" create-label="＋ 新建制作状态" new-label="新制作状态" empty-label="请选择替代状态" :allow-empty="false" :max-length="80" />
          </div>
          <p v-if="replacement && !choices.some(option => option.value === replacement)" class="error-text">替代状态已被删除或不可用，请重新选择。</p>
        </template>
        <p v-if="error" class="error-text" role="alert">{{ error }} <button v-if="!usage" type="button" class="button-link" @click="load">重试</button></p>
        <div class="modal-actions">
          <button ref="cancelButton" type="button" class="secondary" :disabled="busy" autofocus @click="emit('cancel')">取消</button>
          <button class="danger-button" :disabled="!canDelete">{{ busy ? '删除中…' : '确认删除制作状态' }}</button>
        </div>
      </form>
    </div>
  </Teleport>
</template>
