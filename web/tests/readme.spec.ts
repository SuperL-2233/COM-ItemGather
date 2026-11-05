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

for (const newline of ['\n', '\r\n']) {
  test(`编辑和撤销只改变选中的原文，保留空行和表格 ${JSON.stringify(newline)}`, async ({ page }, testInfo) => {
    await login(page)
    const original = ['# 原文保真验收', '', '上段', '', '', '下段', '', '', '', '只修改这个词', '', '| 名称 |价格|', '|:-----|---:|', '|苹果|  10 |', '', '末尾保留空格  ', '', ''].join(newline)
    await seedDraft(page, original)
    await page.goto('/readme')
    await expect(textbox(page)).toBeVisible()
    await expect(page.locator('.markdown-rendered table')).toBeVisible()
    const lines = await page.locator('.cm-line').allTextContents()
    expect(lines.slice(lines.indexOf('上段') + 1, lines.indexOf('下段'))).toEqual(['', ''])
    const before = await page.locator('.cm-line').filter({ hasText: /^上段$/ }).boundingBox()
    const after = await page.locator('.cm-line').filter({ hasText: /^下段$/ }).boundingBox()
    expect(after!.y - before!.y).toBeGreaterThan(before!.height * 2.9)
    await expect(page.locator('.readme-status')).not.toContainText('有未保存的修改')
    await page.screenshot({ path: testInfo.outputPath('readme-source-fidelity.png'), fullPage: true })
    await page.locator('.cm-line').filter({ hasText: /^只修改这个词$/ }).click()
    await page.keyboard.press('End')
    await page.keyboard.insertText('已修改')
    await page.keyboard.press('Control+s')
    const changed = original.replace('只修改这个词', '只修改这个词已修改')
    await expect.poll(async () => (await (await page.request.get('/api/readme')).json()).draft.content).toBe(changed)
    await page.locator('.markdown-rendered table').click()
    await page.locator('.cm-line').filter({ hasText: /^\|苹果\|/ }).click()
    await page.keyboard.press('Home')
    await page.keyboard.press('ArrowRight')
    await page.keyboard.press('Shift+ArrowRight')
    await page.keyboard.press('Shift+ArrowRight')
    await page.keyboard.insertText('青梨')
    await page.keyboard.press('Control+s')
    await expect.poll(async () => (await (await page.request.get('/api/readme')).json()).draft.content).toBe(changed.replace('|苹果|', '|青梨|'))
    await expect(textbox(page)).toHaveAttribute('contenteditable', 'true')
    await textbox(page).focus()
    await page.keyboard.press('Control+z')
    await page.keyboard.press('Control+s')
    await expect.poll(async () => (await (await page.request.get('/api/readme')).json()).draft.content).toBe(changed)
    await expect(textbox(page)).toHaveAttribute('contenteditable', 'true')
    await textbox(page).focus()
    await page.keyboard.press('Control+Shift+Z')
    await page.keyboard.press('Control+s')
    await expect.poll(async () => (await (await page.request.get('/api/readme')).json()).draft.content).toBe(changed.replace('|苹果|', '|青梨|'))
    await expect(textbox(page)).toHaveAttribute('contenteditable', 'true')
    await textbox(page).focus()
    await page.keyboard.press('Control+z')
    await page.keyboard.press('Control+s')
    await expect.poll(async () => (await (await page.request.get('/api/readme')).json()).draft.content).toBe(changed)
    await page.getByRole('button', { name: '即时预览', exact: true }).click()
    await page.getByRole('button', { name: '源码', exact: true }).click()
    await expect(page.locator('.readme-status')).not.toContainText('有未保存的修改')
    await page.reload()
    await expect(textbox(page)).toBeVisible()
    expect((await (await page.request.get('/api/readme')).json()).draft.content).toBe(changed)
  })
}

