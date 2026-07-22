import { expect, test } from '@playwright/test'

import { prepareMockPage } from './helpers/page'
import type { MockToolkitState } from './helpers/fixtures'

const bpmnTabs = [
  { tab: 'Lint', path: '/api/v1/bpmn/lint', input: 'path' },
  { tab: 'Diff', path: '/api/v1/bpmn/diff', input: 'diff' },
  { tab: 'Explain', path: '/api/v1/bpmn/explain', input: 'path' },
  { tab: 'Review', path: '/api/v1/bpmn/review', input: 'path' },
  { tab: 'Test gen', path: '/api/v1/bpmn/test-generate', input: 'path' },
  { tab: 'Scan', path: '/api/v1/bpmn/scan', input: 'scan' },
] as const

const resultStates: Array<{ state: MockToolkitState; pill: string }> = [
  { state: 'success', pill: 'success' },
  { state: 'findings', pill: 'findings' },
  { state: 'partial', pill: 'partial' },
  { state: 'unsupported', pill: 'unsupported' },
  { state: 'failure', pill: 'failure' },
]

async function runBpmnTab(
  page: import('@playwright/test').Page,
  tab: (typeof bpmnTabs)[number]['tab'],
  input: (typeof bpmnTabs)[number]['input'],
): Promise<void> {
  await page.getByRole('tab', { name: tab }).click()
  if (input === 'scan') {
    await page.getByPlaceholder('/tmp/cam-demo').fill('/tmp/cam-demo')
  } else if (input === 'diff') {
    await page.getByPlaceholder('/Users/…/order-v1.bpmn').fill('/tmp/a.bpmn')
    await page.getByPlaceholder('/Users/…/order-v2.bpmn').fill('/tmp/b.bpmn')
  } else {
    await page.getByPlaceholder('/Users/…/process.bpmn').fill('/tmp/process.bpmn')
  }
  await page.getByRole('button', { name: 'Run' }).click()
}

for (const workflow of bpmnTabs) {
  test(`BPMN ${workflow.tab} tab runs with success output`, async ({ page }) => {
    const { monitor } = await prepareMockPage(page, {
      toolkitRoutes: { [workflow.path]: 'success' },
    })
    await page.goto('/bpmn')
    await page.waitForLoadState('networkidle')
    await runBpmnTab(page, workflow.tab, workflow.input)
    await expect(page.getByText('success', { exact: true })).toBeVisible()
    const visualTabs = new Set(['Lint', 'Diff', 'Explain', 'Review'])
    if (visualTabs.has(workflow.tab)) {
      await expect(page.locator('.bpmn-diagram-canvas, .banner.ok').first()).toBeVisible()
    } else {
      await expect(page.getByText('OK')).toBeVisible()
    }
    monitor.assertClean()
  })
}

for (const { state, pill } of resultStates) {
  test(`BPMN lint renders explicit ${pill} result state`, async ({ page }) => {
    const { monitor } = await prepareMockPage(page, {
      toolkitRoutes: { '/api/v1/bpmn/lint': state },
    })
    await page.goto('/bpmn')
    await page.waitForLoadState('networkidle')
    await runBpmnTab(page, 'Lint', 'path')
    if (state === 'unsupported' || state === 'failure') {
      await expect(page.locator('.banner.warn, .banner.error')).toBeVisible()
    } else {
      await expect(page.getByText(pill, { exact: true })).toBeVisible()
    }
    monitor.assertClean()
  })
}

test('BPMN idle state shows guidance before run', async ({ page }) => {
  const { monitor } = await prepareMockPage(page)
  await page.goto('/bpmn')
  await page.waitForLoadState('networkidle')
  await expect(
    page.getByText('Choose inputs and run the selected developer tool.'),
  ).toBeVisible()
  monitor.assertClean()
})

test('BPMN test gen exposes ZIP download control', async ({ page }) => {
  const { monitor } = await prepareMockPage(page, {
    toolkitRoutes: { '/api/v1/bpmn/test-generate': 'success' },
  })
  await page.goto('/bpmn')
  await page.waitForLoadState('networkidle')
  await page.getByRole('tab', { name: 'Test gen' }).click()
  await page.getByPlaceholder('/Users/…/process.bpmn').fill('/tmp/process.bpmn')
  await expect(page.getByRole('button', { name: 'Download tests (ZIP)' })).toBeEnabled()
  monitor.assertClean()
})
