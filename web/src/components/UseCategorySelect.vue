<script setup lang="ts">
import { nextTick, onMounted, ref } from 'vue'
import { api } from '../api'
import { useEvents } from '../events'
const value = defineModel<string>({ required: true })
const options = ref<string[]>(['主动', '被动']); const adding = ref(false); const name = ref(''); const error = ref(''); const busy = ref(false); const input = ref<HTMLInputElement>()
async function load() { try { options.value = (await api<{ items: string[] }>('/tool-categories')).items } catch (e) { error.value = (e as Error).message } }
async function change(event: Event) { const selected = (event.target as HTMLSelectElement).value; if (selected === '+') { adding.value = true; name.value = ''; await nextTick(); input.value?.focus() } else value.value = selected }
async function add() { if (!name.value.trim() || busy.value) return; busy.value = true; error.value = ''; try { const result = await api<{ items: string[] }>('/tool-categories', { method: 'POST', body: JSON.stringify({ name: name.value.trim() }) }); options.value = result.items; value.value = result.items.find(item => item.toLocaleLowerCase() === name.value.trim().toLocaleLowerCase()) || name.value.trim(); adding.value = false } catch (e) { error.value = (e as Error).message } finally { busy.value = false } }
onMounted(load); useEvents(load)
</script>
<template><div><div v-if="adding" class="inline-controls"><input ref="input" v-model="name" aria-label="新使用类别" placeholder="输入新的使用类别" maxlength="80" @keydown.enter.stop.prevent="add" @keydown.esc.stop.prevent="adding = false" /><button type="button" class="secondary small" :disabled="busy || !name.trim()" @click="add">确认</button><button type="button" class="text-button" :disabled="busy" @click="adding = false">取消</button></div><select v-else :value="value" aria-label="使用类别" @change="change"><option value="">待定</option><option v-if="value && !options.includes(value)" :value="value">{{ value }}</option><option v-for="item in options" :key="item" :value="item">{{ item }}</option><option value="+">＋ 新增类别</option></select><p v-if="error" class="error-text" role="alert">{{ error }}</p></div></template>