test('预览在段落、标题、表格、代码和 HTML 块之间逐行显示空行', async ({ page }, testInfo) => {
  await login(page)
  const blocks = [
    { before: '前段', after: '后段', selector: '.cm-line', first: /^前段$/, last: /^后段$/ },
    { before: '## 前标题', after: '## 后标题', selector: '.markdown-heading', first: /前标题/, last: /后标题/ },
    { before: '| 前表 |\n|---|\n| 甲 |', after: '| 后表 |\n|---|\n| 乙 |', selector: '.markdown-rendered table', first: /前表/, last: /后表/ },
    { before: '```txt\n前代码\n```', after: '```txt\n后代码\n```', selector: '.markdown-rendered pre', first: /前代码/, last: /后代码/ },
    { before: '<div>前块</div>', after: '<div>后块</div>', selector: '.markdown-rendered', first: /前块/, last: /后块/ },
  ]
  for (const block of blocks) {
    for (const blankLines of [1, 2, 3]) {
      const original = block.before + '\n'.repeat(blankLines + 1) + block.after
      await seedDraft(page, original)
      await page.goto('/readme')
      await expect(textbox(page)).toBeVisible()
      const first = page.locator(block.selector).filter({ hasText: block.first })
      const last = page.locator(block.selector).filter({ hasText: block.last })
      await expect(first).toBeVisible()
      await expect(last).toBeVisible()
      const top = await first.boundingBox()
      const bottom = await last.boundingBox()
      const lineHeight = await page.locator('.cm-scroller').evaluate(element => parseFloat(getComputedStyle(element).lineHeight))
      expect(bottom!.y - top!.y - top!.height, `${block.before}: ${blankLines} 个空行`).toBeGreaterThanOrEqual(blankLines * lineHeight - 1)
      if (block.before === '前段' && blankLines === 2) await page.screenshot({ path: testInfo.outputPath('readme-two-blank-lines.png'), fullPage: true })
    }
  }
})

test('Markdown 正文填满编辑区域，长文末尾可见并可继续编辑', async ({ page }, testInfo) => {
  await login(page)
  await seedDraft(page, '# 编辑区域高度验收\n\n' + Array.from({ length: 60 }, (_, index) => `第 ${index + 1} 段：长文内容应正常滚动。\n\n`).join('') + '## 最后一章\n\n文档末尾标记\n')
  await page.goto('/readme')
  const field = textbox(page)
  await expect(field).toBeVisible()
  const fillsHeight = () => field.evaluate(element => ({ height: element.getBoundingClientRect().height, available: element.closest('.cm-scroller')!.getBoundingClientRect().height }))
  const initial = await fillsHeight()
  expect(initial.available).toBeGreaterThan(400)
  expect(initial.height).toBeGreaterThanOrEqual(initial.available - 1)
  await page.screenshot({ path: testInfo.outputPath('readme-height-fixed.png'), fullPage: true })
  await page.getByRole('navigation', { name: '章节导航', exact: true }).getByRole('button', { name: '最后一章', exact: true }).click()
  await expect.poll(() => field.evaluate(element => {
    const last = element.lastElementChild!.getBoundingClientRect()
    const bounds = element.closest('.cm-scroller')!.getBoundingClientRect()
    return last.bottom <= bounds.bottom && last.top >= bounds.top
  })).toBeTruthy()
  await appendText(page, '底部继续编辑验收')
  await page.keyboard.press('Control+s')
  await expect(page.locator('.readme-status')).not.toContainText('有未保存的修改')
  await expect.poll(async () => (await (await page.request.get('/api/readme')).json()).draft.content).toContain('底部继续编辑验收')
  await page.getByRole('button', { name: '全屏', exact: true }).click()
  const fullscreen = await fillsHeight()
  expect(fullscreen.height).toBeGreaterThanOrEqual(fullscreen.available - 1)
  await page.getByRole('button', { name: '全屏', exact: true }).click()
  await page.getByRole('button', { name: '源码', exact: true }).click()
  const source = await fillsHeight()
  expect(source.height).toBeGreaterThanOrEqual(source.available - 1)
  await page.setViewportSize({ width: 390, height: 844 })
  const mobile = await fillsHeight()
  expect(mobile.height).toBeGreaterThanOrEqual(mobile.available - 1)
})

