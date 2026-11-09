<script setup lang="ts">
import { ref } from 'vue'
import IssueMarkdown from './IssueMarkdown.vue'
defineProps<{ modelValue: string; label: string; disabled?: boolean; placeholder?: string }>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
const preview = ref(false)
</script>

<template>
  <div class="issue-composer">
    <div class="issue-composer-tabs" role="group" :aria-label="`${label}模式`"><button :aria-pressed="!preview" @click="preview = false">编写</button><button :aria-pressed="preview" @click="preview = true">预览</button><span>支持 Markdown</span></div>
    <textarea v-show="!preview" :aria-label="label" :value="modelValue" :disabled="disabled" :placeholder="placeholder" maxlength="60000" spellcheck="false" @input="emit('update:modelValue', ($event.target as HTMLTextAreaElement).value)" />
    <div v-if="preview" class="issue-composer-preview" :aria-label="`${label}预览`"><IssueMarkdown :content="modelValue" /></div>
  </div>
</template>
