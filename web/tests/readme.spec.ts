import { test, expect, type Page } from '@playwright/test'

async function login(page: Page, member = false) {
  await page.goto('/login')
  await page.getByLabel('用户名').fill(member ? 'member' : 'admin')
  await page.getByLabel('密码', { exact: true }).fill(member ? 'e2e-member-password' : 'e2e-admin-password')
  await page.getByRole('button', { name: '登录', exact: true }).click()
  await expect(page).toHaveURL(/\/ideas$/)
}
async function seedDraft(page: Page, content: string) {
  const { csrf_token } = await (await page.request.get('/api/auth/me')).json()
  const { draft } = await (await page.request.get('/api/readme')).json()
  const response = await page.request.put('/api/readme', { headers: { 'X-CSRF-Token': csrf_token }, data: { version: draft.version, content } })
  expect(response.ok(), await response.text()).toBeTruthy()
}
const textbox = (page: Page) => page.getByRole('textbox', { name: 'README Markdown 编辑器', exact: true }).filter({ visible: true })
async function appendText(page: Page, value: string) {
  const field = textbox(page)
  await field.click()
  await page.keyboard.press('Control+End')
  await page.keyboard.press('Enter')
  await page.keyboard.insertText(value)
}

test('读我位于公司和搜索之间，客户端即时编辑、目录、保存与身份按账号隔离', async ({ page }, testInfo) => {
  await login(page)
  await seedDraft(page, '# 游戏编辑验收\n\n## 世界观\n\n这是 **游戏设定**。\n\n' + Array.from({ length: 20 }, (_, index) => `世界观段落 ${index}。\n\n`).join('') + '## 操作说明\n\n- 移动\n- 交互\n\n```go\nfmt.Println("hello")\n```\n')
  const externalRequests: string[] = []
  page.on('request', request => { if (new URL(request.url()).origin !== new URL(page.url()).origin) externalRequests.push(request.url()) })
  await page.getByRole('navigation', { name: '主导航' }).getByRole('link', { name: '读我', exact: true }).click()
  const labels = await page.getByRole('navigation', { name: '主导航' }).getByRole('link').allTextContents()
  expect(labels.slice(labels.indexOf('公司'), labels.indexOf('搜索') + 1)).toEqual(['公司', '读我', '搜索'])
  await expect(textbox(page)).toBeVisible()
  await expect(page.locator('.vditor-ir h1')).toContainText('游戏编辑验收')
  const outline = page.getByRole('navigation', { name: '章节导航', exact: true })
  await expect(outline).toBeVisible()
  await expect(outline).toContainText('世界观')
  await expect(page.locator('.vditor-ir strong')).toHaveText('游戏设定')
  await page.screenshot({ path: testInfo.outputPath('readme-desktop.png'), fullPage: true })
  const chapter = outline.getByRole('button', { name: '操作说明', exact: true })
  await chapter.focus()
  await chapter.press('Enter')
  await expect.poll(() => page.locator('.vditor-ir .vditor-reset').evaluate(element => element.scrollTop)).toBeGreaterThan(0)

  // Save immediately, before the editor's debounced input callback fires.
  await appendText(page, '末尾输入也必须完整保存')
  await page.keyboard.press('Control+s')
  await expect(page.getByRole('status').filter({ hasText: '草稿已保存，尚未推送到 GitHub' })).toBeVisible()
  const saved = await (await page.request.get('/api/readme')).json()
  expect(saved.draft.content).toContain('末尾输入也必须完整保存')
  await page.getByLabel('提交姓名', { exact: true }).fill('作者甲')
  await page.getByLabel('GitHub 提交邮箱', { exact: true }).fill('123+author@users.noreply.github.com')
  await page.getByRole('button', { name: '保存提交身份', exact: true }).click()
  await page.reload()
  await expect(textbox(page)).toContainText('末尾输入也必须完整保存')
  await expect(page.locator('.readme-author summary')).toContainText('123+author@users.noreply.github.com')
  expect(externalRequests).toEqual([])

  await page.getByRole('button', { name: '退出', exact: true }).click()
  await page.getByLabel('用户名').fill('member')
  await page.getByLabel('密码', { exact: true }).fill('e2e-member-password')
  await page.getByRole('button', { name: '登录', exact: true }).click()
  await expect(page).toHaveURL(/\/ideas$/)
  await page.goto('/readme')
  await expect(textbox(page)).not.toContainText('末尾输入也必须完整保存')
  await expect(page.getByLabel('GitHub 提交邮箱', { exact: true })).toHaveValue('')
})

