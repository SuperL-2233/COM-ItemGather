<script setup lang="ts">
import { ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { api, auth } from '../api'

const route = useRoute()
const loading = ref(false)
const busy = ref(false)
const error = ref('')
const request = ref<{ id: string; client_name: string; client_id: string; redirect_uri: string }>()
let generation = 0

watch(() => route.query.request, async value => {
  const current = ++generation
  loading.value = false
  request.value = undefined
  error.value = ''
  if (typeof value !== 'string' || !/^[A-Za-z0-9_-]{43}$/.test(value)) {
    error.value = '授权请求无效，请从 AI 客户端重新连接。'
    return
  }
  loading.value = true
  try {
    const result = await api<{ client_name: string; client_id: string; redirect_uri: string }>(`/mcp/oauth/requests/${value}`)
    if (current === generation) request.value = { ...result, id: value }
  } catch (e) { if (current === generation) error.value = (e as Error).message }
  finally { if (current === generation) loading.value = false }
}, { immediate: true })

async function decide(allow: boolean) {
  const pending = request.value
  if (!pending || busy.value) return
  busy.value = true
  error.value = ''
  try {
    const result = await api<{ redirect_uri: string }>(`/mcp/oauth/requests/${pending.id}`, { method: 'POST', body: JSON.stringify({ allow }) })
    window.location.assign(result.redirect_uri)
  } catch (e) { error.value = (e as Error).message; busy.value = false }
}
</script>

<template>
  <section class="mcp-authorize">
    <div class="page-heading"><div><h1>授权创意库只读访问</h1><p>已登录为 {{ auth.user?.nickname }}。</p></div></div>
    <div class="panel" :aria-busy="loading || busy">
      <p v-if="loading" class="muted">正在核对连接请求…</p>
      <p v-if="error" class="error-text" role="alert">{{ error }}</p>
      <template v-if="request">
        <h2>{{ request.client_name }}</h2>
        <p>此客户端请求读取团队共享的 Call of Mommy 创意库。</p>
        <ul class="mcp-permissions">
          <li>搜索、浏览灵感、公司、商品、道具、其他物品和配方。</li>
          <li>读取正文、设定属性、制作状态和关联关系。</li>
          <li>查看附件文件信息。</li>
        </ul>
        <p class="muted">此授权不能修改内容，也不能读取账号资料、回收站、备份或附件原图。授权最长有效 30 天，可在“设置 → MCP 只读接入”随时撤销。</p>
        <dl class="mcp-client-details">
          <dt>客户端标识</dt><dd>{{ request.client_id }}</dd>
          <dt>完成后返回</dt><dd>{{ request.redirect_uri }}</dd>
        </dl>
        <div class="mcp-consent-actions">
          <button class="primary" :disabled="busy || loading" @click="decide(true)">{{ busy ? '正在处理…' : '允许只读访问' }}</button>
          <button class="secondary" :disabled="busy || loading" @click="decide(false)">拒绝</button>
        </div>
      </template>
      <RouterLink v-else-if="!loading" to="/settings">返回设置</RouterLink>
    </div>
  </section>
</template>

<style scoped>
.mcp-authorize { max-width: 700px; margin: 0 auto; }
.mcp-authorize p, .mcp-permissions { line-height: 1.8; }
.mcp-permissions { padding-left: 1.5rem; }
.mcp-client-details { padding: 1rem; background: var(--bg); border-radius: 8px; }
.mcp-client-details dt { font-weight: 600; font-size: .85rem; margin-top: .7rem; }
.mcp-client-details dt:first-child { margin-top: 0; }
.mcp-client-details dd { margin: .3rem 0 0; overflow-wrap: anywhere; font-size: .85rem; }
.mcp-consent-actions { display: flex; flex-wrap: wrap; gap: .75rem; margin-top: 1.5rem; }
</style>