test('读我客户端编辑、目录和草稿按账号隔离，作者在设置维护并通过下拉选择', async ({ page }, testInfo) => {
  await login(page)
  await seedDraft(page, '# 游戏编辑验收\n\n## 世界观\n\n这是 **游戏设定**。\n\n' + Array.from({ length: 20 }, (_, index) => `世界观段落 ${index}。\n\n`).join('') + '## 操作说明\n\n- 移动\n- 交互\n\n```go\nfmt.Println("hello")\n```\n')
  const externalRequests: string[] = []
  page.on('request', request => { if (new URL(request.url()).origin !== new URL(page.url()).origin) externalRequests.push(request.url()) })
  await page.getByRole('navigation', { name: '主导航' }).getByRole('link', { name: '读我', exact: true }).click()
  const labels = await page.getByRole('navigation', { name: '主导航' }).getByRole('link').allTextContents()
  expect(labels.slice(labels.indexOf('公司'), labels.indexOf('搜索') + 1)).toEqual(['公司', '读我', '搜索'])
  await expect(textbox(page)).toBeVisible()
  await expect(page.locator('.cm-content [role="heading"][aria-level="1"]')).toContainText('游戏编辑验收')
  const outline = page.getByRole('navigation', { name: '章节导航', exact: true })
  await expect(outline).toBeVisible()
  await expect(outline).toContainText('世界观')
  await expect(page.locator('.cm-content')).toContainText('游戏设定')
  await page.screenshot({ path: testInfo.outputPath('readme-desktop.png'), fullPage: true })
  const chapter = outline.getByRole('button', { name: '操作说明', exact: true })
  await chapter.focus()
  await chapter.press('Enter')
  await expect.poll(() => page.locator('.markdown-editor .cm-scroller').evaluate(element => element.scrollTop)).toBeGreaterThan(0)

  // Save immediately after input, without waiting for the next render.
  await appendText(page, '末尾输入也必须完整保存')
  await page.keyboard.press('Control+s')
  await expect(page.getByRole('status').filter({ hasText: '草稿已保存，尚未推送到 GitHub' })).toBeVisible()
  const saved = await (await page.request.get('/api/readme')).json()
  expect(saved.draft.content).toContain('末尾输入也必须完整保存')
  const popup = page.waitForEvent('popup')
  await page.getByRole('link', { name: '管理作者 ↗', exact: true }).click()
  const settings = await popup
  await expect(settings).toHaveURL(/\/settings\?tab=readme-authors$/)
  for (const [name, email] of [['作者甲', '123+author@users.noreply.github.com'], ['作者乙', '456+coauthor@users.noreply.github.com']]) {
    await settings.getByLabel('作者姓名', { exact: true }).fill(name)
    await settings.getByLabel('GitHub 提交邮箱', { exact: true }).fill(email)
    await settings.getByRole('button', { name: '添加作者', exact: true }).click()
    await expect(settings.getByRole('listitem').filter({ hasText: email })).toBeVisible()
  }
  await settings.close()
  await page.bringToFront()
  const choice = page.getByRole('combobox', { name: '提交作者', exact: true })
  await expect(choice.getByRole('option', { name: '作者乙 · 456+coauthor@users.noreply.github.com', exact: true })).toBeAttached()
  await choice.selectOption({ label: '作者乙 · 456+coauthor@users.noreply.github.com' })
  await expect(page.getByRole('button', { name: '保存', exact: true })).toHaveText('保存')
  await page.reload()
  await textbox(page).click()
  await page.keyboard.press('Control+End')
  await expect(textbox(page)).toContainText('末尾输入也必须完整保存')
  await expect(choice.locator('option:checked')).toHaveText('作者乙 · 456+coauthor@users.noreply.github.com')
  expect(externalRequests).toEqual([])

  await page.getByRole('button', { name: '退出', exact: true }).click()
  await page.getByLabel('用户名').fill('member')
  await page.getByLabel('密码', { exact: true }).fill('e2e-member-password')
  await page.getByRole('button', { name: '登录', exact: true }).click()
  await expect(page).toHaveURL(/\/ideas$/)
  await page.goto('/readme')
  await expect(textbox(page)).not.toContainText('末尾输入也必须完整保存')
  await expect(choice).toHaveValue('')
  await expect(choice.getByRole('option', { name: '作者甲 · 123+author@users.noreply.github.com', exact: true })).toBeAttached()
})

