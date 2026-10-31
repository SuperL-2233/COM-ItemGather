import { test, expect, type Page, type Browser } from '@playwright/test'

const adminPassword = 'e2e-admin-password'
const memberPassword = 'e2e-member-password'

async function signIn(page: Page, username: string, password: string) {
  page.setDefaultTimeout(10000)
  page.setDefaultNavigationTimeout(15000)
  page.on('pageerror', error => console.error('E2E browser error:', error.message))
  await page.goto('/ideas')
  await expect(page).toHaveURL(/\/login/)
  await page.getByLabel('用户名').fill(username)
  await page.getByLabel('密码', { exact: true }).fill(password)
  await page.getByRole('checkbox', { name: /记住此设备/ }).check()
  await page.getByRole('button', { name: '登录', exact: true }).click()
  await expect(page).toHaveURL(/\/ideas$/)
  await expect(page.getByRole('textbox', { name: '记录一条灵感' })).toBeVisible()
}

async function readDraft(page: Page): Promise<{ text: string } | undefined> {
  return page.evaluate(() => new Promise((resolve, reject) => {
    const request = indexedDB.open('com-itemgather-offline', 1)
    request.onerror = () => reject(request.error)
    request.onsuccess = () => {
      const get = request.result.transaction('drafts', 'readonly').objectStore('drafts').get('composer')
      get.onsuccess = () => resolve(get.result)
      get.onerror = () => reject(get.error)
    }
  }))
}

async function secondBrowser(browser: Browser) {
  const context = await browser.newContext()
  const page = await context.newPage()
  await signIn(page, 'member', memberPassword)
  return { context, page }
}

test('持久登录、AI 整理、双浏览器 SSE 和手动关联商品', async ({ page, browser }) => {
  await signIn(page, 'admin', adminPassword)
  await page.reload()
  await expect(page.getByRole('textbox', { name: '记录一条灵感' })).toBeVisible()
  const second = await secondBrowser(browser)
  try {
    await page.getByRole('textbox', { name: '记录一条灵感' }).fill('绿色风暴')
    await page.getByRole('button', { name: '保存灵感' }).click()
    await expect(page.locator('.idea-card').filter({ hasText: '绿色风暴' })).toHaveCount(1)
    await expect(page.locator('.idea-card').filter({ hasText: '绿色风暴' })).toContainText('青苹果饮料')
    await expect(second.page.locator('.idea-card').filter({ hasText: '绿色风暴' })).toHaveCount(1)
    await expect(second.page.locator('.idea-card').filter({ hasText: '绿色风暴' })).toContainText('青苹果饮料')

    await page.locator('.idea-card').filter({ hasText: '绿色风暴' }).locator('.card-title').click()
    await expect(page).toHaveURL(/\/ideas\/[0-9a-f-]+$/)
    await page.getByRole('button', { name: '由此灵感创建商品' }).click()
    await page.getByLabel('内部工作名称').fill('青苹果汽水')
    await page.getByRole('button', { name: '创建并关联', exact: true }).click()
    await expect(page).toHaveURL(/\/products\/[0-9a-f-]+$/)
    await expect(page.locator('.linked-idea, .link-row, main')).toContainText('绿色风暴')
    const links = await page.request.get('/api/ideas')
    const ideas = (await links.json()).items
    expect(ideas.find((idea: { id: string }) => page.url().includes(idea.id))).toBeUndefined()
    const detail = await page.request.get(`/api/ideas/${ideas.find((idea: { original_content: string }) => idea.original_content === '绿色风暴').id}`)
    expect((await detail.json()).product_count).toBe(1)
  } finally {
    await second.context.close()
  }
})

test('离线草稿恢复、幂等重试和退出清理', async ({ page }) => {
  await signIn(page, 'member', memberPassword)
  const input = page.getByRole('textbox', { name: '记录一条灵感' })
  await input.fill('尚未提交的本机草稿')
  await expect.poll(async () => (await readDraft(page))?.text).toBe('尚未提交的本机草稿')
  await page.reload()
  await expect(input).toHaveValue('尚未提交的本机草稿')
  await input.fill('断网也要保留')
  await page.evaluate(() => navigator.serviceWorker.ready.then(() => undefined))
  await page.context().setOffline(true)
  await page.getByRole('button', { name: '保存灵感' }).click()
  await expect(page.locator('.outbox')).toContainText('断网也要保留')
  await page.reload()
  const pending = await page.evaluate(() => new Promise<{ text: string }[]>((resolve, reject) => {
    const request = indexedDB.open('com-itemgather-offline', 1)
    request.onerror = () => reject(request.error)
    request.onsuccess = () => {
      const get = request.result.transaction('outbox', 'readonly').objectStore('outbox').getAll()
      get.onsuccess = () => resolve(get.result)
      get.onerror = () => reject(get.error)
    }
  }))
  expect(pending.filter(item => item.text === '断网也要保留')).toHaveLength(1)
  let interrupted = false
  await page.route('**/api/ideas', async route => {
    if (route.request().method() === 'POST' && !interrupted) {
      interrupted = true
      await route.fetch()
      await route.abort('failed')
    } else await route.continue()
  })
  await page.context().setOffline(false)
  await page.goto('/ideas')
  await expect(page.locator('.outbox .error-text')).toBeVisible()
  await page.getByRole('button', { name: '重试', exact: true }).click()
  await expect(page.locator('.idea-card').filter({ hasText: '断网也要保留' })).toHaveCount(1)
  await expect(page.locator('.outbox')).toHaveCount(0)
  const result = await page.request.get('/api/ideas')
  const ideas = (await result.json()).items.filter((idea: { original_content: string }) => idea.original_content === '断网也要保留')
  expect(ideas).toHaveLength(1)
  await input.fill('退出后不应保留')
  await expect.poll(async () => (await readDraft(page))?.text).toBe('退出后不应保留')
  await page.getByRole('button', { name: '退出', exact: true }).click()
  await expect(page).toHaveURL(/\/login/)
  expect(await readDraft(page)).toBeUndefined()
})

test('纯图片投稿不调用 AI 并使用未命名标题', async ({ page }) => {
  await signIn(page, 'admin', adminPassword)
  await page.locator('input[type=file]').setInputFiles('public/icon-192.png')
  await page.getByRole('button', { name: '保存灵感' }).click()
  const card = page.locator('.idea-card').filter({ hasText: '未命名灵感' })
  await expect(card).toHaveCount(1)
  await expect(card).not.toContainText('AI 整理失败')
  await expect(page.locator('.composer-status')).toHaveText('已保存到服务器')
  const result = await page.request.get('/api/ideas')
  const idea = (await result.json()).items.find((idea: { original_content: string }) => idea.original_content === '')
  expect(idea.ai_status).toBe('skipped')
  const detail = await page.request.get(`/api/ideas/${idea.id}`)
  expect((await detail.json()).attachments).toHaveLength(1)
})
