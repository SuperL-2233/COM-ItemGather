import { test, expect, type Page, type Browser } from '@playwright/test'
import { readFile } from 'node:fs/promises'

const accountIDs = new WeakMap<Page, string>()

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
  accountIDs.set(page, (await (await page.request.get('/api/auth/me')).json()).user.id)
}

async function readDraft(page: Page): Promise<{ text: string } | undefined> {
  return page.evaluate(userID => new Promise((resolve, reject) => {
    const request = indexedDB.open('com-itemgather-offline', 2)
    request.onerror = () => reject(request.error)
    request.onsuccess = () => {
      const get = request.result.transaction('drafts', 'readonly').objectStore('drafts').get(`composer:${userID}`)
      get.onsuccess = () => resolve(get.result)
      get.onerror = () => reject(get.error)
    }
  }), accountIDs.get(page))
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
    await page.getByRole('textbox', { name: '记录一条灵感' }).fill('一款叫绿色风暴的青苹果汽水')
    await page.getByRole('button', { name: '保存灵感' }).click()
    await expect(page.locator('.idea-card').filter({ hasText: '绿色风暴' })).toHaveCount(1)
    await expect(page.locator('.idea-card').filter({ hasText: '绿色风暴' })).toContainText('青苹果饮料')
    await expect(page.locator('.idea-card').filter({ hasText: '绿色风暴' }).locator('.tag').filter({ hasText: '商品概念' })).toHaveCount(1)
    await expect(second.page.locator('.idea-card').filter({ hasText: '绿色风暴' })).toHaveCount(1)
    await expect(second.page.locator('.idea-card').filter({ hasText: '绿色风暴' })).toContainText('青苹果饮料')

    await page.locator('.idea-card').filter({ hasText: '绿色风暴' }).locator('.card-title').click()
    await expect(page).toHaveURL(/\/ideas\/[0-9a-f-]+$/)
    await page.getByRole('button', { name: '由此灵感创建商品' }).click()
    await page.getByLabel('商品中文名（可留空）').fill('青苹果汽水')
    await page.getByRole('button', { name: '创建并关联', exact: true }).click()
    await expect(page).toHaveURL(/\/products\/[0-9a-f-]+$/)
    await expect(page.locator('.linked-idea, .link-row, main')).toContainText('绿色风暴')
    const links = await page.request.get('/api/ideas')
    const ideas = (await links.json()).items
    expect(ideas.find((idea: { id: string }) => page.url().includes(idea.id))).toBeUndefined()
    const detail = await page.request.get(`/api/ideas/${ideas.find((idea: { original_content: string }) => idea.original_content === '一款叫绿色风暴的青苹果汽水').id}`)
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
    const request = indexedDB.open('com-itemgather-offline', 2)
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
  // The previous offline error remains visible while automatic delivery starts.
  // Retry only after the simulated lost response has actually failed.
  const lostResponse = page.waitForEvent('requestfailed', request => request.method() === 'POST' && new URL(request.url()).pathname === '/api/ideas')
  await page.context().setOffline(false)
  await page.goto('/ideas')
  await lostResponse
  await expect(page.locator('.composer-status')).toHaveText('同步失败，内容仍保留在本机')
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
  await expect(card.locator('.card-preview img')).toBeVisible()
  await expect(page.locator('.composer-status')).toHaveText('已保存到服务器')
  const result = await page.request.get('/api/ideas')
  const idea = (await result.json()).items.find((idea: { original_content: string }) => idea.original_content === '')
  expect(idea.ai_status).toBe('skipped')
  const detail = await page.request.get(`/api/ideas/${idea.id}`)
  expect((await detail.json()).attachments).toHaveLength(1)
})

async function seedQueue(page: Page, text: string, userID?: string) {
  await page.evaluate(({ text, userID }) => new Promise<void>((resolve, reject) => {
    const opened = indexedDB.open('com-itemgather-offline', 2)
    opened.onerror = () => reject(opened.error)
    opened.onsuccess = () => {
      const db = opened.result
      const tx = db.transaction('outbox', 'readwrite')
      tx.objectStore('outbox').put({ id: crypto.randomUUID(), user_id: userID, text, files: [], created_at: new Date().toISOString() })
      tx.oncomplete = () => { db.close(); resolve() }
      tx.onabort = () => reject(tx.error)
    }
  }), { text, userID })
}

