import { reactive } from 'vue'
import type { User } from './types'

export const auth = reactive({ user: null as User | null, csrf: '', ready: false })
let initializing: Promise<void> | null = null

export class ApiError extends Error {
  status: number
  code: string
  details: any
  constructor(status: number, code: string, message: string, details?: any) { super(message); this.status = status; this.code = code; this.details = details }
}

export async function api<T>(path: string, options: RequestInit = {}): Promise<T> {
  const method = (options.method || 'GET').toUpperCase()
  const headers = new Headers(options.headers)
  if (options.body && !(options.body instanceof FormData) && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json')
  if (!['GET', 'HEAD', 'OPTIONS'].includes(method) && auth.csrf && !path.startsWith('/auth/login') && !path.startsWith('/auth/register') && !path.startsWith('/auth/reset-password')) headers.set('X-CSRF-Token', auth.csrf)
  const response = await fetch(`/api${path}`, { ...options, headers, credentials: 'same-origin' })
  const type = response.headers.get('content-type') || ''
  const data = type.includes('application/json') ? await response.json() : null
  if (!response.ok) {
    const error = data?.error || {}
    if (response.status === 401 && path !== '/auth/me' && path !== '/auth/login') { auth.user = null; auth.csrf = ''; window.dispatchEvent(new Event('auth:expired')) }
    throw new ApiError(response.status, error.code || 'request_failed', error.message || `请求失败 (${response.status})`, error.details)
  }
  return data as T
}

export async function initializeAuth(force = false) {
  if (initializing && !force) return initializing
  initializing = (async () => {
    try { const result = await api<{ user: User; csrf_token: string }>('/auth/me'); auth.user = result.user; auth.csrf = result.csrf_token }
    catch { if (force) { auth.user = null; auth.csrf = '' } }
    finally { auth.ready = true }
  })()
  return initializing
}
export function setAuth(user: User, csrf: string) { auth.user = user; auth.csrf = csrf; auth.ready = true }
export function clearAuth() { auth.user = null; auth.csrf = '' }
export const jsonBody = (value: unknown): RequestInit => ({ method: 'POST', body: JSON.stringify(value) })
