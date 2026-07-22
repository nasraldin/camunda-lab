import fs from 'node:fs/promises'
import os from 'node:os'
import path from 'node:path'

import { expect, test } from '@playwright/test'

import { assertSafeGzip, assertSafeZip, mockGzipArchive, mockZipArchive } from './helpers/downloads'
import { prepareMockPage } from './helpers/page'

test('project backup download is nonempty safe gzip', async ({ page }) => {
  const archive = mockGzipArchive('project backup fixture')
  const { monitor } = await prepareMockPage(page, { backupArchive: archive })
  await page.goto('/project')
  await page.waitForLoadState('networkidle')

  const downloadPromise = page.waitForEvent('download')
  await page.getByRole('button', { name: 'Download backup (gzip)' }).click()
  const download = await downloadPromise
  expect(download.suggestedFilename()).toMatch(/\.tar\.gz$/)

  const tempDir = await fs.mkdtemp(path.join(os.tmpdir(), 'camunda-lab-backup-'))
  const filePath = path.join(tempDir, download.suggestedFilename())
  await download.saveAs(filePath)
  const buffer = await fs.readFile(filePath)
  assertSafeGzip(buffer)
  monitor.assertClean()
})

test('BPMN test gen ZIP download is nonempty with safe entries', async ({ page }) => {
  const archive = mockZipArchive()
  const { monitor } = await prepareMockPage(page, { testsZip: archive })
  await page.goto('/bpmn')
  await page.waitForLoadState('networkidle')
  await page.getByRole('tab', { name: 'Test gen' }).click()
  await page.getByPlaceholder('/Users/…/process.bpmn').fill('/tmp/process.bpmn')

  const downloadPromise = page.waitForEvent('download')
  await page.getByRole('button', { name: 'Download tests (ZIP)' }).click()
  const download = await downloadPromise
  expect(download.suggestedFilename()).toMatch(/\.zip$/)

  const tempDir = await fs.mkdtemp(path.join(os.tmpdir(), 'camunda-lab-tests-'))
  const filePath = path.join(tempDir, download.suggestedFilename())
  await download.saveAs(filePath)
  const buffer = await fs.readFile(filePath)
  assertSafeZip(buffer)
  monitor.assertClean()
})

test('ActionResult text download produces nonempty file', async ({ page }) => {
  const { monitor } = await prepareMockPage(page, {
    toolkitRoutes: { '/api/v1/bpmn/lint': 'success' },
  })
  await page.goto('/bpmn')
  await page.waitForLoadState('networkidle')
  await page.getByPlaceholder('/Users/…/process.bpmn').fill('/tmp/process.bpmn')
  await page.getByRole('button', { name: 'Run' }).click()
  await expect(page.getByRole('button', { name: 'Download text' })).toBeEnabled()

  const downloadPromise = page.waitForEvent('download')
  await page.getByRole('button', { name: 'Download text' }).click()
  const download = await downloadPromise
  const tempDir = await fs.mkdtemp(path.join(os.tmpdir(), 'camunda-lab-text-'))
  const filePath = path.join(tempDir, download.suggestedFilename())
  await download.saveAs(filePath)
  const stat = await fs.stat(filePath)
  expect(stat.size).toBeGreaterThan(0)
  monitor.assertClean()
})