test('撤销当前设备清理草稿和队列，下一账号不会读取或提交', async ({ page }) => {
  await signIn(page, 'admin', adminPassword)
  const secret = '撤销设备前的管理员草稿'
  const queued = '撤销设备前的管理员待同步内容'
  await page.getByRole('textbox', { name: '记录一条灵感' }).fill(secret)
  await expect.poll(async () => (await readDraft(page))?.text).toBe(secret)
  await page.getByRole('link', { name: /设置/ }).click()
  await seedQueue(page, queued, accountIDs.get(page))
  await page.getByRole('button', { name: '登录设备', exact: true }).click()
  page.once('dialog', dialog => dialog.accept())
  await page.locator('.device-row').filter({ hasText: '当前设备' }).getByRole('button', { name: '撤销', exact: true }).click()
  await expect(page).toHaveURL(/\/login/)
  expect(await readDraft(page)).toBeUndefined()
  await signIn(page, 'member', memberPassword)
  await expect(page.getByRole('textbox', { name: '记录一条灵感' })).toHaveValue('')
  await expect(page.locator('.outbox')).toHaveCount(0)
  const result = await (await page.request.get(`/api/ideas?q=${encodeURIComponent(queued)}`)).json()
  expect(result.total).toBe(0)
})

test('Session 失效后保留原账号草稿，新账号不接管其待同步内容', async ({ page }) => {
  await signIn(page, 'admin', adminPassword)
  const userID = accountIDs.get(page)!
  const text = '过期会话的未同步内容'
  await page.getByRole('textbox', { name: '记录一条灵感' }).fill('过期会话的草稿')
  await expect.poll(async () => (await readDraft(page))?.text).toBe('过期会话的草稿')
  await page.getByRole('link', { name: /设置/ }).click()
  await seedQueue(page, text, userID)
  const me = await (await page.request.get('/api/auth/me')).json()
  expect((await page.request.post('/api/auth/logout', { headers: { 'X-CSRF-Token': me.csrf_token } })).ok()).toBeTruthy()
  await page.goto('/ideas')
  await expect(page).toHaveURL(/\/login/)
  expect((await readDraft(page))?.text).toBe('过期会话的草稿')
  await signIn(page, 'member', memberPassword)
  await expect(page.getByRole('textbox', { name: '记录一条灵感' })).toHaveValue('')
  await expect(page.locator('.outbox')).toHaveCount(0)
  expect((await (await page.request.get(`/api/ideas?q=${encodeURIComponent(text)}`)).json()).total).toBe(0)
})

test('图片上传成功而响应丢失时，重试仍只保存一个附件', async ({ page }) => {
  await signIn(page, 'admin', adminPassword)
  let interrupted = false
  await page.route('**/api/attachments', async route => {
    if (route.request().method() === 'POST' && !interrupted) {
      interrupted = true
      // Intercepted Chromium multipart requests omit file bytes. Reconstruct the
      // same image and idempotency key, save it, then drop the browser response.
      const body = route.request().postDataBuffer()?.toString() || ''
      const ideaID = body.match(/name="idea_id"\r\n\r\n([^\r]+)/)?.[1]
      const clientID = body.match(/name="client_id"\r\n\r\n([^\r]+)/)?.[1]
      const response = await page.request.post('/api/attachments', {
        headers: { 'X-CSRF-Token': route.request().headers()['x-csrf-token'] },
        multipart: { idea_id: ideaID!, client_id: clientID!, file: { name: 'icon-192.png', mimeType: 'image/png', buffer: await readFile('public/icon-192.png') } },
      })
      expect(response.status()).toBe(201)
      await route.abort('failed')
    } else await route.continue()
  })
  const text = '图片幂等回归验收'
  await page.getByRole('textbox', { name: '记录一条灵感' }).fill(text)
  await page.locator('input[type=file]').setInputFiles('public/icon-192.png')
  await page.getByRole('button', { name: '保存灵感' }).click()
  await expect(page.locator('.outbox .error-text')).toBeVisible()
  await page.locator('.outbox').getByRole('button', { name: '重试', exact: true }).click()
  await expect(page.locator('.outbox')).toHaveCount(0)
  const ideas = (await (await page.request.get('/api/ideas')).json()).items
  const idea = ideas.find((item: { original_content: string }) => item.original_content === text)
  const detail = await (await page.request.get(`/api/ideas/${idea.id}`)).json()
  expect(ideas.filter((item: { original_content: string }) => item.original_content === text)).toHaveLength(1)
  expect(detail.attachments).toHaveLength(1)
  expect((await page.request.get(detail.attachments[0].url)).headers()['cache-control']).toBe('no-store')
})

