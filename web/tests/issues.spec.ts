import { test, expect, type Page } from '@playwright/test'
import type { GitHubIssue, IssueComment } from '../src/issues'

const timestamp = '2026-11-09T08:00:00Z'
function issue(number = 7, overrides: Partial<GitHubIssue> = {}): GitHubIssue {
  return { number, title: '进入厨房时角色卡在门边', body: '## 复现步骤\n\n1. 进入厨房\n2. 与门交互\n\n**期望正常通过**。\n', state: 'open', state_reason: null, html_url: `https://github.com/SuperL-2233/CallOfMommy/issues/${number}`, user: { login: 'game-player' }, labels: [{ name: 'bug', color: 'd73a4a' }], assignees: [{ login: 'developer' }], comments: 2, locked: false, created_at: timestamp, updated_at: timestamp, ...overrides }
}
function comment(id: number, body = `第 ${id} 条评论`): IssueComment { return { id, body, html_url: `https://github.com/SuperL-2233/CallOfMommy/issues/7#issuecomment-${id}`, user: { login: 'game-player' }, created_at: timestamp, updated_at: timestamp } }
async function login(page: Page) {
  await page.goto('/login')
  await page.getByLabel('用户名').fill('admin')
  await page.getByLabel('密码', { exact: true }).fill('e2e-admin-password')
  await page.getByRole('button', { name: '登录', exact: true }).click()
  await expect(page).toHaveURL(/\/ideas$/)
}

test('问题位于读我右侧，按 GitHub 状态筛选、分页并读取详情和评论', async ({ page }, testInfo) => {
  await login(page)
  const states: string[] = []
  await page.route(/\/api\/issues(?:\/\d+(?:\/comments)?)?(?:\?.*)?$/, async route => {
    const url = new URL(route.request().url())
    if (url.pathname.endsWith('/comments')) {
      await route.fulfill({ json: { items: url.searchParams.get('page') === '2' ? [comment(3)] : [comment(1), comment(2)], next_page: url.searchParams.get('page') === '2' ? 0 : 2 } }); return
    }
    if (url.pathname === '/api/issues/9') { await route.fulfill({ json: { issue: issue(9, { title: '已修复的显示问题', state: 'closed' }), repository: 'SuperL-2233/CallOfMommy' } }); return }
    const state = url.searchParams.get('state') || 'open'; states.push(state)
    await route.fulfill({ json: { configured: true, repository: 'SuperL-2233/CallOfMommy', items: state === 'closed' ? [issue(9, { title: '已修复的显示问题', state: 'closed' })] : url.searchParams.get('page') === '2' ? [issue(8, { title: '新增饮料建议' })] : [issue()], next_page: state === 'open' && url.searchParams.get('page') !== '2' ? 2 : 0 } })
  })
  await page.getByRole('navigation', { name: '主导航' }).getByRole('link', { name: '问题', exact: true }).click()
  const labels = await page.getByRole('navigation', { name: '主导航' }).getByRole('link').allTextContents()
  expect(labels.slice(labels.indexOf('读我'), labels.indexOf('搜索') + 1)).toEqual(['读我', '问题', '搜索'])
  await expect(page.getByRole('link', { name: /进入厨房时角色卡在门边/ })).toBeVisible()
  await expect(page.locator('.issue-row')).toContainText('负责人 developer')
  await page.getByRole('button', { name: '加载更多问题', exact: true }).click()
  await expect(page.locator('.issue-row')).toHaveCount(2)
  await page.screenshot({ path: testInfo.outputPath('issues-desktop.png'), fullPage: true })
  await page.getByRole('button', { name: '已关闭', exact: true }).click()
  await expect(page).toHaveURL(/\/issues\?state=closed$/)
  await expect(page.locator('.issue-row')).toHaveCount(1)
  await page.getByRole('link', { name: /已修复的显示问题/ }).click()
  await expect(page.getByRole('heading', { name: '已修复的显示问题 #9' })).toBeVisible()
  await expect(page.getByRole('button', { name: '重新打开', exact: true })).toBeVisible()
  await expect(page.locator('.issue-description strong')).toHaveText('期望正常通过')
  await expect(page.locator('.issue-comment')).toHaveCount(2)
  await page.getByRole('button', { name: '加载更多评论', exact: true }).click()
  await expect(page.locator('.issue-comment')).toHaveCount(3)
  expect(states).toContain('closed')
})

