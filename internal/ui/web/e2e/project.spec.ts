import { expect, test } from '@playwright/test'

import { expectNoMutationBeforeConfirm } from './helpers/api'
import { prepareMockPage } from './helpers/page'

test('project scaffold init success', async ({ page }) => {
  const { monitor } = await prepareMockPage(page, {
    toolkitRoutes: { '/api/v1/project/init': 'success' },
  })
  await page.goto('/project')
  await page.waitForLoadState('networkidle')
  await page.getByRole('button', { name: 'Scaffold project' }).click()
  await expect(page.getByText('success', { exact: true })).toBeVisible()
  monitor.assertClean()
})

test('project environments list and switch profile', async ({ page }) => {
  const { monitor } = await prepareMockPage(page, {
    envActive: 'lab',
    envProfiles: [
      { name: 'lab', kind: 'local' },
      { name: 'staging', kind: 'remote' },
    ],
    toolkitRoutes: { '/api/v1/env/use': 'success' },
  })
  await page.goto('/project')
  await page.waitForLoadState('networkidle')
  await expect(page.getByRole('button', { name: 'Remove' })).toBeVisible()
  await page.getByRole('button', { name: 'Use' }).nth(1).click()
  await expect(page.getByText('success', { exact: true })).toBeVisible()
  monitor.assertClean()
})

test('project add remote profile', async ({ page }) => {
  const { monitor } = await prepareMockPage(page, {
    toolkitRoutes: { '/api/v1/env': 'success' },
  })
  await page.goto('/project')
  await page.waitForLoadState('networkidle')
  await page.getByPlaceholder('staging').fill('qa')
  await page.getByPlaceholder('https://…/v2').fill('https://example.test/v2')
  await page.getByRole('button', { name: 'Add remote profile' }).click()
  await expect(page.getByText('success', { exact: true })).toBeVisible()
  monitor.assertClean()
})

test('project backup download button is available', async ({ page }) => {
  const { monitor } = await prepareMockPage(page)
  await page.goto('/project')
  await page.waitForLoadState('networkidle')
  await expect(page.getByRole('button', { name: 'Download backup (gzip)' })).toBeEnabled()
  monitor.assertClean()
})

test('project restore requires typed RESTORE confirmation', async ({ page }) => {
  const { monitor, mutations } = await prepareMockPage(page)
  await page.goto('/project')
  await page.waitForLoadState('networkidle')
  await page.getByLabel('Restore archive').setInputFiles({
    name: 'backup.tar.gz',
    mimeType: 'application/gzip',
    buffer: Buffer.from('archive'),
  })
  await expect(page.getByRole('dialog')).toBeVisible()
  expect(mutations).toHaveLength(0)
  await expect(page.getByRole('button', { name: 'Restore backup' })).toBeDisabled()
  await page.getByLabel('Type RESTORE to confirm').fill('RESTORE')
  await page.getByRole('button', { name: 'Restore backup' }).click()
  await expect.poll(() => mutations.map((r) => new URL(r.url()).pathname)).toContain('/api/v1/restore')
  monitor.assertClean()
})

test('project environment removal requires confirmation', async ({ page }) => {
  const { monitor, mutations } = await prepareMockPage(page)
  await page.goto('/project')
  await page.waitForLoadState('networkidle')
  await expectNoMutationBeforeConfirm(
    page,
    mutations,
    'Remove',
    'Remove environment',
    '/api/v1/env/staging',
  )
  monitor.assertClean()
})
