import { spawn, spawnSync } from 'node:child_process'
import { createServer } from 'node:http'
import { mkdtempSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import path from 'node:path'

const root = path.resolve('..')
const baseURL = process.env.E2E_BASE_URL || 'http://127.0.0.1:18088'
const data = mkdtempSync(process.env.E2E_DATA_PREFIX || path.join(tmpdir(), 'com-itemgather-e2e-'))
const password = 'e2e-admin-password'
const environment = {
  ...process.env,
  APP_ENV: 'test',
  APP_LISTEN_ADDR: new URL(baseURL).host,
  APP_PUBLIC_URL: baseURL,
  COOKIE_SECURE: 'false',
  DATABASE_PATH: path.join(data, 'app.db'),
  ATTACHMENT_DIR: path.join(data, 'attachments'),
  BACKUP_DIR: path.join(data, 'backups'),
  BACKUP_INTERVAL: '1h',
  WEB_DIST: path.join(root, 'web', 'dist'),
  AI_API_KEY: 'local-e2e-key',
  AI_MODEL: 'local-e2e-model',
  AI_TIMEOUT: '3s',
  AI_MAX_RETRIES: '1',
  AI_WORKER_CONCURRENCY: '1',
}
const failedCalls = new Map()
const mock = createServer((request, response) => {
  let body = ''
  request.on('data', chunk => { body += chunk })
  request.on('end', () => {
    if (request.headers.authorization !== 'Bearer local-e2e-key') {
      response.writeHead(401).end()
      return
    }
    const payload = JSON.parse(body)
    const content = payload.messages.at(-1).content
    if (content.startsWith('整理重试验收') && (failedCalls.get(content) || 0) < 2) {
      failedCalls.set(content, (failedCalls.get(content) || 0) + 1)
      response.writeHead(500).end()
      return
    }
    const soda = content === '一款叫绿色风暴的青苹果汽水' || content.startsWith('分类精简验收')
    const nameOnly = content === '绿色风暴' || content.endsWith('先记下这个名字')
    const result = {
      display_title: content === '一款叫绿色风暴的青苹果汽水' ? '绿色风暴' : content.slice(0, 80),
      summary: soda ? '青苹果饮料' : nameOnly ? '' : content.slice(0, 200),
      idea_types: soda ? ['商品概念'] : nameOnly ? ['名称'] : [],
      product_directions: soda ? ['饮料'] : [],
    }
    response.writeHead(200, { 'Content-Type': 'application/json' })
    response.end(JSON.stringify({ choices: [{ message: { content: JSON.stringify(result) }, finish_reason: 'stop' }] }))
  })
})
await new Promise((resolve, reject) => {
  mock.once('error', reject)
  mock.listen(0, '127.0.0.1', resolve)
})
environment.AI_BASE_URL = `http://127.0.0.1:${mock.address().port}`
const initialized = spawnSync(path.join(root, 'bin', 'com-itemgather'), ['create-admin', '-username', 'admin', '-nickname', '管理员'], { env: { ...environment, ADMIN_PASSWORD: password }, encoding: 'utf8' })
if (initialized.status !== 0) {
  mock.close()
  rmSync(data, { recursive: true, force: true })
  throw new Error(`E2E admin setup failed: ${initialized.stderr}`)
}
const application = spawn(path.join(root, 'bin', 'com-itemgather'), ['serve'], { env: environment, stdio: ['ignore', 'pipe', 'pipe'] })
application.stderr.on('data', chunk => process.stderr.write(chunk))
let stopping = false
async function stop(code = 0) {
  if (stopping) return
  stopping = true
  application.kill('SIGTERM')
  mock.close()
  await new Promise(resolve => {
    const timer = setTimeout(() => { application.kill('SIGKILL'); resolve() }, 5000)
    application.once('exit', () => { clearTimeout(timer); resolve() })
  })
  rmSync(data, { recursive: true, force: true })
  process.exit(code)
}
application.once('error', error => { console.error(error); stop(1) })
application.once('exit', code => { if (!stopping && code !== 0) { console.error(`E2E backend exited with ${code}`); stop(1) } })
process.on('SIGINT', () => stop())
process.on('SIGTERM', () => stop())
process.on('uncaughtException', error => { console.error(error); stop(1) })
