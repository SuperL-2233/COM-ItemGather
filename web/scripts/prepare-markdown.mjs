import { cp, mkdir } from 'node:fs/promises'
import { createRequire } from 'node:module'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

// Vditor loads its parser and optional renderers on demand. Serve those assets
// from this application, including in development, so private Markdown never
// needs a third-party CDN. Version the URL for the PWA's static cache.
const require = createRequire(import.meta.url)
const source = path.dirname(require.resolve('vditor/package.json'))
const { version } = require('vditor/package.json')
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const target = path.join(root, 'public', 'vendor', `vditor-${version}`, 'dist')
await mkdir(target, { recursive: true })
for (const directory of ['js', 'css', 'images']) {
  await cp(path.join(source, 'dist', directory), path.join(target, directory), { recursive: true })
}
