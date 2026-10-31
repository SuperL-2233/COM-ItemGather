<script setup lang="ts">
import { ref } from 'vue'
import { api } from '../api'
import type { Idea } from '../types'
import { ideaTitle, shortDate, aiLabels } from '../utils'
const props = defineProps<{ idea: Idea }>()
const emit = defineEmits<{ changed: [] }>()
const busy = ref(false)
async function favorite() { busy.value = true; try { await api(`/ideas/${props.idea.id}/favorite`, { method: 'POST', body: JSON.stringify({ favorite: !props.idea.favorite }) }); emit('changed') } finally { busy.value = false } }
</script>
<template>
  <article class="idea-card" :class="{ 'ai-failed': idea.ai_status === 'failed' }">
    <div class="card-main"><RouterLink :to="`/ideas/${idea.id}`" class="card-title">{{ ideaTitle(idea) }}</RouterLink><p v-if="idea.summary && idea.summary !== ideaTitle(idea)" class="card-summary">{{ idea.summary }}</p><div v-if="idea.tags.length || idea.product_directions.length" class="tag-row"><span v-for="direction in idea.product_directions" :key="`d-${direction}`" class="tag direction">{{ direction }}</span><span v-for="tag in idea.tags" :key="tag" class="tag">{{ tag }}</span></div><div class="card-meta"><span>{{ idea.author_nickname }}</span><time :datetime="idea.created_at">{{ shortDate(idea.created_at) }}</time><span v-if="idea.product_count">{{ idea.product_count }} 个关联商品</span><RouterLink v-if="idea.ai_status === 'failed'" :to="`/ideas/${idea.id}`" class="error-text">{{ aiLabels.failed }}</RouterLink><span v-else-if="['pending','running'].includes(idea.ai_status)" class="muted">{{ aiLabels[idea.ai_status] }}…</span></div></div><button class="favorite-button" :class="{ active: idea.favorite }" :aria-label="idea.favorite ? '取消收藏' : '收藏'" :disabled="busy" @click="favorite">{{ idea.favorite ? '★' : '☆' }}</button>
  </article>
</template>
