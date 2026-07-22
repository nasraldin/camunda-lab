import { expect, type Page, type Request } from '@playwright/test'

import { mockGzipArchive, mockZipArchive } from './downloads'
import { mockToolkitResponse, type MockToolkitState } from './fixtures'

export const csrfToken = 'browser-test-csrf-token'

export type ApiMockOptions = {
  csrf?: string
  toolkitState?: MockToolkitState
  toolkitPath?: string
  toolkitRoutes?: Record<string, MockToolkitState>
  backupArchive?: Buffer
  testsZip?: Buffer
  incidents?: unknown[]
  envProfiles?: Array<{ name: string; kind: string }>
  envActive?: string
}

const mutationMethods = new Set(['POST', 'PUT', 'PATCH', 'DELETE'])

function isMutation(method: string): boolean {
  return mutationMethods.has(method)
}

async function fulfillOverview(route: Parameters<Parameters<Page['route']>[1]>[0]): Promise<void> {
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
      defaultVersion: '8.9',
      containers: [],
      running: 0,
      total: 0,
    },
  })
}

async function fulfillDefault(
  route: Parameters<Parameters<Page['route']>[1]>[0],
  path: string,
  options: ApiMockOptions,
): Promise<boolean> {
  const request = route.request()

  if (path === '/api/v1/session') {
    await route.fulfill({ json: { csrfToken: options.csrf ?? csrfToken } })
    return true
  }
  if (path === '/api/v1/overview') {
    await fulfillOverview(route)
    return true
  }
  if (path === '/api/v1/update') {
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
    return true
  }
  if (path === '/api/v1/env') {
    await route.fulfill({
      json: {
        ok: true,
        active: options.envActive ?? 'lab',
        profiles: options.envProfiles ?? [
          { name: 'lab', kind: 'local' },
          { name: 'staging', kind: 'remote' },
        ],
      },
    })
    return true
  }
  if (path === '/api/v1/incidents') {
    await route.fulfill({
      json: {
        ok: true,
        items: options.incidents ?? [
          {
            key: 'incident-1',
            id: 'incident-1',
            errorMessage: 'job failed',
            error: 'job failed',
            processDefinitionId: 'order',
            process: 'order',
          },
        ],
      },
    })
    return true
  }
  if (path === '/api/v1/containers') {
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
    return true
  }
  if (path === '/api/v1/urls') {
    await route.fulfill({
      json: {
        urls: [
          { name: 'Operate', url: 'http://127.0.0.1:8081', notes: 'mock' },
          { name: 'Tasklist', url: 'http://127.0.0.1:8082', notes: 'mock' },
        ],
      },
    })
    return true
  }
  if (path === '/api/v1/doctor' || path === '/api/v1/doctor/deep') {
    await route.fulfill({ json: { ok: true, report: 'All checks passed.' } })
    return true
  }
  if (path === '/api/v1/smoke') {
    await route.fulfill({
      json: {
        OK: true,
        Checks: [{ Name: 'Operate', URL: 'http://127.0.0.1:8081', OK: true, Detail: 'mock' }],
      },
    })
    return true
  }
  if (path.startsWith('/api/v1/probe')) {
    await route.fulfill({
      json: {
        name: 'Operate',
        ok: true,
        kind: 'http',
        checkedURL: 'http://127.0.0.1:8081',
        detail: 'mock probe',
      },
    })
    return true
  }
  if (path === '/api/v1/ai/status') {
    await route.fulfill({
      json: {
        enabled: false,
        openaiKey: '',
        anthropicKey: '',
        openaiBaseUrl: '',
        supported: true,
        supportError: '',
      },
    })
    return true
  }
  if (path === '/api/v1/ai/config') {
    await route.fulfill({ json: { config: '{}' } })
    return true
  }
  if (path === '/api/v1/tools/c8ctl/status') {
    await route.fulfill({ json: { installed: false, path: '' } })
    return true
  }

  if (request.method() === 'GET' && path.startsWith('/api/v1/toolkit/')) {
    return false
  }

  await route.fulfill({ json: { ok: true, output: 'ok' } })
  return true
}

function toolkitResponseForPath(
  path: string,
  options: ApiMockOptions,
): ReturnType<typeof mockToolkitResponse> | null {
  if (options.toolkitRoutes?.[path]) {
    return mockToolkitResponse(options.toolkitRoutes[path])
  }
  if (options.toolkitPath === path && options.toolkitState) {
    return mockToolkitResponse(options.toolkitState)
  }
  return null
}

export async function mockAPI(page: Page, options: ApiMockOptions = {}): Promise<Request[]> {
  const mutations: Request[] = []
  const token = options.csrf ?? csrfToken

  await page.route('**/api/v1/**', async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname
    const method = request.method()

    if (isMutation(method)) mutations.push(request)

    if (path === '/api/v1/session') {
      await route.fulfill({ json: { csrfToken: token } })
      return
    }

    if (path === '/api/v1/backup/download' && method === 'POST') {
      const body = options.backupArchive ?? mockGzipArchive()
      await route.fulfill({
        status: 200,
        headers: {
          'Content-Type': 'application/gzip',
          'Content-Disposition': 'attachment; filename="lab-backup.tar.gz"',
        },
        body,
      })
      return
    }

    if (path === '/api/v1/bpmn/test-generate/download' && method === 'POST') {
      const body = options.testsZip ?? mockZipArchive()
      await route.fulfill({
        status: 200,
        headers: {
          'Content-Type': 'application/zip',
          'Content-Disposition': 'attachment; filename="camunda-lab-tests.zip"',
        },
        body,
      })
      return
    }

    const toolkitResponse = toolkitResponseForPath(path, options)
    if (toolkitResponse) {
      await route.fulfill({ json: toolkitResponse })
      return
    }

    await fulfillDefault(route, path, options)
  })

  return mutations
}

export async function expectNoMutationBeforeConfirm(
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
  await expect.poll(() => mutations.map((request) => new URL(request.url()).pathname)).toContain(
    expectedPath,
  )
}

export function assertSameOriginMutations(mutations: Request[], baseURL: string): void {
  const origin = new URL(baseURL).origin
  for (const request of mutations) {
    expect(new URL(request.url()).origin).toBe(origin)
  }
}
