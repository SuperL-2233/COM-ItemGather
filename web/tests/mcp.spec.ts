import { createHash, randomBytes } from 'node:crypto'
import { test, expect, type Page } from '@playwright/test'

async function authorizationURL(page: Page, baseURL: string) {
  const registered = await page.request.post('/api/mcp/oauth/register', {
    data: { client_name: '浏览器验收客户端', redirect_uris: ['http://127.0.0.1:17777/callback'], token_endpoint_auth_method: 'none' },
  })
  expect(registered.status()).toBe(201)
  const { client_id: clientID } = await registered.json()
  const verifier = randomBytes(48).toString('base64url')
  const params = new URLSearchParams({
    client_id: clientID, redirect_uri: 'http://127.0.0.1:17777/callback', response_type: 'code',
    scope: 'creative-library:read', resource: `${baseURL}/api/mcp`, state: 'browser-state',
    code_challenge: createHash('sha256').update(verifier).digest('base64url'), code_challenge_method: 'S256',
  })
  await page.route('http://127.0.0.1:17777/callback**', route => route.fulfill({ contentType: 'text/html', body: '<h1>客户端已收到授权结果</h1>' }))
  return { url: `/api/mcp/oauth/authorize?${params}`, clientID, verifier }
}

async function logInForConsent(page: Page) {
  await expect(page).toHaveURL(/\/login\?redirect=/)
  await page.getByLabel('用户名').fill('admin')
  await page.getByLabel('密码', { exact: true }).fill('e2e-admin-password')
  await page.getByRole('button', { name: '登录', exact: true }).click()
  await expect(page).toHaveURL(/\/mcp\/authorize\?request=/)
  await expect(page.getByRole('heading', { name: '浏览器验收客户端' })).toBeVisible()
}

test('MCP 登录授权、读取工具与设置中撤销连接', async ({ page, baseURL }) => {
  const { url, clientID, verifier } = await authorizationURL(page, baseURL!)
  await page.goto(url)
  await logInForConsent(page)
  await expect(page.getByText('此授权不能修改内容', { exact: false })).toBeVisible()
  await page.screenshot({ path: '/tmp/com-mcp-consent-desktop.png', fullPage: true })
  await page.getByRole('button', { name: '允许只读访问' }).click()
  await expect(page).toHaveURL(/127\.0\.0\.1:17777\/callback\?/)
  const callback = new URL(page.url())
  expect(callback.searchParams.get('state')).toBe('browser-state')
  expect(callback.searchParams.get('iss')).toBe(baseURL)
  const tokenResponse = await page.request.post(`${baseURL}/api/mcp/oauth/token`, {
    form: { client_id: clientID, grant_type: 'authorization_code', code: callback.searchParams.get('code')!,
      code_verifier: verifier, redirect_uri: 'http://127.0.0.1:17777/callback', resource: `${baseURL}/api/mcp` },
  })
  expect(tokenResponse.ok()).toBeTruthy()
  const { access_token: accessToken } = await tokenResponse.json()
  const read = () => page.request.post(`${baseURL}/api/mcp`, {
    headers: { Authorization: `Bearer ${accessToken}`, Accept: 'application/json, text/event-stream', 'MCP-Protocol-Version': '2025-11-25' },
    data: { jsonrpc: '2.0', id: 1, method: 'tools/list', params: {} },
  })
  const tools = await read()
  expect(tools.ok()).toBeTruthy()
  expect((await tools.json()).result.tools.map((tool: { name: string }) => tool.name).sort()).toEqual(['fetch', 'get_library_info', 'list_records', 'search'])
  await page.goto(`${baseURL}/settings`)
  await page.getByRole('button', { name: 'MCP 只读接入', exact: true }).click()
  await expect(page.getByLabel('MCP 服务器地址')).toHaveValue(`${baseURL}/api/mcp`)
  await expect(page.locator('.mcp-connection').filter({ hasText: '浏览器验收客户端' })).toContainText('有效')
  await expect(page.getByRole('button', { name: '生成 15 分钟只读 Token' })).toHaveCount(0)
  await page.getByRole('button', { name: '撤销授权', exact: true }).click()
  await expect(page.locator('.mcp-connection').filter({ hasText: '浏览器验收客户端' })).toContainText('已撤销')
  expect((await read()).status()).toBe(401)
  await page.screenshot({ path: '/tmp/com-mcp-settings-desktop.png', fullPage: true })
})

test('手机授权页可拒绝，且不会产生授权码', async ({ page, baseURL }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  const { url } = await authorizationURL(page, baseURL!)
  await page.goto(url)
  await logInForConsent(page)
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  await page.screenshot({ path: '/tmp/com-mcp-consent-mobile.png', fullPage: true })
  await page.getByRole('button', { name: '拒绝', exact: true }).click()
  await expect(page).toHaveURL(/127\.0\.0\.1:17777\/callback\?/)
  const callback = new URL(page.url())
  expect(callback.searchParams.get('error')).toBe('access_denied')
  expect(callback.searchParams.has('code')).toBe(false)
})
