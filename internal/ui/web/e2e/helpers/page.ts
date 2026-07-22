import { expect, type Page } from '@playwright/test'

import { checkAccessibility } from './axe'
import { mockAPI, type ApiMockOptions } from './api'
import { attachTestMonitor, type TestMonitor } from './console'

export type PreparedPage = {
  monitor: TestMonitor
  mutations: Awaited<ReturnType<typeof mockAPI>>
}

export async function prepareMockPage(page: Page, options: ApiMockOptions = {}): Promise<PreparedPage> {
  const monitor = attachTestMonitor(page)
  const mutations = await mockAPI(page, options)
  return { monitor, mutations }
}

export async function visitClean(
  page: Page,
  path: string,
  options: ApiMockOptions = {},
): Promise<PreparedPage> {
  const prepared = await prepareMockPage(page, options)
  await page.goto(path)
  await page.waitForLoadState('networkidle')
  prepared.monitor.assertClean()
  return prepared
}

export async function setTheme(page: Page, theme: 'light' | 'dark'): Promise<void> {
  await page.evaluate((mode) => {
    document.documentElement.setAttribute('data-theme', mode)
    localStorage.setItem('camunda-lab-theme', mode)
  }, theme)
}

export async function assertCardBorderVisible(page: Page): Promise<void> {
  const borderWidth = await page.locator('.card').first().evaluate((element) => {
    const style = window.getComputedStyle(element)
    return style.borderTopWidth
  })
  expect(borderWidth).not.toBe('0px')
}

export async function auditPage(page: Page): Promise<void> {
  await checkAccessibility(page)
  await assertCardBorderVisible(page)
}

export const primaryRoutes = [
  { path: '/', heading: 'Home' },
  { path: '/setup', heading: 'Get started' },
  { path: '/apps', heading: 'Apps' },
  { path: '/bpmn', heading: 'BPMN toolkit' },
  { path: '/cluster', heading: 'Cluster' },
  { path: '/project', heading: 'Project' },
  { path: '/admin', heading: 'Logins' },
  { path: '/containers', heading: 'Services' },
  { path: '/logs', heading: 'Logs' },
  { path: '/ai', heading: 'AI helpers' },
  { path: '/tools', heading: 'Extras' },
  { path: '/danger', heading: 'Reset lab' },
] as const
