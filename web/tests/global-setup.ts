import { expect, request, type FullConfig } from '@playwright/test'

export default async function globalSetup(config: FullConfig) {
  const baseURL = config.projects[0].use.baseURL!
  const client = await request.newContext({ baseURL })
  const response = await client.post('/api/auth/login', { data: { username: 'admin', password: 'e2e-admin-password', remember: true, device_name: 'E2E 初始化' } })
  expect(response.ok()).toBeTruthy()
  const { csrf_token: csrf } = await response.json()
  const invitation = await client.post('/api/admin/invitations', { headers: { 'X-CSRF-Token': csrf }, data: {} })
  expect(invitation.status()).toBe(201)
  const { url } = await invitation.json()
  const token = new URL(url).searchParams.get('token')
  const registered = await client.post('/api/auth/register', { data: { token, username: 'member', nickname: '第二位成员', password: 'e2e-member-password', remember: true } })
  expect(registered.status()).toBe(201)
  await client.dispose()
}