test('远端冲突保留编辑内容，手动合并后再直推 main', async ({ page }) => {
  await login(page)
  let draft = { content: '# 游戏说明\n\n原版正文。\n', base_content: '# 游戏说明\n\n原版正文。\n', base_sha: 'base-sha', version: 1, updated_at: '2026-11-05T00:00:00Z', last_commit_url: '' }
  const remote = { content: '# 游戏说明\n\n其他成员更新。\n', sha: 'remote-sha' }
  let pushes = 0
  await page.route(/\/api\/readme(?:\/(?:push|rebase))?$/, async route => {
    const request = route.request()
    const pathname = new URL(request.url()).pathname
    const body = request.method() === 'GET' ? undefined : request.postDataJSON()
    if (pathname.endsWith('/push')) {
      pushes++
      expect(body.version).toBe(draft.version)
      if (pushes === 1) { await route.fulfill({ status: 409, json: { error: { code: 'readme_remote_conflict', message: '远端已更新，草稿已保留', details: { remote } } } }); return }
      draft = { ...draft, base_content: draft.content, base_sha: 'pushed-sha', version: draft.version + 1, last_commit_url: 'https://github.com/SuperL-2233/CallOfMommy/commit/aaaaaaaa' }
      await route.fulfill({ json: draft }); return
    }
    if (pathname.endsWith('/rebase')) {
      expect(body.remote_sha).toBe(remote.sha)
      expect(body.content).toContain('其他成员更新')
      expect(body.content).toContain('我新增的内容')
      draft = { ...draft, content: body.content, base_content: remote.content, base_sha: remote.sha, version: draft.version + 1 }
      await route.fulfill({ json: draft }); return
    }
    if (request.method() === 'PUT') {
      draft = { ...draft, content: body.content, version: draft.version + 1 }
      await route.fulfill({ json: draft }); return
    }
    await route.fulfill({ json: { draft, author: { name: '作者甲', email: '123+author@users.noreply.github.com' }, github: { configured: true, repository: 'SuperL-2233/CallOfMommy', branch: 'main', path: 'README.md' }, remote_error: '' } })
  })
  await page.goto('/readme')
  await expect(textbox(page)).toBeVisible()
  await appendText(page, '我新增的内容')
  await page.getByRole('button', { name: '提交推送', exact: true }).click()
  await expect(page.getByRole('dialog', { name: '提交并推送 README', exact: true })).toContainText('main')
  await page.getByRole('button', { name: '确认提交推送', exact: true }).click()
  await expect(page.getByRole('dialog', { name: '比较远端并合并', exact: true })).toBeVisible()
  await expect(page.getByLabel('GitHub 最新版本', { exact: true })).toHaveValue(remote.content)
  await expect(page.getByLabel('合并后的 Markdown', { exact: true })).toHaveValue(/我新增的内容/)
  await page.getByLabel('合并后的 Markdown', { exact: true }).fill(remote.content + '\n我新增的内容\n')
  await page.getByRole('button', { name: '保存合并草稿', exact: true }).click()
  await expect(page.getByRole('dialog')).toHaveCount(0)
  expect(pushes).toBe(1)
  await page.getByRole('button', { name: '提交推送', exact: true }).click()
  await page.getByRole('button', { name: '确认提交推送', exact: true }).click()
  await expect(page.getByRole('link', { name: '查看上次提交 ↗', exact: true })).toHaveAttribute('href', /github.com\/SuperL-2233\/CallOfMommy\/commit\//)
  await expect(page.getByRole('button', { name: '提交推送', exact: true })).toBeDisabled()
  expect(pushes).toBe(2)
})

test('移动端目录可展开，HTML 被过滤，未保存内容可在刷新后恢复', async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await login(page)
  await seedDraft(page, '# 移动端阅读\n\n## 章节一\n\n正文内容。\n\n<img src="x" onerror="window.readmeXss=1">\n\n<script>window.readmeXss=2</script>\n\n[不安全链接](javascript:alert(1))\n')
  await page.goto('/readme')
  await expect(textbox(page)).toBeVisible()
  await page.getByRole('button', { name: '☰ 章节导航', exact: true }).click()
  await expect(page.getByRole('navigation', { name: '章节导航', exact: true })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1)).toBeTruthy()
  expect(await page.evaluate(() => (window as Window & { readmeXss?: number }).readmeXss)).toBeUndefined()
  await expect(page.locator('.vditor-ir [onerror], .vditor-ir [onload], .vditor-ir a[href^="javascript:"]')).toHaveCount(0)
  await page.screenshot({ path: testInfo.outputPath('readme-mobile.png'), fullPage: true })
  await page.getByRole('button', { name: '☰ 章节导航', exact: true }).click()
  await appendText(page, '刷新恢复验收')
  await expect(page.locator('.readme-status')).toContainText('有未保存的修改')
  await page.waitForTimeout(850)
  page.on('dialog', dialog => dialog.accept())
  await page.reload()
  await expect(textbox(page)).toContainText('刷新恢复验收')
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('.readme-status')).not.toContainText('有未保存的修改')
})
