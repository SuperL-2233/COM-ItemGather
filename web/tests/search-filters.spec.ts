import { test, expect, type Page } from '@playwright/test'
import { randomUUID } from 'node:crypto'

async function login(page: Page) {
  await page.goto('/login')
  await page.getByLabel('用户名').fill('admin')
  await page.getByLabel('密码', { exact: true }).fill('e2e-admin-password')
  await page.getByRole('button', { name: '登录', exact: true }).click()
  await expect(page).toHaveURL(/\/ideas$/)
}
async function seed(page: Page, collection: string, data: Record<string, unknown>) {
  const me = await (await page.request.get('/api/auth/me')).json()
  const result = await page.request.post(`/api/${collection}`, { headers: { 'X-CSRF-Token': me.csrf_token }, data })
  expect(result.ok(), await result.text()).toBeTruthy()
  return result.json()
}

for (const pageType of [
  { path: 'tools', label: '搜索道具', cards: '.product-card' },
  { path: 'companies', label: '搜索公司', cards: '.product-card' },
  { path: 'recipes', label: '搜索配方', cards: '.product-card' },
  { path: 'ideas', label: '搜索灵感', cards: '.idea-card' },
]) {
  test(`${pageType.label}合并连续输入的请求，并在等待时保留列表`, async ({ page }) => {
    await login(page)
    const prefix = `UnifiedLive-${pageType.path}`
    for (const name of [`${prefix} 原有记录`, `${prefix} Target 目标记录`]) {
      if (pageType.path === 'recipes') {
        const output = await seed(page, 'products', { chinese_name: name })
        await seed(page, 'recipe-groups', { output_id: output.id })
      } else if (pageType.path === 'ideas') {
        const idea = await seed(page, 'ideas', { content: name, client_id: randomUUID() })
        await expect.poll(async () => (await (await page.request.get(`/api/ideas/${idea.id}`)).json()).ai_status).toBe('succeeded')
      } else await seed(page, pageType.path, { chinese_name: name })
    }
    await page.goto(`/${pageType.path}`)
    const input = page.getByRole('searchbox', { name: pageType.label, exact: true })
    await input.fill(prefix)
    await expect(page.locator(pageType.cards)).toHaveCount(2)
    const original = page.locator(pageType.cards).filter({ hasText: '原有记录' })
    let count = 0
    let release!: () => void
    const gate = new Promise<void>(resolve => { release = resolve })
    const endpoint = pageType.path === 'recipes' ? 'recipe-groups' : pageType.path
    await page.route(`**/api/${endpoint}?*`, async route => {
      if (new URL(route.request().url()).searchParams.get('q')?.startsWith(`${prefix} `)) {
        count++
        await gate
      }
      await route.continue()
    })
    await input.press('End')
    await input.pressSequentially(' Target', { delay: 30 })
    await expect.poll(() => count).toBe(1)
    await expect(original).toBeVisible()
    await expect(page.getByText('读取中…', { exact: true })).toHaveCount(0)
    release()
    await expect(page.locator(pageType.cards)).toHaveCount(1)
    await expect(page.locator(pageType.cards)).toContainText('目标记录')
    expect(count).toBe(1)
    await input.fill(`${prefix} 原有记录`)
    await input.press('Enter')
    await expect(page.locator(pageType.cards)).toContainText('原有记录')
    await page.waitForTimeout(350)
    expect(count).toBe(2)
    await expect(page.getByRole('button', { name: '搜索', exact: true })).toHaveCount(0)
  })
}

