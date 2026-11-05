<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import type { MCPConnection } from '../types'
import { formatDate, toast } from '../utils'

const serverURL = ref('')
const connections = ref<MCPConnection[]>([])
const loading = ref(true)
const revoking = ref('')
const error = ref('')
const config = computed(() => `[mcp_servers.com_creative_library]\nurl = ${JSON.stringify(serverURL.value)}\n`)

async function load() {
  const result = await api<{ server_url: string; items: MCPConnection[] }>('/mcp/connections')
  serverURL.value = result.server_url
  connections.value = result.items
}
function connectionStatus(item: MCPConnection) {
  if (item.revoked_at) return '已撤销'
  return new Date(item.expires_at).getTime() <= Date.now() ? '已过期' : '有效'
}
async function copy(value: string) {
  try { await navigator.clipboard.writeText(value); toast('已复制') }
  catch { error.value = '无法访问剪贴板，请选中内容手动复制。' }
}
async function revoke(item: MCPConnection) {
  revoking.value = item.id
  error.value = ''
  try {
    await api(`/mcp/connections/${item.id}`, { method: 'DELETE' })
    await load()
    toast('MCP 授权已撤销')
  } catch (e) { error.value = (e as Error).message }
  finally { revoking.value = '' }
}
onMounted(async () => {
  try { await load() }
  catch (e) { error.value = (e as Error).message }
  finally { loading.value = false }
})
</script>

<template>
  <section class="panel mcp-settings" :aria-busy="loading">
    <h2>MCP 只读接入</h2>
    <p class="muted">让 AI 按需搜索和阅读团队创意库。连接时使用本站账号登录，授权后即可读取灵感、公司、商品、道具、其他物品、配方及关联关系。</p>
    <p v-if="error" class="error-text" role="alert">{{ error }}</p>
    <p v-if="loading" class="muted">正在加载接入信息…</p>
    <template v-else-if="serverURL">
      <div class="field">
        <label for="mcp-server-url">MCP 服务器地址</label>
        <input id="mcp-server-url" :value="serverURL" readonly spellcheck="false" />
        <div><button class="secondary" @click="copy(serverURL)">复制 MCP 地址</button></div>
      </div>
      <h3>ChatGPT 网页端</h3>
      <p>在插件中添加自定义 MCP 服务器，填入上面的地址，认证选择 OAuth，再完成本站登录和只读授权。</p>
      <details>
        <summary>本地客户端配置</summary>
        <p class="muted">支持 Streamable HTTP 与 OAuth 的客户端可使用同一地址。Codex 可在 MCP 设置中添加服务器，或将下面的配置加入配置文件后登录授权。</p>
        <pre class="mcp-config">{{ config }}</pre>
        <button class="secondary small" @click="copy(config)">复制 Codex 配置</button>
      </details>
      <p class="muted mcp-scope">权限为创意库只读。图片仅提供文件信息；账号资料、回收站、系统配置和备份不在授权范围内。</p>
      <h3>已授权连接</h3>
      <p class="muted">授权最长有效 30 天，期间客户端自动刷新访问凭据。撤销后，该连接立即失去访问权限。</p>
      <div v-for="item in connections" :key="item.id" class="device-row mcp-connection">
        <div>
          <strong>{{ item.client_name }} · {{ connectionStatus(item) }}</strong>
          <small>授权 {{ formatDate(item.created_at) }} · 到期 {{ formatDate(item.expires_at) }}<span v-if="item.last_used_at"> · 最近使用 {{ formatDate(item.last_used_at) }}</span></small>
        </div>
        <button v-if="connectionStatus(item) === '有效'" class="secondary small" :disabled="!!revoking" @click="revoke(item)">{{ revoking === item.id ? '正在撤销…' : '撤销授权' }}</button>
      </div>
      <p v-if="!connections.length" class="muted">还没有授权过 MCP 连接。先在 AI 客户端添加服务器并完成授权。</p>
    </template>
  </section>
</template>

<style scoped>
.mcp-settings p { line-height: 1.7; }
.mcp-config { overflow-x: auto; padding: 1rem; border-radius: var(--radius-sm, 8px); background: var(--bg); font-size: .85rem; }
.mcp-settings details { margin: 1rem 0; }
.mcp-settings summary { cursor: pointer; font-weight: 600; }
.mcp-scope { margin-top: 1.25rem; }
.mcp-connection > div { min-width: 0; overflow-wrap: anywhere; }
.mcp-connection button { flex-shrink: 0; }
</style>