test('远端冲突保留编辑内容，手动合并后再直推 main', async ({ page }, testInfo) => {
  await login(page)
  let draft = { content: '# 游戏说明\n\n原版正文。\n', base_content: '# 游戏说明\n\n原版正文。\n', base_sha: 'base-sha', version: 1, updated_at: '2026-11-05T00:00:00Z', last_commit_url: '' }
  const remote = { content: '# 游戏说明\r\n\r\n\r\n其他成员更新。  \r\n\r\n', sha: 'remote-sha' }
  const authors = [{ id: 'author-a', name: '作者甲', email: '123+author@users.noreply.github.com', version: 1 }, { id: 'author-b', name: '作者乙', email: '456+coauthor@users.noreply.github.com', version: 2 }]
  let selectedAuthorID = authors[0].id
  let pushes = 0
  await page.route(/\/api\/readme(?:\/(?:push|rebase|author|authors))?$/, async route => {
    const request = route.request()
    const pathname = new URL(request.url()).pathname
    const body = request.method() === 'GET' ? undefined : request.postDataJSON()
    if (pathname.endsWith('/authors')) { await route.fulfill({ json: { items: authors, selected_author_id: selectedAuthorID } }); return }
    if (pathname.endsWith('/author')) { selectedAuthorID = body.author_id; await route.fulfill({ json: { selected_author_id: selectedAuthorID } }); return }
    if (pathname.endsWith('/push')) {
      pushes++
      expect(body.version).toBe(draft.version)
      expect(body.author_id).toBe('author-b')
      expect(body.author_version).toBe(2)
      if (pushes === 1) { await route.fulfill({ status: 409, json: { error: { code: 'readme_remote_conflict', message: '远端已更新，草稿已保留', details: { remote } } } }); return }
      draft = { ...draft, base_content: draft.content, base_sha: 'pushed-sha', version: draft.version + 1, last_commit_url: 'https://github.com/SuperL-2233/CallOfMommy/commit/aaaaaaaa' }
      await route.fulfill({ json: draft }); return
    }
    if (pathname.endsWith('/rebase')) {
      expect(body.remote_sha).toBe(remote.sha)
      expect(body.content).toBe(remote.content + '我新增的内容\r\n')
      draft = { ...draft, content: body.content, base_content: remote.content, base_sha: remote.sha, version: draft.version + 1 }
      await route.fulfill({ json: draft }); return
    }
    if (request.method() === 'PUT') {
      draft = { ...draft, content: body.content, version: draft.version + 1 }
      await route.fulfill({ json: draft }); return
    }
    await route.fulfill({ json: { draft, authors, selected_author_id: selectedAuthorID, github: { configured: true, repository: 'SuperL-2233/CallOfMommy', branch: 'main', path: 'README.md' }, remote_error: '' } })
  })
  await page.goto('/readme')
  await expect(textbox(page)).toBeVisible()
  await page.getByRole('combobox', { name: '提交作者', exact: true }).selectOption('author-b')
  await expect(page.getByRole('button', { name: '保存', exact: true })).toHaveText('保存')
  await page.screenshot({ path: testInfo.outputPath('readme-layout-desktop.png'), fullPage: true })
  await appendText(page, '我新增的内容')
  await page.getByRole('button', { name: '提交推送', exact: true }).click()
  await expect(page.getByRole('dialog', { name: '提交并推送 README', exact: true })).toContainText('main')
  await expect(page.getByRole('dialog', { name: '提交并推送 README', exact: true })).toContainText('456+coauthor@users.noreply.github.com')
  await page.getByRole('button', { name: '确认提交推送', exact: true }).click()
  await expect(page.getByRole('dialog', { name: '比较远端并合并', exact: true })).toBeVisible()
  await expect(page.getByLabel('GitHub 最新版本', { exact: true })).toHaveValue(remote.content.replace(/\r\n/g, '\n'))
  const merged = page.getByRole('textbox', { name: '合并后的 Markdown', exact: true })
  await expect(merged).toContainText('我新增的内容')
  await page.getByRole('button', { name: '将最新全文填入右侧', exact: true }).click()
  await merged.click()
  await page.keyboard.press('Control+End')
  await page.keyboard.insertText('我新增的内容')
  await page.keyboard.press('Enter')
  await page.getByRole('button', { name: '保存合并草稿', exact: true }).click()
  await expect(page.getByRole('dialog')).toHaveCount(0)
  expect(pushes).toBe(1)
  await page.getByRole('button', { name: '提交推送', exact: true }).click()
  await page.getByRole('button', { name: '确认提交推送', exact: true }).click()
  await expect(page.getByRole('link', { name: '查看上次提交 ↗', exact: true })).toHaveAttribute('href', /github.com\/SuperL-2233\/CallOfMommy\/commit\//)
  await expect(page.getByRole('button', { name: '提交推送', exact: true })).toBeDisabled()
  expect(pushes).toBe(2)
})

test('设置中可编辑删除作者，读我同步更新选项且删除后不会自动换用其他作者', async ({ page }) => {
  await login(page)
  const { csrf_token } = await (await page.request.get('/api/auth/me')).json()
  const retained = await page.request.post('/api/readme/authors', { headers: { 'X-CSRF-Token': csrf_token }, data: { name: '保留作者管理验收', email: 'retained-author@example.com' } })
  expect(retained.ok()).toBeTruthy()
  const remaining = (await (await page.request.get('/api/readme/authors')).json()).items.length
  const response = await page.request.post('/api/readme/authors', { headers: { 'X-CSRF-Token': csrf_token }, data: { name: '作者管理验收', email: 'manage-author@example.com' } })
  expect(response.ok()).toBeTruthy()
  const author = await response.json()
  const selected = await page.request.put('/api/readme/author', { headers: { 'X-CSRF-Token': csrf_token }, data: { author_id: author.id, author_version: author.version } })
  expect(selected.ok()).toBeTruthy()
  await page.goto('/readme')
  const choice = page.getByRole('combobox', { name: '提交作者', exact: true })
  await expect(choice).toHaveValue(author.id)
  const popup = page.waitForEvent('popup')
  await page.getByRole('link', { name: '管理作者 ↗', exact: true }).click()
  const settings = await popup
  await settings.getByRole('button', { name: '编辑作者 作者管理验收', exact: true }).click()
  await settings.getByLabel('作者姓名', { exact: true }).fill('作者管理验收更新')
  await settings.getByLabel('GitHub 提交邮箱', { exact: true }).fill('managed-updated@example.com')
  await settings.getByRole('button', { name: '保存作者', exact: true }).click()
  await expect(settings.getByRole('listitem').filter({ hasText: '作者管理验收更新' })).toContainText('managed-updated@example.com')
  await page.bringToFront()
  await expect(choice.locator('option:checked')).toHaveText('作者管理验收更新 · managed-updated@example.com')
  await settings.bringToFront()
  settings.once('dialog', dialog => dialog.accept())
  await settings.getByRole('button', { name: '删除作者 作者管理验收更新', exact: true }).click()
  await expect(settings.getByRole('listitem').filter({ hasText: 'managed-updated@example.com' })).toHaveCount(0)
  await settings.close()
  await page.bringToFront()
  await expect(choice).toHaveValue('')
  await expect(choice.locator('option')).toHaveCount(remaining + 1)
  await page.reload()
  await expect(choice).toHaveValue('')
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
  await expect(page.locator('.cm-content [onerror], .cm-content [onload], .cm-content a[href^="javascript:"]')).toHaveCount(0)
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
