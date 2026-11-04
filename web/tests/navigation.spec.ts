import { test, expect, type Page } from '@playwright/test'
import { randomUUID } from 'node:crypto'

async function login(page: Page, username = 'admin', password = 'e2e-admin-password') {
  await page.goto('/login')
  await page.getByLabel('用户名').fill(username)
  await page.getByLabel('密码', { exact: true }).fill(password)
  await page.getByRole('button', { name: '登录', exact: true }).click()
  await expect(page).toHaveURL(/\/ideas$/)
}

async function creator(page: Page) {
  const { csrf_token } = await (await page.request.get('/api/auth/me')).json()
  return async (path: string, data: Record<string, unknown>) => {
    const response = await page.request.post(`/api/${path}`, { headers: { 'X-CSRF-Token': csrf_token }, data })
    expect(response.ok(), await response.text()).toBeTruthy()
    return response.json()
  }
}

async function expectPosition(page: Page, top: number) {
  await expect.poll(() => page.evaluate(expected => Math.abs(window.scrollY - expected), top)).toBeLessThan(4)
}

test('商品编辑后通过返回链接和浏览器后退保留分页、位置、筛选及公司范围', async ({ page }) => {
  await login(page)
  const create = await creator(page)
  const company = await create('companies', { chinese_name: '浏览位置公司' })
  const prefix = '浏览位置商品'
  const names = Array.from({ length: 145 }, (_, index) => `${prefix} ${String(index).padStart(3, '0')}`)
  const records: { id: string }[] = []
  for (const chinese_name of names) records.push(await create('products', { chinese_name, company_id: company.id, category: '浏览位置分类' }))

  await page.goto(`/products?company_id=${company.id}`)
  const search = page.getByRole('searchbox', { name: '搜索商品', exact: true })
  await search.fill(prefix)
  const sort = page.getByRole('combobox', { name: '商品排序', exact: true })
  await sort.click()
  await page.getByRole('option', { name: '名称 · 正序', exact: true }).click()
  await page.getByRole('button', { name: '筛选商品', exact: true }).click()
  const filter = page.getByRole('dialog', { name: '商品筛选', exact: true })
  await filter.getByRole('navigation').getByRole('button', { name: '商品分类', exact: true }).click()
  await filter.getByRole('button', { name: '浏览位置分类', exact: true }).click()
  await filter.getByRole('button', { name: '完成', exact: true }).click()
  const rows = page.locator('.catalog-item')
  await expect(rows).toHaveCount(30)
  for (let count = 30; count < 145; count += 30) {
    await page.locator('.load-more-status').scrollIntoViewIfNeeded()
    await expect(rows).toHaveCount(Math.min(count + 30, 145))
  }
  const target = page.getByRole('link', { name: names[118], exact: true })
  await target.scrollIntoViewIfNeeded()
  const top = await page.evaluate(() => window.scrollY)
  expect(top).toBeGreaterThan(3000)
  await target.click()
  await expect(page.getByLabel('商品中文名')).toHaveValue(names[118])
  await expectPosition(page, 0)
  await page.getByLabel('商品描述').fill('编辑后返回应继续浏览原来的位置')
  await page.getByRole('button', { name: '保存修改', exact: true }).click()
  await expect(page.getByRole('button', { name: '已保存', exact: true })).toBeDisabled()
  const back = page.getByRole('link', { name: '← 返回商品库', exact: true })
  await expect(back).toHaveAttribute('href', `/products?company_id=${company.id}`)
  await back.click()
  await expect(search).toHaveValue(prefix)
  await expect(sort).toHaveAttribute('data-value', 'name_asc')
  await expect(page.getByRole('button', { name: '移除商品分类：浏览位置分类筛选', exact: true })).toBeVisible()
  await expect(rows).toHaveCount(145)
  await expectPosition(page, top)

  // A subsequent SSE refresh must retain every loaded page, including rows
  // beyond the API's 100-item maximum response size.
  const { csrf_token } = await (await page.request.get('/api/auth/me')).json()
  const refreshed = page.waitForResponse(async response => {
    const url = new URL(response.url())
    if (url.pathname !== '/api/products' || url.searchParams.get('offset') !== '100') return false
    const result = await response.json()
    return result.items.some((item: { id: string; version: number }) => item.id === records[118].id && item.version === 3)
  })
  const changed = await page.request.patch(`/api/products/${records[118].id}`, { headers: { 'X-CSRF-Token': csrf_token }, data: { version: 2, description: '验证后续实时刷新仍保留位置' } })
  expect(changed.ok(), await changed.text()).toBeTruthy()
  await refreshed
  await expect(rows).toHaveCount(145)
  await expectPosition(page, top)
  await page.getByRole('link', { name: names[119], exact: true }).click()
  await expect(page.getByLabel('商品中文名')).toHaveValue(names[119])
  await page.goBack()
  await expect(search).toHaveValue(prefix)
  await expect(rows).toHaveCount(145)
  await expectPosition(page, top)

  // Display state belongs to the signed-in account, including when the next
  // account navigates through the same SPA without reloading the application.
  await page.getByRole('button', { name: '退出', exact: true }).click()
  await page.getByLabel('用户名').fill('member')
  await page.getByLabel('密码', { exact: true }).fill('e2e-member-password')
  await page.getByRole('button', { name: '登录', exact: true }).click()
  await expect(page).toHaveURL(/\/ideas$/)
  await page.getByRole('navigation', { name: '主导航' }).getByRole('link', { name: '商品', exact: true }).click()
  await expect(search).toHaveValue('')
  await expect(page.locator('.product-active-filters')).toHaveCount(0)
  await expectPosition(page, 0)
})