test('道具使用类别、来源和状态复用漏斗，支持同组多选及清除', async ({ page }) => {
  await login(page)
  for (const [name, category, source, status] of [
    ['主动项', '主动', '购买', 'concept'],
    ['被动项', '被动', 'NPC 赠送', 'design'],
    ['完成项', '被动', '购买', 'completed'],
  ]) await seed(page, 'tools', { chinese_name: `统一漏斗道具 ${name}`, use_category: category, development_status: status, tool_settings: { sources: [{ method: source }] } })
  await page.goto('/tools')
  await page.getByRole('searchbox', { name: '搜索道具', exact: true }).fill('统一漏斗道具')
  const cards = page.locator('.product-card')
  await expect(cards).toHaveCount(3)
  const funnel = page.getByRole('button', { name: '筛选道具', exact: true })
  await funnel.click()
  const popover = page.getByRole('dialog', { name: '道具筛选', exact: true })
  const nav = popover.getByRole('navigation')
  const options = popover.locator('.filter-options-pane')
  await expect(nav.getByRole('button')).toHaveText(['使用类别', '获取来源', '制作状态'])
  const search = popover.getByRole('searchbox', { name: '搜索使用类别选项', exact: true })
  await expect(search).toBeFocused()
  await search.fill('主')
  await expect(options.getByRole('button', { name: '被动', exact: true })).toHaveCount(0)
  await options.getByRole('button', { name: '主动', exact: true }).click()
  await expect(cards).toHaveCount(1)
  await search.fill('')
  await options.getByRole('button', { name: '被动', exact: true }).click()
  await expect(cards).toHaveCount(3)
  await nav.getByRole('button', { name: '获取来源', exact: true }).click()
  await options.getByRole('button', { name: '购买', exact: true }).click()
  await expect(cards).toHaveCount(2)
  await options.getByRole('button', { name: 'NPC 赠送', exact: true }).click()
  await expect(cards).toHaveCount(3)
  await nav.getByRole('button', { name: '制作状态', exact: true }).click()
  await options.getByRole('button', { name: '设计中', exact: true }).click()
  await expect(cards).toHaveCount(1)
  await options.getByRole('button', { name: '构思中', exact: true }).click()
  await expect(cards).toHaveCount(2)
  await expect(funnel.locator('.filter-count')).toHaveText('3')
  await popover.getByRole('button', { name: '清除筛选', exact: true }).click()
  await expect(cards).toHaveCount(3)
  await expect(funnel.locator('.filter-count')).toHaveCount(0)
  await page.screenshot({ path: '/tmp/com-unified-tool-filters.png', fullPage: true })
  await page.keyboard.press('Escape')
  await expect(popover).toHaveCount(0)
  await expect(funnel).toBeFocused()
})

test('灵感浏览范围使用同一漏斗，保留收藏与清除行为', async ({ page }) => {
  await login(page)
  const idea = await seed(page, 'ideas', { content: '灵感漏斗收藏目标', client_id: randomUUID() })
  await seed(page, `ideas/${idea.id}/favorite`, { favorite: true })
  await page.goto('/ideas')
  await page.getByRole('searchbox', { name: '搜索灵感', exact: true }).fill('灵感漏斗收藏目标')
  await expect(page.locator('.idea-card')).toHaveCount(1)
  const funnel = page.getByRole('button', { name: '筛选灵感', exact: true })
  await funnel.click()
  const popover = page.getByRole('dialog', { name: '灵感筛选', exact: true })
  await popover.getByRole('button', { name: '收藏', exact: true }).click()
  await expect(popover.getByRole('button', { name: '收藏', exact: true })).toHaveAttribute('aria-pressed', 'true')
  await expect(page.locator('.idea-card')).toHaveCount(1)
  await popover.getByRole('button', { name: '我创建的', exact: true }).click()
  await expect(popover.getByRole('button', { name: '收藏', exact: true })).toHaveAttribute('aria-pressed', 'false')
  await popover.getByRole('button', { name: '清除筛选', exact: true }).click()
  await expect(popover.getByRole('button', { name: '全部灵感', exact: true })).toHaveAttribute('aria-pressed', 'true')
  await expect(funnel.locator('.filter-count')).toHaveCount(0)
})

test('全局搜索输入即查、多选可分享，取消旧请求并保留当前结果', async ({ page }) => {
  await login(page)
  await seed(page, 'products', { chinese_name: 'GlobalLiveOld 原有商品' })
  await seed(page, 'products', { chinese_name: 'GlobalLiveNew 目标商品' })
  await seed(page, 'tools', { chinese_name: 'GlobalLiveNew 目标道具' })
  await page.goto('/search?q=GlobalLiveOld&kind=products')
  await expect(page.locator('.product-card')).toHaveCount(1)
  const input = page.getByRole('searchbox', { name: '全局搜索关键词', exact: true })
  let count = 0
  let release!: () => void
  const gate = new Promise<void>(resolve => { release = resolve })
  await page.route('**/api/search?*', async route => {
    if (new URL(route.request().url()).searchParams.get('q') === 'GlobalLiveNew') { count++; await gate }
    await route.continue()
  })
  await input.fill('')
  await input.pressSequentially('GlobalLiveNew', { delay: 25 })
  await expect.poll(() => count).toBe(1)
  await expect(page.locator('.product-card')).toContainText('原有商品')
  release()
  await expect(page.locator('.product-card')).toContainText('目标商品')
  expect(count).toBe(1)
  await page.getByRole('button', { name: '筛选搜索结果', exact: true }).click()
  const popover = page.getByRole('dialog', { name: '搜索结果筛选', exact: true })
  await popover.locator('.filter-options-pane').getByRole('button', { name: '道具', exact: true }).click()
  await expect(page.locator('.product-card')).toHaveCount(2)
  await expect.poll(() => new URL(page.url()).searchParams.getAll('kind')).toEqual(['products', 'tools'])
  await page.reload()
  await expect(input).toHaveValue('GlobalLiveNew')
  await expect(page.locator('.product-card')).toHaveCount(2)
  await page.getByRole('button', { name: '筛选搜索结果', exact: true }).click()
  await expect(popover.locator('.filter-options-pane').getByRole('button', { name: '商品', exact: true })).toHaveAttribute('aria-pressed', 'true')
  await expect(popover.locator('.filter-options-pane').getByRole('button', { name: '道具', exact: true })).toHaveAttribute('aria-pressed', 'true')
  await popover.getByRole('button', { name: '完成', exact: true }).click()

  let slowStarted = false
  let releaseSlow!: () => void
  const slow = new Promise<void>(resolve => { releaseSlow = resolve })
  await page.route('**/api/search?*', async route => {
    if (new URL(route.request().url()).searchParams.get('q') !== 'GlobalSlow') return route.continue()
    const response = await route.fetch()
    slowStarted = true
    await slow
    await route.fulfill({ response }).catch(() => {})
  })
  await input.fill('GlobalSlow')
  await expect.poll(() => slowStarted).toBe(true)
  await input.fill('GlobalLiveNew')
  await expect.poll(() => new URL(page.url()).searchParams.get('q')).toBe('GlobalLiveNew')
  await expect(page.locator('.product-card')).toHaveCount(2)
  releaseSlow()
  await page.waitForTimeout(350)
  await expect(page.locator('.product-card')).toHaveCount(2)
  await expect(page.locator('.error-text')).toHaveCount(0)
  await expect(page.getByRole('button', { name: '搜索', exact: true })).toHaveCount(0)
})

