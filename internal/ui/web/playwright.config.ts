import { defineConfig, devices } from '@playwright/test'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const rootDir = path.dirname(fileURLToPath(import.meta.url))

export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  workers: 1,
  retries: 0,
  forbidOnly: Boolean(process.env.CI),
  outputDir: path.join(rootDir, 'test-results'),
  reporter: [['list'], ['html', { open: 'never', outputFolder: path.join(rootDir, 'playwright-report') }]],
  use: {
    ...devices['Desktop Chrome'],
    baseURL: 'http://127.0.0.1:4173',
    channel: 'chrome',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'off',
    acceptDownloads: true,
  },
  projects: [
    {
      name: 'mock',
      testIgnore: /live\.spec\.ts/,
      use: {
        baseURL: 'http://127.0.0.1:4173',
      },
    },
    {
      name: 'live',
      testMatch: /live\.spec\.ts/,
      use: {
        baseURL: process.env.CAMUNDA_LAB_UI_URL ?? 'http://127.0.0.1:9090',
      },
    },
  ],
  webServer: {
    command: 'npm run dev -- --host 127.0.0.1 --port 4173',
    url: 'http://127.0.0.1:4173',
    reuseExistingServer: !process.env.CI,
  },
})
