import { test, expect, type Page } from '@playwright/test'

const sortLabels = {
  name_asc: '名称 · 正序', name_desc: '名称 · 倒序',
  updated_asc: '修改时间 · 正序', updated_desc: '修改时间 · 倒序',
  created_asc: '创建时间 · 正序', created_desc: '创建时间 · 倒序',
} as const

async function login(page: Page) {
  await page.goto('/login')
  await page.getByLabel('用户名').fill('admin')
  await page.getByLabel('密码', { exact: true }).fill('e2e-admin-password')
  await page.getByRole('button', { name: '登录', exact: true }).click()
  await expect(page).toHaveURL(/\/ideas$/)
}

async function selectSort(page: Page, value: keyof typeof sortLabels) {
  const sort = page.getByRole('combobox', { name: '商品排序', exact: true })
  await sort.click()
  await page.getByRole('option', { name: sortLabels[value], exact: true }).click()
  await expect(sort).toHaveAttribute('data-value', value)
}

test('六种商品排序保留搜索与筛选，记住选择并取消过期排序请求', async ({ page }) => {
  await login(page)
  const { csrf_token } = await (await page.request.get('/api/auth/me')).json()
  const names = ['排序界面 波板糖', '排序界面 阿梨', '排序界面 橙汽水']
  const products = []
  for (const chinese_name of names) {
    const response = await page.request.post('/api/products', { headers: { 'X-CSRF-Token': csrf_token }, data: { chinese_name } })
    expect(response.ok(), await response.text()).toBeTruthy()
    products.push(await response.json())
  }
  const changed = await page.request.patch(`/api/products/${products[1].id}`, { headers: { 'X-CSRF-Token': csrf_token }, data: { version: products[1].version, description: '最后修改的商品' } })
  expect(changed.ok(), await changed.text()).toBeTruthy()
  await page.goto('/products')
  const search = page.getByRole('searchbox', { name: '搜索商品', exact: true })
  await search.fill('排序界面')
  const titles = page.locator('.catalog-names h2 a')
  await expect(titles).toHaveCount(3)
  const sort = page.getByRole('combobox', { name: '商品排序', exact: true })
  await expect(sort).toHaveAttribute('data-value', 'created_desc')
  await expect(sort).toHaveText('创建时间 · 倒序')
  await expect(page.locator('.product-sort select')).toHaveCount(0)
  await sort.press('ArrowDown')
  await expect(page.getByRole('option')).toHaveCount(6)
  await expect(page.getByRole('option', { name: '创建时间 · 倒序', exact: true })).toBeFocused()
  await page.keyboard.press('ArrowUp')
  await expect(page.getByRole('option', { name: '修改时间 · 正序', exact: true })).toBeFocused()
  await page.keyboard.press('Escape')
  await expect(sort).toBeFocused()
  await expect(page.getByRole('option')).toHaveCount(0)
  for (const [value, order] of [
    ['name_asc', [1, 0, 2]], ['name_desc', [2, 0, 1]],
    ['updated_asc', [0, 2, 1]], ['updated_desc', [1, 2, 0]],
    ['created_asc', [0, 1, 2]], ['created_desc', [2, 1, 0]],
  ] as const) {
    const response = page.waitForResponse(response => {
      const url = new URL(response.url())
      return url.pathname === '/api/products' && url.searchParams.get('sort') === value && url.searchParams.get('offset') === '0'
    })
    await selectSort(page, value)
    expect((await response).ok()).toBeTruthy()
    await expect(titles).toHaveText(order.map(index => names[index]))
    await expect(search).toHaveValue('排序界面')
  }
  await selectSort(page, 'name_asc')
  await page.getByRole('button', { name: '筛选商品', exact: true }).click()
  const filter = page.getByRole('dialog', { name: '商品筛选', exact: true })
  await filter.getByRole('navigation').getByRole('button', { name: '制作状态', exact: true }).click()
  await filter.getByRole('button', { name: '构思中', exact: true }).click()
  await filter.getByRole('button', { name: '完成', exact: true }).click()
  await expect(titles).toHaveText([names[1], names[0], names[2]])
  await expect(sort).toHaveAttribute('data-value', 'name_asc')
  await page.getByRole('button', { name: '卡片', exact: true }).click()
  await expect(titles).toHaveText([names[1], names[0], names[2]])
  await page.setViewportSize({ width: 390, height: 844 })
  await expect(sort).toBeVisible()
  await sort.click()
  await expect(page.getByRole('listbox', { name: '商品排序方式', exact: true })).toBeVisible()
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBeTruthy()
  await page.screenshot({ path: '/tmp/com-products-sorting-mobile.png' })
  await page.keyboard.press('Escape')
  await page.reload()
  await expect(sort).toHaveAttribute('data-value', 'name_asc')
  await expect(page.getByRole('button', { name: '卡片', exact: true })).toHaveAttribute('aria-pressed', 'true')
  await search.fill('排序界面')
  await expect(titles).toHaveText([names[1], names[0], names[2]])

  let slowStarted = false
  let release!: () => void
  const gate = new Promise<void>(resolve => { release = resolve })
  await page.route('**/api/products?*', async route => {
    if (new URL(route.request().url()).searchParams.get('sort') !== 'name_desc') return route.continue()
    const response = await route.fetch()
    slowStarted = true
    await gate
    await route.fulfill({ response }).catch(() => {})
  })
  await selectSort(page, 'name_desc')
  await expect.poll(() => slowStarted).toBeTruthy()
  await selectSort(page, 'updated_desc')
  await expect(titles).toHaveText([names[1], names[2], names[0]])
  release()
  await page.waitForTimeout(300)
  await expect(titles).toHaveText([names[1], names[2], names[0]])
  await expect(sort).toHaveAttribute('data-value', 'updated_desc')
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.getByRole('button', { name: '列表', exact: true }).click()
  await page.screenshot({ path: '/tmp/com-products-sorting-desktop.png' })
})
