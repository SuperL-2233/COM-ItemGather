import { defineConfig } from '@playwright/test'

const baseURL = process.env.E2E_BASE_URL || 'http://127.0.0.1:18088'

export default defineConfig({
  testDir: './tests',
  testMatch: '**/*.spec.ts',
  globalSetup: './tests/global-setup.ts',
  fullyParallel: false,
  workers: 1,
  timeout: 60000,
  expect: { timeout: 15000 },
  reporter: 'list',
  use: {
    baseURL,
    headless: true,
    actionTimeout: 10000,
    navigationTimeout: 15000,
    trace: 'retain-on-failure',
  },
  webServer: {
    command: 'node tests/server.mjs',
    url: `${baseURL}/api/health`,
    reuseExistingServer: false,
    timeout: 30000,
  },
})
