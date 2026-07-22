import { expect, test } from '@playwright/test'

import { auditPage, assertCardBorderVisible, prepareMockPage, primaryRoutes, setTheme, visitClean } from './helpers/page'

test.describe('navigation', () => {
  for (const route of primaryRoutes) {
    test(`loads ${route.path} with clean console and accessibility`, async ({ page }) => {
      await visitClean(page, route.path)
      await expect(page.getByRole('heading', { level: 1, name: route.heading })).toBeVisible()
      await auditPage(page)
    })
  }

  test('sidebar navigation reaches BPMN and Cluster workflows', async ({ page }) => {
    const { monitor } = await prepareMockPage(page)
    await page.goto('/')
    await page.getByRole('link', { name: 'BPMN' }).click()
    await expect(page).toHaveURL(/\/bpmn$/)
    await expect(page.getByRole('heading', { name: 'BPMN toolkit' })).toBeVisible()
    await page.getByRole('link', { name: 'Cluster' }).click()
    await expect(page).toHaveURL(/\/cluster$/)
    await expect(page.getByRole('heading', { name: 'Cluster' })).toBeVisible()
    monitor.assertClean()
    await auditPage(page)
  })

  test('hard refresh keeps the active route usable', async ({ page }) => {
    await visitClean(page, '/project')
    await page.reload({ waitUntil: 'networkidle' })
    await expect(page.getByRole('heading', { level: 1, name: 'Project' })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Scaffold project' })).toBeVisible()
    await auditPage(page)
  })

  test('light and dark themes keep card borders visible', async ({ page }) => {
    const { monitor } = await prepareMockPage(page)
    await page.goto('/bpmn')
    await page.waitForLoadState('networkidle')
    const theme = await page.locator('html').getAttribute('data-theme')
    if (theme === 'dark') {
      await page.getByRole('button', { name: 'Switch to light' }).click()
    }
    await auditPage(page)
    await page.getByRole('button', { name: 'Switch to dark' }).click()
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
    await assertCardBorderVisible(page)
    monitor.assertClean()
  })

  test('theme toggle updates data-theme via nav control', async ({ page }) => {
    const { monitor } = await prepareMockPage(page)
    await page.goto('/')
    await page.waitForLoadState('networkidle')
    await setTheme(page, 'light')
    await page.getByRole('button', { name: 'Switch to dark' }).click()
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
    await page.getByRole('button', { name: 'Switch to light' }).click()
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')
    monitor.assertClean()
  })
})
