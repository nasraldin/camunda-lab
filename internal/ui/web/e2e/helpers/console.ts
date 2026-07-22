import { expect, type Page, type Response } from '@playwright/test'

export type ConsoleIssue = {
  type: 'error' | 'warning' | 'pageerror'
  text: string
}

export type NetworkIssue = {
  url: string
  status?: number
  failure?: string
}

export type TestMonitor = {
  consoleIssues: ConsoleIssue[]
  networkIssues: NetworkIssue[]
  assertClean: () => void
}

const ignoredConsolePatterns = [
  /^Download the React DevTools/,
  /^%cDownload the React DevTools/,
  /^\[vite\]/,
  /^Failed to load resource: the server responded with a status of 404/,
]

const ignoredNetworkPatterns = [/\/favicon\.ico$/, /\.map$/, /^data:/]

function shouldIgnoreConsole(text: string): boolean {
  return ignoredConsolePatterns.some((pattern) => pattern.test(text))
}

function shouldIgnoreNetwork(url: string): boolean {
  return ignoredNetworkPatterns.some((pattern) => pattern.test(url))
}

function formatConsoleIssues(issues: ConsoleIssue[]): string {
  return issues.map((issue) => `[${issue.type}] ${issue.text}`).join('\n')
}

function formatNetworkIssues(issues: NetworkIssue[]): string {
  return issues
    .map((issue) => {
      if (issue.failure) return `${issue.url} (${issue.failure})`
      return `${issue.url} (${issue.status ?? 'unknown'})`
    })
    .join('\n')
}

export function attachTestMonitor(page: Page): TestMonitor {
  const consoleIssues: ConsoleIssue[] = []
  const networkIssues: NetworkIssue[] = []

  page.on('console', (message) => {
    const type = message.type()
    if (type !== 'error' && type !== 'warning') return
    const text = message.text()
    if (shouldIgnoreConsole(text)) return
    consoleIssues.push({ type, text })
  })

  page.on('pageerror', (error) => {
    consoleIssues.push({ type: 'pageerror', text: error.message })
  })

  page.on('response', (response: Response) => {
    const url = response.url()
    if (shouldIgnoreNetwork(url)) return
    if (!url.includes('/api/v1/')) return
    const status = response.status()
    if (status >= 400) {
      networkIssues.push({ url, status })
    }
  })

  page.on('requestfailed', (request) => {
    const url = request.url()
    if (shouldIgnoreNetwork(url)) return
    networkIssues.push({ url, failure: request.failure()?.errorText ?? 'request failed' })
  })

  return {
    consoleIssues,
    networkIssues,
    assertClean() {
      expect(consoleIssues, formatConsoleIssues(consoleIssues)).toEqual([])
      expect(networkIssues, formatNetworkIssues(networkIssues)).toEqual([])
    },
  }
}