test('旧版无归属本地数据不被下一账号自动接管', async ({ page }) => {
  await page.goto('/login')
  await page.evaluate(() => new Promise<void>((resolve, reject) => {
    const opened = indexedDB.open('com-itemgather-offline', 1)
    opened.onupgradeneeded = () => { opened.result.createObjectStore('drafts'); opened.result.createObjectStore('outbox', { keyPath: 'id' }) }
    opened.onerror = () => reject(opened.error)
    opened.onsuccess = () => {
      const db = opened.result
      const tx = db.transaction(['drafts', 'outbox'], 'readwrite')
      tx.objectStore('drafts').put({ text: '旧版未知账号草稿', files: [] }, 'composer')
      tx.objectStore('outbox').put({ id: 'legacy-outbox', text: '旧版未知账号队列', files: [], created_at: new Date().toISOString() })
      tx.oncomplete = () => { db.close(); resolve() }
      tx.onabort = () => reject(tx.error)
    }
  }))
  await signIn(page, 'member', memberPassword)
  await expect(page.getByRole('textbox', { name: '记录一条灵感' })).toHaveValue('')
  await expect(page.locator('.outbox')).toHaveCount(0)
  expect((await (await page.request.get(`/api/ideas?q=${encodeURIComponent('旧版未知账号队列')}`)).json()).total).toBe(0)
})

test('仅修改正文后普通重新整理会更新摘要，详情可以收藏', async ({ page }) => {
  await signIn(page, 'admin', adminPassword)
  await page.getByRole('textbox', { name: '记录一条灵感' }).fill('普通整理回归的第一版')
  await page.getByRole('button', { name: '保存灵感' }).click()
  const card = page.locator('.idea-card').filter({ hasText: '普通整理回归的第一版' })
  await expect(card.locator('.card-meta')).not.toContainText('整理中')
  await card.locator('.card-title').click()
  await expect(page.locator('#content')).toBeVisible()
  await page.getByRole('button', { name: '收藏', exact: true }).click()
  await expect(page.getByRole('button', { name: '取消收藏', exact: true })).toBeVisible()
  await page.locator('#content').fill('普通整理回归的第二版')
  await page.getByRole('button', { name: '保存修改', exact: true }).click()
  await expect(page.getByRole('button', { name: '已保存', exact: true })).toBeVisible()
  await page.getByRole('button', { name: '重新整理', exact: true }).click()
  await expect(page.locator('#summary')).toHaveValue('普通整理回归的第二版')
})

test('失败卡片可以直接重新整理，内容类型可见', async ({ page }) => {
  await signIn(page, 'admin', adminPassword)
  const text = '整理重试验收：卡片重试'
  await page.getByRole('textbox', { name: '记录一条灵感' }).fill(text)
  await page.getByRole('button', { name: '保存灵感' }).click()
  const card = page.locator('.idea-card').filter({ hasText: text })
  await expect(card).toHaveClass(/ai-failed/)
  await card.getByRole('button', { name: '重新整理', exact: true }).click()
  await expect(card).not.toHaveClass(/ai-failed/)
  await expect.poll(async () => (await (await page.request.get('/api/ideas')).json()).items.find((item: { original_content: string }) => item.original_content === text).ai_status).toBe('succeeded')
})

test('商品搜索输入即查，连续输入合并请求且等待时保留列表', async ({ page }) => {
  await signIn(page, 'admin', adminPassword)
  const me = await (await page.request.get('/api/auth/me')).json()
  for (const chinese_name of ['即时搜索原有商品', 'LiveSearchTarget 即时搜索目标商品']) {
    const created = await page.request.post('/api/products', { headers: { 'X-CSRF-Token': me.csrf_token }, data: { chinese_name } })
    expect(created.status()).toBe(201)
  }
  await page.goto('/products')
  const input = page.getByRole('searchbox', { name: '搜索商品', exact: true })
  const original = page.locator('.product-card').filter({ hasText: '即时搜索原有商品' })
  await expect(original).toBeVisible()
  await page.waitForTimeout(250)
  let searches = 0
  let releaseSearch!: () => void
  const searchResponse = new Promise<void>(resolve => { releaseSearch = resolve })
  await page.route('**/api/products?*', async route => {
    const query = new URL(route.request().url()).searchParams.get('q') || ''
    if (!query) return route.continue()
    searches++
    if (query === 'LiveSearchTarget') await searchResponse
    await route.continue()
  })
  await input.pressSequentially('LiveSearchTarget', { delay: 35 })
  await expect.poll(() => searches).toBe(1)
  await expect(original).toBeVisible()
  await expect(page.getByText('读取中…', { exact: true })).toHaveCount(0)
  releaseSearch()
  await expect(page.locator('.product-card')).toHaveCount(1)
  await expect(page.locator('.product-card')).toContainText('LiveSearchTarget 即时搜索目标商品')
  expect(searches).toBe(1)
  await expect(page.getByRole('button', { name: '搜索', exact: true })).toHaveCount(0)
})

