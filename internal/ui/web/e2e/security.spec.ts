import { expect, test } from '@playwright/test'

import {
  assertSameOriginMutations,
  csrfToken,
  expectNoMutationBeforeConfirm,
  mockAPI,
} from './helpers/api'
import { prepareMockPage } from './helpers/page'

test('csrf transport covers JSON, multipart, DELETE, and one invalid-token refresh', async ({
  page,
}) => {
  let sessionCalls = 0
  const mutations: Awaited<ReturnType<typeof mockAPI>> = []

  await page.route('**/api/v1/**', async (route) => {
    const request = route.request()
    const path = new URL(request.url()).pathname
    if (path === '/api/v1/session') {
      sessionCalls += 1
      await route.fulfill({ json: { csrfToken: sessionCalls === 1 ? 'stale-token' : csrfToken } })
      return
    }
    if (!['GET', 'HEAD', 'OPTIONS'].includes(request.method())) {
      mutations.push(request)
      if (path === '/api/v1/project/init' && mutations.length === 1) {
        await route.fulfill({ status: 403, json: { error: 'csrf_invalid', code: 'csrf_invalid' } })
        return
      }
    }
    if (path === '/api/v1/overview') {
      await route.fulfill({
        json: {
          cliVersion: 'test',
          labHome: '/tmp/lab',
          configured: true,
          config: {
            version: '8.9',
            profile: 'light',
            resources: 'small',
            host: 'localhost',
            project: '',
            aiEnabled: false,
          },
          supportedVersions: ['8.9'],
          defaultVersion: '8.9',
        },
      })
      return
    }
    if (path === '/api/v1/env') {
      await route.fulfill({
        json: {
          ok: true,
          active: 'lab',
          profiles: [
            { name: 'lab', kind: 'local' },
            { name: 'staging', kind: 'remote' },
          ],
        },
      })
      return
    }
    await route.fulfill({ json: { ok: true, output: 'ok' } })
  })

  await page.goto('/project')
  await page.getByRole('button', { name: 'Scaffold project' }).click()
  await expect.poll(() => mutations.length).toBe(2)
  expect(sessionCalls).toBe(2)
  expect(mutations[0]?.headers()['x-camunda-lab-csrf']).toBe('stale-token')
  expect(mutations[1]?.headers()['x-camunda-lab-csrf']).toBe(csrfToken)

  await page.getByRole('button', { name: 'Remove' }).click()
  await page.getByRole('button', { name: 'Remove environment' }).click()
  await expect.poll(() => mutations.some((r) => r.method() === 'DELETE')).toBe(true)
  await expect(page.getByRole('dialog')).toBeHidden()
  expect(mutations.at(-1)?.headers()['x-camunda-lab-csrf']).toBe(csrfToken)

  await page.getByLabel('Restore archive').setInputFiles({
    name: 'backup.tar.gz',
    mimeType: 'application/gzip',
    buffer: Buffer.from('archive'),
  })
  await page.getByLabel('Type RESTORE to confirm').fill('RESTORE')
  await page.getByRole('button', { name: 'Restore backup' }).click()
  await expect.poll(() => mutations.some((r) => r.postDataBuffer()?.includes(Buffer.from('archive')))).toBe(
    true,
  )
  const multipart = mutations.at(-1)
  expect(multipart?.headers()['x-camunda-lab-csrf']).toBe(csrfToken)
  expect(multipart?.headers()['content-type']).toMatch(/^multipart\/form-data; boundary=/)
})

test('mutations stay same-origin for lab controls', async ({ page, baseURL }) => {
  const { monitor, mutations } = await prepareMockPage(page)
  await page.goto('/')
  await page.waitForLoadState('networkidle')
  await page.getByRole('button', { name: 'Stop lab' }).click()
  await page.getByRole('button', { name: 'Stop lab', exact: true }).last().click()
  await expect.poll(() => mutations.length).toBeGreaterThan(0)
  assertSameOriginMutations(mutations, baseURL ?? 'http://127.0.0.1:4173')
  monitor.assertClean()
})

test('blocks accidental cross-origin API mutations', async ({ page }) => {
  const crossOriginAttempts: string[] = []
  await page.route('https://evil.example/**', async (route) => {
    crossOriginAttempts.push(route.request().url())
    await route.fulfill({ status: 500, body: 'blocked' })
  })
  const { monitor, mutations } = await prepareMockPage(page)
  await page.goto('/project')
  await page.waitForLoadState('networkidle')
  await page.getByRole('button', { name: 'Scaffold project' }).click()
  await expect.poll(() => mutations.length).toBeGreaterThan(0)
  expect(crossOriginAttempts).toHaveLength(0)
  assertSameOriginMutations(mutations, 'http://127.0.0.1:4173')
  monitor.assertClean()
})

test('confirmation modal traps focus and restores invoker', async ({ page }) => {
  const { monitor } = await prepareMockPage(page)
  await page.goto('/')
  const invoker = page.getByRole('button', { name: 'Stop lab' })
  await invoker.focus()
  await invoker.click()
  const dialog = page.getByRole('dialog')
  await expect(dialog).toBeVisible()
  await page.keyboard.press('Shift+Tab')
  await expect(dialog.locator(':focus')).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(dialog).toBeHidden()
  await expect(invoker).toBeFocused()
  monitor.assertClean()
})

test('service restart requires confirmation', async ({ page }) => {
  const { monitor, mutations } = await prepareMockPage(page)
  await page.goto('/containers')
  await page.waitForLoadState('networkidle')
  await expectNoMutationBeforeConfirm(
    page,
    mutations,
    'Restart',
    'Restart service',
    '/api/v1/containers/zeebe/restart',
  )
  monitor.assertClean()
})

test('destructive lab reset requires typed DELETE twice', async ({ page }) => {
  const { monitor, mutations } = await prepareMockPage(page)
  await page.goto('/danger')
  await page.waitForLoadState('networkidle')
  await page.getByLabel('Type DELETE to confirm').fill('DELETE')
  await page.getByRole('button', { name: 'Delete everything' }).click()
  expect(mutations).toHaveLength(0)
  await expect(page.getByRole('dialog')).toBeVisible()
  await page.getByLabel('Type DELETE to confirm', { exact: true }).last().fill('DELETE')
  await page.getByRole('button', { name: 'Permanently delete lab' }).click()
  await expect.poll(() => mutations.map((r) => new URL(r.url()).pathname)).toContain('/api/v1/nuke')
  monitor.assertClean()
})
