import { test, expect, type Page } from '@playwright/test'
import { randomUUID } from 'node:crypto'

async function signIn(page: Page) {
  await page.goto('/login')
  await page.getByLabel('用户名').fill('admin')
  await page.getByLabel('密码', { exact: true }).fill('e2e-admin-password')
  await page.getByRole('button', { name: '登录', exact: true }).click()
  await expect(page).toHaveURL(/\/ideas$/)
  const { csrf_token } = await (await page.request.get('/api/auth/me')).json()
  return { 'X-CSRF-Token': csrf_token }
}

test('灵感只保留类型和商品方向，重新生成后整组刷新卡片', async ({ page }) => {
  const headers = await signIn(page)
  const content = `分类精简验收 ${randomUUID()}：一款青苹果汽水`
  const created = await page.request.post('/api/ideas', { headers, data: { content, client_id: randomUUID() } })
  expect(created.ok()).toBeTruthy()
  const idea = await created.json()
  const card = page.locator(`.idea-card:has(.card-title[href="/ideas/${idea.id}"])`)
  await expect(card.locator('.tag')).toHaveText(['商品概念', '饮料'])
  await card.locator('.card-title').click()
  await expect(page.getByLabel('灵感类型', { exact: true })).toHaveValue('商品概念')
  await expect(page.getByLabel('商品方向', { exact: true })).toHaveValue('饮料')
  await expect(page.getByLabel('名称候选（每行一个）')).toHaveCount(0)
  await expect(page.getByLabel('标签', { exact: true })).toHaveCount(0)
  await expect(page.getByLabel('属性名', { exact: true })).toHaveCount(0)
  await expect(page.getByRole('button', { name: '+ 添加属性', exact: true })).toHaveCount(0)

  await page.getByLabel('灵感类型', { exact: true }).fill('包装, 世界观')
  await page.getByLabel('商品方向', { exact: true }).fill('乳制品, 零食')
  await page.getByLabel('补充备注', { exact: true }).fill('保留的人工备注')
  await page.getByRole('button', { name: '保存修改', exact: true }).click()
  await expect(page.getByRole('button', { name: '已保存', exact: true })).toBeVisible()
  await page.getByRole('link', { name: '← 返回灵感库', exact: true }).click()
  await expect(card.locator('.tag')).toHaveText(['包装', '世界观', '乳制品', '零食'])

  await card.locator('.card-title').click()
  page.once('dialog', dialog => dialog.accept())
  await page.getByRole('button', { name: '重新生成全部字段', exact: true }).click()
  await expect(page.getByLabel('灵感类型', { exact: true })).toHaveValue('商品概念')
  await expect(page.getByLabel('商品方向', { exact: true })).toHaveValue('饮料')
  await expect(page.getByLabel('补充备注', { exact: true })).toHaveValue('保留的人工备注')
  await page.getByRole('link', { name: '← 返回灵感库', exact: true }).click()
  await expect(card.locator('.tag')).toHaveText(['商品概念', '饮料'])

  const latest = await (await page.request.get(`/api/ideas/${idea.id}`)).json()
  for (const key of ['name_candidates', 'tags', 'attributes']) expect(latest).not.toHaveProperty(key)
  await page.getByRole('link', { name: /设置$/ }).click()
  await expect(page.getByRole('button', { name: '标签维护', exact: true })).toHaveCount(0)
})

test('只有名字的灵感不补商品方向，未分类灵感不显示分类小标签', async ({ page }) => {
  const headers = await signIn(page)
  for (const [content, expected] of [[`仅名称精简验收 ${randomUUID()}：雾夜长笛，先记下这个名字`, ['名称']], [`未分类精简验收 ${randomUUID()}`, []]] as const) {
    const response = await page.request.post('/api/ideas', { headers, data: { content, client_id: randomUUID() } })
    expect(response.ok()).toBeTruthy()
    const idea = await response.json()
    await expect.poll(async () => (await (await page.request.get(`/api/ideas/${idea.id}`)).json()).ai_status).toBe('succeeded')
    const card = page.locator(`.idea-card:has(.card-title[href="/ideas/${idea.id}"])`)
    await expect(card).toBeVisible()
    await expect(card.locator('.tag')).toHaveText([...expected])
    const result = await (await page.request.get(`/api/ideas/${idea.id}`)).json()
    expect(result.product_directions).toEqual([])
  }
})