test('商品列表自动加载后续内容，英文名作为次要信息呈现', async ({ page }) => {
  await signIn(page, 'admin', adminPassword)
  const me = await (await page.request.get('/api/auth/me')).json()
  for (let index = 0; index < 31; index++) {
    const created = await page.request.post('/api/products', {
      headers: { 'X-CSRF-Token': me.csrf_token },
      data: { chinese_name: `自动加载验收商品 ${index + 1}` },
    })
    expect(created.status()).toBe(201)
  }
  const named = await page.request.post('/api/products', {
    headers: { 'X-CSRF-Token': me.csrf_token },
    data: { chinese_name: '自动加载验收商品：名称样式', english_name: 'Style Sample' },
  })
  expect(named.status()).toBe(201)
  await page.goto('/products')
  await page.getByPlaceholder('搜索名称、公司、分类或描述').fill('自动加载验收商品')
  await expect(page.locator('.product-card').first()).toBeVisible()
  await expect(page.getByRole('button', { name: /加载更多/ })).toHaveCount(0)
  const englishName = page.locator('.catalog-english').filter({ hasText: 'Style Sample' })
  await expect(englishName).toHaveText('Style Sample')
  await expect(englishName).toHaveCSS('font-style', 'normal')
  await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight))
  await expect(page.locator('.product-card')).toHaveCount(32)
  await expect(page.locator('.product-card').last()).toContainText('自动加载验收商品 1')
})

test('商品可以通过漏斗覆层搜索选项并组合筛选', async ({ page }) => {
  await signIn(page, 'admin', adminPassword)
  const me = await (await page.request.get('/api/auth/me')).json()
  const fixtures = [
    { chinese_name: '筛选器目标商品', company: '筛选器公司', category: '筛选器分类', development_status: 'design', attributes: [{ key: '筛选器属性', value: '酸甜', certainty: 'asserted' }] },
    { chinese_name: '筛选器对照商品', company: '对照公司', category: '对照分类', development_status: 'production', attributes: [{ key: '包装', value: '瓶装', certainty: 'asserted' }] },
  ]
  for (const data of fixtures) {
    const created = await page.request.post('/api/products', { headers: { 'X-CSRF-Token': me.csrf_token }, data })
    expect(created.status()).toBe(201)
  }
  await page.goto('/products')
  await page.getByPlaceholder('搜索名称、公司、分类或描述').fill('筛选器')
  await expect(page.locator('.product-card')).toHaveCount(2)

  const funnel = page.getByRole('button', { name: '筛选商品', exact: true })
  await funnel.click()
  const popover = page.getByRole('dialog', { name: '商品筛选', exact: true })
  await expect(popover).toBeVisible()
  const categories = popover.getByRole('navigation', { name: '筛选分类', exact: true })
  const companySearch = page.getByRole('searchbox', { name: '搜索所属公司选项', exact: true })
  const categorySearch = page.getByRole('searchbox', { name: '搜索商品分类选项', exact: true })
  const attributeSearch = page.getByRole('searchbox', { name: '搜索动态属性分类选项', exact: true })
  const statusSearch = page.getByRole('searchbox', { name: '搜索制作状态选项', exact: true })
  await expect(companySearch).toBeFocused()
  await expect(categorySearch).toHaveCount(0)
  await expect(attributeSearch).toHaveCount(0)
  await expect(statusSearch).toHaveCount(0)
  await expect(categories.getByRole('button')).toHaveText(['所属公司', '商品分类', '动态属性分类', '制作状态'])

  const cards = page.locator('.product-card')
  await companySearch.fill('不存在的公司')
  await expect(popover.getByText('没有匹配选项', { exact: true })).toHaveCount(1)
  await companySearch.fill('筛选器公司')
  await expect(popover.getByRole('button', { name: '对照公司', exact: true })).toHaveCount(0)
  const targetCompany = popover.getByRole('button', { name: '筛选器公司', exact: true })
  await targetCompany.click()
  await expect(cards).toHaveCount(1)
  await expect(cards).toContainText('筛选器目标商品')
  await expect(funnel.locator('.filter-count')).toHaveText('1')
  await companySearch.fill('')
  const otherCompany = popover.getByRole('button', { name: '对照公司', exact: true })
  await otherCompany.click()
  await expect(cards).toHaveCount(2)
  await expect(targetCompany).toHaveAttribute('aria-pressed', 'true')
  await expect(otherCompany).toHaveAttribute('aria-pressed', 'true')
  await expect(funnel.locator('.filter-count')).toHaveText('1')
  await otherCompany.click()
  await expect(cards).toHaveCount(1)
  await expect(otherCompany).toHaveAttribute('aria-pressed', 'false')
  await popover.getByRole('button', { name: '全部公司', exact: true }).click()
  await expect(cards).toHaveCount(2)

  await categories.getByRole('button', { name: '商品分类', exact: true }).click()
  await expect(companySearch).toHaveCount(0)
  await expect(categorySearch).toBeVisible()
  await categorySearch.fill('筛选器分类')
  await popover.getByRole('button', { name: '筛选器分类', exact: true }).click()
  await expect(cards).toHaveCount(1)
  await expect(cards).toContainText('筛选器目标商品')
  await popover.getByRole('button', { name: '全部分类', exact: true }).click()
  await expect(cards).toHaveCount(2)

  await categories.getByRole('button', { name: '动态属性分类', exact: true }).click()
  await expect(categorySearch).toHaveCount(0)
  await expect(attributeSearch).toBeVisible()
  await attributeSearch.fill('筛选器属性')
  await popover.getByRole('button', { name: '筛选器属性', exact: true }).click()
  await expect(cards).toHaveCount(1)
  await expect(cards).toContainText('筛选器目标商品')
  await popover.getByRole('button', { name: '全部属性分类', exact: true }).click()
  await expect(cards).toHaveCount(2)

  await categories.getByRole('button', { name: '制作状态', exact: true }).click()
  await expect(attributeSearch).toHaveCount(0)
  await expect(statusSearch).toBeVisible()
  await statusSearch.fill('设计中')
  await popover.getByRole('button', { name: '设计中', exact: true }).click()
  await expect(cards).toHaveCount(1)
  await expect(cards).toContainText('筛选器目标商品')
  await expect(popover).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(popover).toHaveCount(0)
  await expect(funnel).toBeFocused()
})

