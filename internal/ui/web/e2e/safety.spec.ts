import { expect, test } from '@playwright/test'

import { csrfToken, expectNoMutationBeforeConfirm } from './helpers/api'
import { prepareMockPage } from './helpers/page'

test('confirmation gates lab stop and restart', async ({ page }) => {
  const { monitor, mutations } = await prepareMockPage(page)
  await page.goto('/')
  await page.waitForLoadState('networkidle')
  await page.getByRole('button', { name: 'Stop lab' }).click()
  expect(mutations).toHaveLength(0)
  await page.getByRole('button', { name: 'Stop lab', exact: true }).last().click()
  await expect.poll(() => mutations.map((r) => new URL(r.url()).pathname)).toContain('/api/v1/down')
  const count = mutations.length
  await page.getByRole('button', { name: 'Restart lab' }).click()
  expect(mutations).toHaveLength(count)
  await page.getByRole('button', { name: 'Restart lab', exact: true }).last().click()
  await expect.poll(() => mutations.map((r) => new URL(r.url()).pathname)).toContain(
    '/api/v1/restart',
  )
  monitor.assertClean()
})

test('confirmation gates destructive version switch', async ({ page }) => {
  const { monitor, mutations } = await prepareMockPage(page)
  await page.goto('/setup')
  await page.waitForLoadState('networkidle')
  await expectNoMutationBeforeConfirm(
    page,
    mutations,
    /Move to 8\.10/,
    'Clear data and switch',
    '/api/v1/switch',
  )
  monitor.assertClean()
})

test('BPMN toolkit never renders a stale result under another tab', async ({ page }) => {
  let lintAborted = false
  page.on('requestfailed', (request) => {
    if (new URL(request.url()).pathname === '/api/v1/bpmn/lint') lintAborted = true
  })
  await page.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    if (path === '/api/v1/session') {
      await route.fulfill({ json: { csrfToken } })
      return
    }
    if (path === '/api/v1/bpmn/lint') {
      await new Promise((resolve) => setTimeout(resolve, 300))
      await route.fulfill({
        json: {
          ok: true,
          status: 'completed',
          complete: true,
          findings: [],
          warnings: [],
          output: 'STALE LINT RESULT',
        },
      })
      return
    }
    await route.fulfill({ json: { ok: true } })
  })

  await page.goto('/bpmn')
  await page.getByLabel('Or absolute path').fill('/tmp/process.bpmn')
  await page.getByRole('button', { name: 'Run' }).click()
  await page.getByRole('tab', { name: 'Diff' }).click()
  await expect.poll(() => lintAborted).toBe(true)
  await expect(page.getByText('STALE LINT RESULT')).toHaveCount(0)
  await expect(page.getByText('Choose inputs and run the selected developer tool.')).toBeVisible()
})

test('BPMN toolkit disables Run while a request is in flight', async ({ page }) => {
  await page.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    if (path === '/api/v1/session') {
      await route.fulfill({ json: { csrfToken } })
      return
    }
    if (path === '/api/v1/bpmn/lint') {
      await new Promise((resolve) => setTimeout(resolve, 500))
      await route.fulfill({ json: { ok: true, output: 'done' } })
      return
    }
    await route.fulfill({ json: { ok: true } })
  })

  await page.goto('/bpmn')
  await page.getByPlaceholder('/Users/…/process.bpmn').fill('/tmp/process.bpmn')
  const runButton = page.getByRole('button', { name: 'Run' })
  await runButton.click()
  await expect(page.getByRole('button', { name: 'Running…' })).toBeDisabled()
  await expect(page.getByText('Running lint…')).toBeVisible()
  await expect(runButton).toBeEnabled({ timeout: 3000 })
})

test('BPMN toolkit aborts an active request on unmount', async ({ page }) => {
  let aborted = false
  page.on('requestfailed', (request) => {
    if (new URL(request.url()).pathname === '/api/v1/bpmn/lint') aborted = true
  })
  await page.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    if (path === '/api/v1/session') {
      await route.fulfill({ json: { csrfToken } })
      return
    }
    if (path === '/api/v1/bpmn/lint') {
      await new Promise((resolve) => setTimeout(resolve, 500))
      await route.fulfill({ json: { ok: true, output: 'late result' } })
      return
    }
    await route.fulfill({ json: { ok: true } })
  })

  await page.goto('/bpmn')
  await page.getByLabel('Or absolute path').fill('/tmp/process.bpmn')
  await page.getByRole('button', { name: 'Run' }).click()
  await page.goto('/')
  await expect.poll(() => aborted).toBe(true)
})
