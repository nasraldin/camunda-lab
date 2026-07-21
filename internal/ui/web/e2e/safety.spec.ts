import { expect, test, type Page, type Request } from '@playwright/test'

const csrfToken = 'browser-test-csrf-token'

async function mockAPI(page: Page): Promise<Request[]> {
  const mutations: Request[] = []

  await page.route('**/api/v1/**', async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    const method = request.method()

    if (!['GET', 'HEAD', 'OPTIONS'].includes(method)) mutations.push(request)

    if (url.pathname === '/api/v1/session') {
      await route.fulfill({ json: { csrfToken } })
      return
    }
    if (url.pathname === '/api/v1/overview') {
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
          supportedVersions: ['8.9', '8.10'],
          containers: [],
          running: 0,
          total: 0,
        },
      })
      return
    }
    if (url.pathname === '/api/v1/update') {
      await route.fulfill({
        json: {
          ok: true,
          current: 'test',
          latest: 'test',
          updateAvailable: false,
          channel: 'dev',
          executable: '',
          releaseURL: '',
        },
      })
      return
    }
    if (url.pathname === '/api/v1/env') {
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
    if (url.pathname === '/api/v1/incidents') {
      await route.fulfill({
        json: { ok: true, items: [{ id: 'incident-1', error: 'job failed', process: 'order' }] },
      })
      return
    }
    if (url.pathname === '/api/v1/containers') {
      await route.fulfill({
        json: {
          containers: [
            {
              name: 'zeebe-1',
              service: 'zeebe',
              image: 'camunda/zeebe:8.9',
              state: 'running',
              health: 'healthy',
              status: 'running',
            },
          ],
        },
      })
      return
    }

    await route.fulfill({ json: { ok: true, output: 'ok' } })
  })

  return mutations
}

async function expectNoMutationBeforeConfirm(
  page: Page,
  mutations: Request[],
  openButton: string | RegExp,
  confirmButton: string | RegExp,
  expectedPath: string,
): Promise<void> {
  await page.getByRole('button', { name: openButton }).click()
  await expect(page.getByRole('dialog')).toBeVisible()
  expect(mutations).toHaveLength(0)
  await page.getByRole('button', { name: confirmButton }).click()
  await expect.poll(() => mutations.map((r) => new URL(r.url()).pathname)).toContain(expectedPath)
}

test('csrf transport covers JSON, multipart, DELETE, and one invalid-token refresh', async ({
  page,
}) => {
  let sessionCalls = 0
  const mutations: Request[] = []

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

test('confirmation gates incident resolution', async ({ page }) => {
  const mutations = await mockAPI(page)
  await page.goto('/cluster')
  await expectNoMutationBeforeConfirm(
    page,
    mutations,
    'Retry',
    'Retry incident',
    '/api/v1/incidents/incident-1/retry',
  )
})

test('confirmation gates environment removal', async ({ page }) => {
  const mutations = await mockAPI(page)
  await page.goto('/project')
  await expectNoMutationBeforeConfirm(
    page,
    mutations,
    'Remove',
    'Remove environment',
    '/api/v1/env/staging',
  )
})

test('restore selection never mutates and requires typed RESTORE', async ({ page }) => {
  const mutations = await mockAPI(page)
  await page.goto('/project')
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
})

test('confirmation gates Kubernetes restart and scale', async ({ page }) => {
  const mutations = await mockAPI(page)
  await page.goto('/project')
  await page.getByRole('button', { name: 'Restart', exact: true }).click()
  expect(mutations).toHaveLength(0)
  await page.getByRole('button', { name: 'Restart component' }).click()
  await expect.poll(() => mutations.map((r) => new URL(r.url()).pathname)).toContain(
    '/api/v1/k8s/restart',
  )
  const count = mutations.length
  await page.getByRole('button', { name: 'Scale', exact: true }).click()
  expect(mutations).toHaveLength(count)
  await page.getByRole('button', { name: 'Scale component' }).click()
  await expect.poll(() => mutations.map((r) => new URL(r.url()).pathname)).toContain(
    '/api/v1/k8s/scale',
  )
})

test('confirmation gates service restart', async ({ page }) => {
  const mutations = await mockAPI(page)
  await page.goto('/containers')
  await expectNoMutationBeforeConfirm(
    page,
    mutations,
    'Restart',
    'Restart service',
    '/api/v1/containers/zeebe/restart',
  )
})

test('confirmation gates lab stop and restart', async ({ page }) => {
  const mutations = await mockAPI(page)
  await page.goto('/')
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
})

test('confirmation gates destructive version switch', async ({ page }) => {
  const mutations = await mockAPI(page)
  await page.goto('/setup')
  await expectNoMutationBeforeConfirm(
    page,
    mutations,
    /Move to 8\.10/,
    'Clear data and switch',
    '/api/v1/switch',
  )
})

test('reset keeps typed DELETE and explicit modal confirmation', async ({ page }) => {
  const mutations = await mockAPI(page)
  await page.goto('/danger')
  await page.getByLabel('Type DELETE to confirm').fill('DELETE')
  await page.getByRole('button', { name: 'Delete everything' }).click()
  expect(mutations).toHaveLength(0)
  await expect(page.getByRole('dialog')).toBeVisible()
  await page.getByLabel('Type DELETE to confirm', { exact: true }).last().fill('DELETE')
  await page.getByRole('button', { name: 'Permanently delete lab' }).click()
  await expect.poll(() => mutations.map((r) => new URL(r.url()).pathname)).toContain('/api/v1/nuke')
})

test('confirmation modal traps focus, cancels with Escape, and restores invoking focus', async ({
  page,
}) => {
  await mockAPI(page)
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
})
