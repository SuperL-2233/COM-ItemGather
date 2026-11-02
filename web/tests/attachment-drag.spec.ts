import { test, expect } from '@playwright/test'
import { readFileSync } from 'node:fs'

const png = readFileSync('public/icon-192.png').toString('base64')

test('商品附件可一次拖入多张图片并忽略不支持的文件', async ({ page }) => {
  await page.goto('/login')
  await page.getByLabel('用户名').fill('admin')
  await page.getByLabel('密码', { exact: true }).fill('e2e-admin-password')
  await page.getByRole('button', { name: '登录', exact: true }).click()
  await expect(page).toHaveURL(/\/ideas$/)
  const { csrf_token } = await (await page.request.get('/api/auth/me')).json()
  const created = await page.request.post('/api/products', { headers: { 'X-CSRF-Token': csrf_token }, data: { chinese_name: '拖拽附件验收商品' } })
  expect(created.ok(), await created.text()).toBeTruthy()
  const product = await created.json()
  await page.goto(`/products/${product.id}`)
  const dropzone = page.getByRole('region', { name: '图片附件上传区域', exact: true })
  await expect(dropzone).toBeVisible()
  const dataTransfer = await page.evaluateHandle(({ png }) => {
    const transfer = new DataTransfer()
    for (const name of ['dragged-one.png', 'dragged-two.png']) {
      const bytes = Uint8Array.from(atob(png), character => character.charCodeAt(0))
      transfer.items.add(new File([bytes], name, { type: 'image/png' }))
    }
    transfer.items.add(new File(['not an image'], 'ignored.txt', { type: 'text/plain' }))
    return transfer
  }, { png })
  let uploads = 0
  page.on('request', request => {
    if (request.method() === 'POST' && new URL(request.url()).pathname === '/api/attachments') uploads++
  })
  let releaseFirst!: () => void
  const firstGate = new Promise<void>(resolve => { releaseFirst = resolve })
  let firstStarted = false
  await page.route('**/api/attachments', async route => {
    if (route.request().method() === 'POST' && !firstStarted) {
      firstStarted = true
      await firstGate
    }
    await route.continue()
  })
  await dropzone.dispatchEvent('dragenter', { dataTransfer })
  await expect(dropzone).toHaveClass(/dragging/)
  await dropzone.dispatchEvent('drop', { dataTransfer })
  await dataTransfer.dispose()
  await expect(dropzone).not.toHaveClass(/dragging/)
  await expect(page.getByRole('button', { name: '上传中…', exact: true })).toBeDisabled()
  releaseFirst()
  await expect(page.locator('.attachment-grid figure')).toHaveCount(2)
  await expect(page.getByRole('img', { name: 'dragged-one.png', exact: true })).toBeVisible()
  await expect(page.getByRole('img', { name: 'dragged-two.png', exact: true })).toBeVisible()
  await expect(page.getByRole('alert')).toContainText('其他文件未添加')
  expect(uploads).toBe(2)
  await expect(page.getByRole('button', { name: '+ 添加图片', exact: true })).toBeEnabled()
})