test('手机端筛选器不会因外部滑动或长按关闭，仅点按时关闭', async ({ browser }) => {
  const context = await browser.newContext({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true, deviceScaleFactor: 1 })
  const page = await context.newPage()
  try {
    await signIn(page, 'admin', adminPassword)
    const me = await (await page.request.get('/api/auth/me')).json()
    for (let index = 0; index < 8; index++) {
      await page.request.post('/api/products', { headers: { 'X-CSRF-Token': me.csrf_token }, data: { chinese_name: `手机筛选商品 ${index + 1}` } })
    }
    await page.goto('/products')
    await page.getByRole('button', { name: '筛选商品', exact: true }).click()
    const popover = page.getByRole('dialog', { name: '商品筛选', exact: true })
    await expect(popover).toBeVisible()
    const cdp = await context.newCDPSession(page)
    await cdp.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x: 2, y: 650 }] })
    await cdp.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x: 2, y: 300 }] })
    await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] })
    await expect.poll(() => page.evaluate(() => window.scrollY)).toBeGreaterThan(0)
    await expect(popover).toBeVisible()

    await cdp.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x: 2, y: 500 }] })
    await page.waitForTimeout(700)
    await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] })
    await expect(popover).toBeVisible()

    await page.touchscreen.tap(2, 500)
    await expect(popover).toHaveCount(0)
  } finally { await context.close() }
})

test('商品搜索可以按创建者筛选', async ({ page, browser }) => {
  await signIn(page, 'admin', adminPassword)
  const second = await secondBrowser(browser)
  try {
    for (const [target, name] of [[page, '创建者验收：管理员商品'], [second.page, '创建者验收：成员商品']] as const) {
      const me = await (await target.request.get('/api/auth/me')).json()
      const created = await target.request.post('/api/products', { headers: { 'X-CSRF-Token': me.csrf_token }, data: { chinese_name: name } })
      expect(created.status()).toBe(201)
    }
    const userID = accountIDs.get(second.page)!
    await page.goto(`/search?q=${encodeURIComponent('创建者验收')}&kind=products&author=${userID}`)
    await page.getByRole('button', { name: '筛选搜索结果', exact: true }).click()
    const filters = page.getByRole('dialog', { name: '搜索结果筛选', exact: true })
    await filters.getByRole('navigation').getByRole('button', { name: '创建者', exact: true }).click()
    await expect(filters.getByRole('button', { name: '第二位成员', exact: true })).toHaveAttribute('aria-pressed', 'true')
    await filters.getByRole('button', { name: '完成', exact: true }).click()
    await expect(page.locator('.product-card')).toHaveCount(1)
    await expect(page.locator('.product-card')).toContainText('创建者验收：成员商品')
  } finally { await second.context.close() }
})
