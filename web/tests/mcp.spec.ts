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

async function logInForConsent(page: Page, clientName = '浏览器验收客户端') {
  await expect(page).toHaveURL(/\/login\?redirect=/)
  await page.getByLabel('用户名').fill('admin')
  await page.getByLabel('密码', { exact: true }).fill('e2e-admin-password')
  await page.getByRole('button', { name: '登录', exact: true }).click()
  await expect(page).toHaveURL(/\/mcp\/authorize\?request=/)
  await expect(page.getByRole('heading', { name: clientName, exact: true })).toBeVisible()
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

test('ChatGPT Web 固定客户端完成登录、初始化、工具发现和只读调用', async ({ page, baseURL }) => {
  const clientID = 'https://chatgpt.com/oauth/client.json'
  const redirectURI = 'https://chatgpt.com/connector_platform_oauth_redirect'
  const verifier = randomBytes(48).toString('base64url')
  const params = new URLSearchParams({
    client_id: clientID, redirect_uri: redirectURI, response_type: 'code',
    scope: 'creative-library:read', resource: `${baseURL}/api/mcp`, state: 'chatgpt-web-state', ui_locales: 'zh-CN',
    code_challenge: createHash('sha256').update(verifier).digest('base64url'), code_challenge_method: 'S256',
  })
  // Keep the browser callback local to this test; the authorization and token
  // requests use the actual backend and the same stable identity as ChatGPT.
  await page.route(`${redirectURI}**`, route => route.fulfill({ contentType: 'text/html', body: '<h1>ChatGPT 已收到授权结果</h1>' }))
  await page.goto(`/api/mcp/oauth/authorize?${params}`)
  await logInForConsent(page, 'ChatGPT')
  await expect(page.getByText('此授权不能修改内容', { exact: false })).toBeVisible()
  await page.getByRole('button', { name: '允许只读访问', exact: true }).click()
  await expect(page).toHaveURL(url => url.origin + url.pathname === redirectURI)
  const callback = new URL(page.url())
  expect(callback.searchParams.get('state')).toBe('chatgpt-web-state')
  expect(callback.searchParams.get('iss')).toBe(baseURL)
  const tokenResponse = await page.request.post(`${baseURL}/api/mcp/oauth/token`, {
    form: { client_id: clientID, grant_type: 'authorization_code', code: callback.searchParams.get('code')!,
      code_verifier: verifier, redirect_uri: redirectURI, resource: `${baseURL}/api/mcp` },
  })
  expect(tokenResponse.ok(), await tokenResponse.text()).toBeTruthy()
  const credentials = await tokenResponse.json()
  expect(credentials.scope).toBe('creative-library:read')
  const headers = { Authorization: `Bearer ${credentials.access_token}`, Origin: 'https://chatgpt.com', Accept: 'application/json, text/event-stream', 'MCP-Protocol-Version': '2025-03-26' }
  const initialized = await page.request.post(`${baseURL}/api/mcp`, {
    headers,
    data: { jsonrpc: '2.0', id: 1, method: 'initialize', params: { protocolVersion: '2025-03-26', capabilities: {}, clientInfo: { name: 'openai-mcp', version: '1.0.0' } } },
  })
  expect(initialized.ok(), await initialized.text()).toBeTruthy()
  expect((await initialized.json()).result.capabilities.tools).toBeDefined()
  const notified = await page.request.post(`${baseURL}/api/mcp`, { headers, data: { jsonrpc: '2.0', method: 'notifications/initialized' } })
  expect(notified.status()).toBe(202)
  const discovered = await page.request.post(`${baseURL}/api/mcp`, { headers, data: { jsonrpc: '2.0', id: 2, method: 'tools/list', params: {} } })
  expect(discovered.ok(), await discovered.text()).toBeTruthy()
  const tools = (await discovered.json()).result.tools
  expect(tools).toHaveLength(4)
  for (const tool of tools) {
    expect(tool.outputSchema.type).toBe('object')
    expect(tool._meta.securitySchemes).toEqual([{ type: 'oauth2', scopes: ['creative-library:read'] }])
    expect(tool.annotations.readOnlyHint).toBe(true)
  }
  const result = await page.request.post(`${baseURL}/api/mcp`, {
    headers,
    data: { jsonrpc: '2.0', id: 3, method: 'tools/call', params: { name: 'get_library_info', arguments: {} } },
  })
  expect(result.ok(), await result.text()).toBeTruthy()
  expect((await result.json()).result.isError).not.toBe(true)

  const { csrf_token } = await (await page.request.get(`${baseURL}/api/auth/me`)).json()
  const created = await page.request.post(`${baseURL}/api/products`, { headers: { 'X-CSRF-Token': csrf_token }, data: { chinese_name: 'ChatGPT 发现验收', description: '工具发现后读取的完整正文' } })
  expect(created.ok()).toBeTruthy()
  const product = await created.json()
  const detail = await page.request.post(`${baseURL}/api/mcp`, { headers, data: { jsonrpc: '2.0', id: 4, method: 'tools/call', params: { name: 'fetch', arguments: { id: `product:${product.id}` } } } })
  expect(detail.ok(), await detail.text()).toBeTruthy()
  const document = (await detail.json()).result
  expect(document.isError).not.toBe(true)
  expect(document.structuredContent.text).toContain('工具发现后读取的完整正文')
  expect(JSON.parse(document.content[0].text)).toEqual(document.structuredContent)
})