test('问题与评论的 Markdown 预览不改写原文，失败保留内容，发布后可关闭和重新打开', async ({ page }, testInfo) => {
  await login(page)
  let current = issue(21)
  let creates = 0; let commentCreates = 0
  const original = '## 复现步骤\n\n\n**完整原文**  \n\n| 名称 |价格|\n|:-----|---:|\n|苹果|  10 |\n\n- [ ] 待办项\n- [x] 完成项\n\n<img src="x" onerror="window.issueXss=1">\n\n[坏链接](javascript:alert(1))\n'
  const commentSource = '## 补充\n\n\n保留两行间隔  \n'
  const comments: IssueComment[] = []
  const changes: string[] = []
  await page.route(/\/api\/issues(?:\/\d+(?:\/comments)?)?(?:\?.*)?$/, async route => {
    const request = route.request(); const url = new URL(request.url())
    if (request.method() === 'POST' && url.pathname === '/api/issues') {
      const body = request.postDataJSON(); expect(body).toEqual({ title: '原文完整的问题', body: original }); creates++
      if (creates === 1) { await route.fulfill({ status: 502, json: { error: { code: 'github_issues_permission', message: '请在安装设置中接受 Issues 新权限' } } }); return }
      current = issue(21, { title: body.title, body: body.body + '\n\n---\n通过游戏设定库发布 · 管理员', comments: 0 })
      await route.fulfill({ status: 201, json: current }); return
    }
    if (url.pathname.endsWith('/comments')) {
      if (request.method() === 'POST') {
        expect(request.postDataJSON()).toEqual({ body: commentSource }); commentCreates++
        if (commentCreates === 1) { await route.fulfill({ status: 502, json: { error: { code: 'github_unavailable', message: '连接 GitHub 失败，请核对后重试' } } }); return }
        const posted = comment(101, commentSource + '\n\n---\n通过游戏设定库发布 · 管理员'); comments.push(posted); current.comments++
        await route.fulfill({ status: 201, json: posted }); return
      }
      await route.fulfill({ json: { items: comments, next_page: 0 } }); return
    }
    if (request.method() === 'PATCH') { changes.push(request.postDataJSON().state); current = { ...current, state: request.postDataJSON().state }; await route.fulfill({ json: current }); return }
    await route.fulfill({ json: { issue: current, repository: 'SuperL-2233/CallOfMommy' } })
  })
  await page.goto('/issues/new')
  await page.getByLabel('标题', { exact: true }).fill('原文完整的问题')
  await page.getByRole('textbox', { name: '问题说明', exact: true }).fill(original)
  await page.getByRole('button', { name: '预览', exact: true }).click()
  await expect(page.getByLabel('问题说明预览').locator('strong')).toHaveText('完整原文')
  await expect(page.getByLabel('问题说明预览').getByRole('checkbox', { name: '已完成', exact: true })).toHaveAttribute('aria-checked', 'true')
  await expect(page.getByLabel('问题说明预览').getByRole('checkbox', { name: '未完成', exact: true })).toHaveAttribute('aria-checked', 'false')
  await expect(page.locator('.issue-markdown [onerror], .issue-markdown a[href^="javascript:"]')).toHaveCount(0)
  expect(await page.evaluate(() => (window as Window & { issueXss?: number }).issueXss)).toBeUndefined()
  await page.getByRole('button', { name: '编写', exact: true }).click()
  await expect(page.getByRole('textbox', { name: '问题说明', exact: true })).toHaveValue(original)
  await page.getByRole('button', { name: '发布问题', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('Issues 新权限')
  await expect(page.getByRole('textbox', { name: '问题说明', exact: true })).toHaveValue(original)
  await page.getByRole('button', { name: '发布问题', exact: true }).click()
  await expect(page).toHaveURL(/\/issues\/21$/)
  await expect(page.getByRole('heading', { name: '原文完整的问题 #21' })).toBeVisible()
  await page.getByRole('textbox', { name: '评论内容', exact: true }).fill(commentSource)
  await page.getByRole('button', { name: '预览', exact: true }).click()
  await expect(page.getByLabel('评论内容预览').getByRole('heading', { name: '补充', exact: true })).toBeVisible()
  await page.getByRole('button', { name: '发布评论', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('核对后重试')
  await page.getByRole('button', { name: '编写', exact: true }).click()
  await expect(page.getByRole('textbox', { name: '评论内容', exact: true })).toHaveValue(commentSource)
  await page.getByRole('button', { name: '发布评论', exact: true }).click()
  await expect(page.locator('.issue-comment')).toContainText('通过游戏设定库发布 · 管理员')
  await expect(page.getByRole('textbox', { name: '评论内容', exact: true })).toHaveValue('')
  await page.getByRole('button', { name: '关闭问题', exact: true }).click()
  await expect(page.locator('.issue-state-badge')).toHaveText('已关闭')
  await page.getByRole('button', { name: '重新打开', exact: true }).click()
  await expect(page.locator('.issue-state-badge')).toHaveText('待处理')
  expect(changes).toEqual(['closed', 'open'])
  await page.screenshot({ path: testInfo.outputPath('issue-detail-desktop.png'), fullPage: true })
})

test('手机问题页不横向溢出，未发布评论离开前确认，锁定讨论不显示评论输入框', async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await login(page)
  let locked = false
  await page.route(/\/api\/issues\/7(?:\/comments)?(?:\?.*)?$/, async route => {
    if (new URL(route.request().url()).pathname.endsWith('/comments')) { await route.fulfill({ json: { items: [], next_page: 0 } }); return }
    await route.fulfill({ json: { issue: issue(7, { locked, title: '很长的问题标题用于检查手机屏幕下正常换行，不挤出页面边界', body: '| A | B |\n|---|---|\n| 长表格内容 | 另一列 |\n\n```txt\n' + 'very-long-code-line-'.repeat(20) + '\n```' }), repository: 'SuperL-2233/CallOfMommy' } })
  })
  await page.goto('/issues/7')
  await expect(page.getByRole('textbox', { name: '评论内容', exact: true })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBeTruthy()
  await page.screenshot({ path: testInfo.outputPath('issue-detail-mobile.png'), fullPage: true })
  await page.getByRole('textbox', { name: '评论内容', exact: true }).fill('还没发布的内容')
  page.once('dialog', dialog => dialog.dismiss())
  await page.getByRole('link', { name: '← 返回问题', exact: true }).click()
  await expect(page).toHaveURL(/\/issues\/7$/)
  await expect(page.getByRole('textbox', { name: '评论内容', exact: true })).toHaveValue('还没发布的内容')
  await page.getByRole('textbox', { name: '评论内容', exact: true }).fill('')
  locked = true
  await page.reload()
  await expect(page.locator('.issue-notice')).toContainText('已锁定讨论')
  await expect(page.getByRole('textbox', { name: '评论内容', exact: true })).toHaveCount(0)
})

test('尚未配置 App 时给出设置提示，问题页仍需网页登录', async ({ page }) => {
  await page.goto('/issues')
  await expect(page).toHaveURL(/\/login\?redirect=/)
  await login(page)
  await page.route(/\/api\/issues\?/, route => route.fulfill({ json: { configured: false, repository: '', items: [], next_page: 0 } }))
  await page.goto('/issues')
  await expect(page.locator('.issue-notice')).toContainText('Issues 读写权限')
  await expect(page.getByRole('link', { name: '新建问题', exact: true })).toHaveCount(0)
})