for (const collection of [
  { path: 'tools', label: '道具', search: '搜索道具', count: 65, selector: '.product-catalog .product-card', back: '← 返回道具库', paginated: true },
  { path: 'ideas', label: '灵感', search: '搜索灵感', count: 45, selector: '.idea-card', back: '← 返回灵感库', paginated: true },
  { path: 'companies', label: '公司', search: '搜索公司', count: 30, selector: '.company-card', back: '← 返回公司库', paginated: false },
  { path: 'recipes', label: '配方', search: '搜索配方', count: 30, selector: '.recipe-catalog-item', back: '← 返回合成配方', paginated: false },
  { path: 'search', label: '搜索', search: '全局搜索关键词', count: 36, selector: '.search-section .product-card', back: '', paginated: false },
]) {
  test(`${collection.label}页面从详情返回保留关键词、已加载内容和浏览位置`, async ({ page }) => {
    await login(page)
    const create = await creator(page)
    const prefix = `返回位置${collection.label}`
    const names = Array.from({ length: collection.count }, (_, index) => `${prefix} ${String(index).padStart(3, '0')}`)
    for (const name of names) {
      if (collection.path === 'ideas') await create('ideas', { client_id: randomUUID(), content: name })
      else if (collection.path === 'recipes') {
        const item = await create('products', { chinese_name: name })
        await create('recipe-groups', { output_id: item.id })
      } else await create(collection.path === 'search' ? 'products' : collection.path, { chinese_name: name })
    }
    if (collection.path === 'ideas') {
      await expect.poll(async () => {
        const result = await (await page.request.get(`/api/ideas?q=${encodeURIComponent(prefix)}&limit=100`)).json()
        return result.items.filter((item: { ai_status: string }) => item.ai_status === 'succeeded').length
      }).toBe(collection.count)
    }
    if (collection.path === 'tools') await page.setViewportSize({ width: 390, height: 844 })
    await page.goto(collection.path === 'search' ? `/search?q=${encodeURIComponent(prefix)}&kind=products` : `/${collection.path}`)
    const search = page.getByRole('searchbox', { name: collection.search, exact: true })
    if (collection.path !== 'search') await search.fill(prefix)
    if (collection.path === 'tools') await page.getByRole('group', { name: '道具展示方式', exact: true }).getByRole('button', { name: '卡片', exact: true }).click()
    const rows = page.locator(collection.selector)
    await expect(rows.first()).toBeVisible()
    if (collection.paginated) {
      while (await rows.count() < collection.count) {
        const count = await rows.count()
        await page.getByRole('button', { name: /加载更多/ }).click()
        await expect.poll(() => rows.count()).toBeGreaterThan(count)
      }
    }
    await expect(rows).toHaveCount(collection.count)
    const target = page.getByRole('link', { name: names[3], exact: true }).first()
    await target.scrollIntoViewIfNeeded()
    const top = await page.evaluate(() => window.scrollY)
    expect(top).toBeGreaterThan(400)
    const detailPath = await target.getAttribute('href')
    await target.click()
    await expect(page).toHaveURL(url => url.pathname === detailPath)
    await expect(page.getByRole('heading', { name: names[3], exact: true })).toBeVisible()
    const freshList = page.waitForResponse(async response => {
      const path = collection.path === 'recipes' ? '/api/recipe-groups' : `/api/${collection.path}`
      const url = new URL(response.url())
      if (url.pathname !== path || url.searchParams.get('q') !== prefix || !response.ok()) return false
      await response.finished()
      return true
    })
    if (collection.back) await page.getByRole('link', { name: collection.back, exact: true }).click()
    else await page.goBack()
    await freshList
    await page.evaluate(() => new Promise(requestAnimationFrame))
    await expect(search).toHaveValue(prefix)
    await expect(rows).toHaveCount(collection.count)
    if (collection.path === 'tools') await expect(page.locator('.product-catalog')).toHaveClass(/is-grid/)
    await expectPosition(page, top)
    // Wait for the list's fresh response and a second detail round-trip, so
    // restoring cached markup alone cannot hide a first-page-only refresh.
    await target.click()
    await expect(page).toHaveURL(url => url.pathname === detailPath)
    await expect(page.getByRole('heading', { name: names[3], exact: true })).toBeVisible()
    await page.goBack()
    await expect(rows).toHaveCount(collection.count)
    await expectPosition(page, top)
  })
}
