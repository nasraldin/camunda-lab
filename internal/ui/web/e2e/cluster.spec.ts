import { expect, test } from '@playwright/test'

import { expectNoMutationBeforeConfirm } from './helpers/api'
import { prepareMockPage } from './helpers/page'

test('cluster incidents list shows mocked incident row', async ({ page }) => {
  const { monitor } = await prepareMockPage(page)
  await page.goto('/cluster')
  await page.waitForLoadState('networkidle')
  await expect(page.locator('.url-row-label', { hasText: 'incident-1' })).toBeVisible()
  await expect(page.locator('.url-row-value', { hasText: 'job failed' })).toBeVisible()
  monitor.assertClean()
})

test('cluster incidents empty state', async ({ page }) => {
  const { monitor } = await prepareMockPage(page, { incidents: [] })
  await page.goto('/cluster')
  await page.waitForLoadState('networkidle')
  await expect(page.getByText('No incidents.')).toBeVisible()
  monitor.assertClean()
})

test('cluster trace success result', async ({ page }) => {
  const { monitor } = await prepareMockPage(page, {
    toolkitRoutes: { '/api/v1/trace/2251799813685249': 'success' },
  })
  await page.goto('/cluster')
  await page.waitForLoadState('networkidle')
  await page.getByPlaceholder('2251799813685249').fill('2251799813685249')
  await page.getByRole('button', { name: 'Show timeline' }).click()
  await expect(page.getByText('success', { exact: true })).toBeVisible()
  monitor.assertClean()
})

test('cluster trace partial follow result', async ({ page }) => {
  const { monitor } = await prepareMockPage(page, {
    toolkitRoutes: { '/api/v1/trace/2251799813685249': 'partial' },
  })
  await page.goto('/cluster')
  await page.waitForLoadState('networkidle')
  await page.getByPlaceholder('2251799813685249').fill('2251799813685249')
  await page.getByRole('checkbox', { name: 'Bounded follow (max 20 events / 30s)' }).click({ force: true })
  await page.getByRole('button', { name: 'Follow timeline' }).click()
  await expect(page.getByText('partial', { exact: true })).toBeVisible()
  monitor.assertClean()
})

test('cluster plan and drift workflows', async ({ page }) => {
  const { monitor } = await prepareMockPage(page, {
    toolkitRoutes: {
      '/api/v1/plan': 'success',
      '/api/v1/drift': 'findings',
    },
  })
  await page.goto('/cluster')
  await page.waitForLoadState('networkidle')
  await page.getByPlaceholder('/tmp/cam-demo').fill('/tmp/cam-demo')
  await page.getByRole('button', { name: 'Run plan' }).click()
  await expect(page.getByText('success', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Check drift' }).click()
  await expect(page.getByText('findings', { exact: true })).toBeVisible()
  monitor.assertClean()
})

test('cluster incident retry requires confirmation', async ({ page }) => {
  const { monitor, mutations } = await prepareMockPage(page)
  await page.goto('/cluster')
  await page.waitForLoadState('networkidle')
  await expectNoMutationBeforeConfirm(
    page,
    mutations,
    'Retry',
    'Retry incident',
    '/api/v1/incidents/incident-1/retry',
  )
  monitor.assertClean()
  expect(mutations.some((request) => request.method() === 'POST')).toBe(true)
})
