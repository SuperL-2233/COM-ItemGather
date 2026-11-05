<script setup lang="ts">
import { ref } from 'vue'
import { api } from '../api'
import type { Idea } from '../types'
import { ideaTitle, shortDate, aiLabels } from '../utils'
const props = defineProps<{ idea: Idea }>()
const emit = defineEmits<{ changed: [] }>()
const busy = ref(false)
const error = ref('')
async function favorite() { busy.value = true; error.value = ''; try { await api(`/ideas/${props.idea.id}/favorite`, { method: 'POST', body: JSON.stringify({ favorite: !props.idea.favorite }) }); emit('changed') } catch (e) { error.value = (e as Error).message } finally { busy.value = false } }
async function retry() { busy.value = true; error.value = ''; try { await api(`/ideas/${props.idea.id}/ai`, { method: 'POST', body: JSON.stringify({ mode: 'normal' }) }); emit('changed') } catch (e) { error.value = (e as Error).message } finally { busy.value = false } }
</script>
<template>
  <article class="idea-card" :class="{ 'ai-failed': idea.ai_status === 'failed' }">
    <RouterLink v-if="idea.attachments.length" :to="`/ideas/${idea.id}`" class="card-preview"><img :src="idea.attachments[0].thumbnail_url || idea.attachments[0].url" :alt="idea.attachments[0].original_name" loading="lazy" /></RouterLink>
    <div class="card-main">
      <RouterLink :to="`/ideas/${idea.id}`" class="card-title">{{ ideaTitle(idea) }}</RouterLink>
      <p v-if="idea.summary && idea.summary !== ideaTitle(idea)" class="card-summary">{{ idea.summary }}</p>
      <div v-if="idea.idea_types.length || idea.product_directions.length" class="tag-row">
        <span v-for="type in idea.idea_types" :key="`t-${type}`" class="tag">{{ type }}</span>
        <span v-for="direction in idea.product_directions" :key="`d-${direction}`" class="tag direction">{{ direction }}</span>
      </div>
      <div class="card-meta"><span>{{ idea.author_nickname }}</span><time :datetime="idea.created_at">{{ shortDate(idea.created_at) }}</time><span v-if="idea.product_count">{{ idea.product_count }} 件关联物品</span><template v-if="idea.ai_status === 'failed'"><span class="error-text">{{ aiLabels.failed }}</span><button class="button-link error-text" :disabled="busy" @click="retry">重新整理</button></template><span v-else-if="['pending','running'].includes(idea.ai_status)" class="muted">{{ aiLabels[idea.ai_status] }}…</span></div>
      <p v-if="error" class="error-text">{{ error }}</p>
    </div>
    <button class="favorite-button" :class="{ active: idea.favorite }" :aria-label="idea.favorite ? '取消收藏' : '收藏'" :disabled="busy" @click="favorite">{{ idea.favorite ? '★' : '☆' }}</button>
  </article>
</template>
<style scoped>
.card-preview { flex: 0 0 88px; align-self: flex-start; }
.card-preview img { width: 88px; height: 88px; object-fit: cover; border-radius: 6px; }
@media (max-width: 480px) { .card-preview { flex-basis: 64px; } .card-preview img { width: 64px; height: 64px; } }
</style>
