import { reactive } from 'vue'
import type { User } from './types'
import { bindOfflineUser, clearOffline, unbindOfflineUser } from './offline'

export const auth = reactive({ user: null as User | null, csrf: '', ready: false })
let initializing: Promise<void> | null = null
let generation = 0

export class ApiError extends Error {
  status: number
  code: string
  details: any
  constructor(status: number, code: string, message: string, details?: any) { super(message); this.status = status; this.code = code; this.details = details }
}

type RequestOptions = RequestInit & { accountID?: string }
export async function api<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { accountID, ...requestOptions } = options
  const requestUserID = auth.user?.id
  const requestGeneration = generation
  if (accountID && accountID !== requestUserID) throw new ApiError(409, 'account_changed', '账号已变化，已停止同步')
  const method = (requestOptions.method || 'GET').toUpperCase()
  const headers = new Headers(requestOptions.headers)
  const publicAuth = ['/auth/login', '/auth/register', '/auth/reset-password'].includes(path)
  if (requestOptions.body && !(requestOptions.body instanceof FormData) && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json')
  if (!publicAuth && requestUserID) headers.set('X-User-ID', requestUserID)
  if (!['GET', 'HEAD', 'OPTIONS'].includes(method) && auth.csrf && !publicAuth) headers.set('X-CSRF-Token', auth.csrf)
  const response = await fetch(`/api${path}`, { ...requestOptions, headers, credentials: 'same-origin' })
  const type = response.headers.get('content-type') || ''
  const data = type.includes('application/json') ? await response.json() : null
  if (!response.ok) {
    const error = data?.error || {}
    if (!publicAuth && requestUserID && requestGeneration === generation && (response.status === 401 || error.code === 'account_changed')) {
      await clearAuth()
      window.dispatchEvent(new Event('auth:expired'))
    }
    throw new ApiError(response.status, error.code || 'request_failed', error.message || `请求失败 (${response.status})`, error.details)
  }
  if (!publicAuth && requestUserID && requestGeneration !== generation) throw new ApiError(409, 'account_changed', '账号已变化，已停止同步')
  return data as T
}

export async function initializeAuth(force = false) {
  if (initializing && !force) return initializing
  const requestGeneration = generation
  initializing = (async () => {
    try { const result = await api<{ user: User; csrf_token: string }>('/auth/me'); if (requestGeneration === generation) setAuth(result.user, result.csrf_token) }
    catch { if (force && requestGeneration === generation) await clearAuth() }
    finally { auth.ready = true }
  })()
  return initializing
}
export function setAuth(user: User, csrf: string) {
  if (auth.user?.id !== user.id || auth.csrf !== csrf) { generation++; bindOfflineUser(user.id) }
  auth.user = user; auth.csrf = csrf; auth.ready = true
  try { localStorage.setItem('com-account-change', JSON.stringify({ user_id: user.id, nonce: crypto.randomUUID() })) } catch {}
}
export async function clearAuth(discardLocal = false, userID = auth.user?.id) {
  if (auth.user && userID && auth.user.id !== userID) {
    if (discardLocal) await clearOffline(userID)
    return
  }
  generation++; unbindOfflineUser()
  auth.user = null; auth.csrf = ''; initializing = null
  if (discardLocal) { try { localStorage.setItem('com-account-change', JSON.stringify({ user_id: null, nonce: crypto.randomUUID() })) } catch {} }
  if (discardLocal && userID) {
    try { await clearOffline(userID) }
    catch (error) { console.error('本机草稿清理失败', error); window.dispatchEvent(new CustomEvent('toast', { detail: '已退出，本机草稿清理失败；其他账号不会读取这些内容' })) }
  }
}
window.addEventListener('storage', event => {
  if (event.key !== 'com-account-change' || !event.newValue || !auth.user) return
  try {
    const changed = JSON.parse(event.newValue)
    if (changed.user_id !== auth.user.id) { void clearAuth(); window.dispatchEvent(new Event('auth:expired')) }
  } catch {}
})
export const jsonBody = (value: unknown): RequestInit => ({ method: 'POST', body: JSON.stringify(value) })