test('关联物品和设定输入即查，切换目标类型后刷新匹配结果', async ({ page }) => {
  await login(page)
  await seed(page, 'products', { chinese_name: '自动关联检索商品' })
  await seed(page, 'tools', { chinese_name: '自动关联检索道具' })
  await seed(page, 'companies', { chinese_name: '自动关联检索公司' })
  const idea = await seed(page, 'ideas', { content: '自动关联检索灵感', client_id: randomUUID() })
  await expect.poll(async () => (await (await page.request.get(`/api/ideas/${idea.id}`)).json()).ai_status).toBe('succeeded')
  await page.goto(`/ideas/${idea.id}`)
  const items = page.locator('aside .panel').filter({ has: page.getByRole('heading', { name: '关联商品与道具', exact: true }) })
  await items.getByRole('searchbox', { name: '搜索关联物品', exact: true }).fill('自动关联检索')
  await seed(page, 'companies', { chinese_name: '触发后台刷新，保留搜索草稿' })
  await expect(items.locator('.search-result')).toHaveText('自动关联检索商品关联')
  await expect(items.getByRole('searchbox', { name: '搜索关联物品', exact: true })).toHaveValue('自动关联检索')
  await items.getByLabel('关联物品类型', { exact: true }).selectOption('tools')
  await expect(items.locator('.search-result')).toHaveText('自动关联检索道具关联')
  await items.locator('.search-result').getByRole('button', { name: '关联', exact: true }).click()
  await expect(items.getByRole('link', { name: '自动关联检索道具', exact: true })).toBeVisible()
  const settings = page.locator('aside .panel').filter({ has: page.getByRole('heading', { name: '关联公司与配方', exact: true }) })
  await settings.getByRole('searchbox', { name: '搜索设定', exact: true }).fill('自动关联检索')
  await expect(settings.locator('.search-result')).toHaveText('自动关联检索公司关联')
  await settings.locator('.search-result').getByRole('button', { name: '关联', exact: true }).click()
  await expect(settings.getByRole('link', { name: '自动关联检索公司', exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: '搜索', exact: true })).toHaveCount(0)
})

test('手机端全局搜索筛选适应宽度并可点按外部关闭', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await login(page)
  await page.goto('/search')
  const funnel = page.getByRole('button', { name: '筛选搜索结果', exact: true })
  await funnel.click()
  const popover = page.getByRole('dialog', { name: '搜索结果筛选', exact: true })
  await expect(popover).toBeVisible()
  await popover.getByRole('searchbox', { name: '搜索记录类型选项', exact: true }).fill('公司')
  await popover.locator('.filter-options-pane').getByRole('button', { name: '公司', exact: true }).click()
  await expect(funnel.locator('.filter-count')).toHaveText('1')
  const bounds = await popover.boundingBox()
  expect(bounds!.x).toBeGreaterThanOrEqual(0)
  expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(390)
  await page.screenshot({ path: '/tmp/com-unified-search-mobile.png', fullPage: true })
  await page.getByRole('heading', { name: '全局搜索', exact: true }).click()
  await expect(popover).toHaveCount(0)
})
