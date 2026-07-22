import AxeBuilder from '@axe-core/playwright'
import { expect, type Page } from '@playwright/test'

function formatViolations(
  violations: Awaited<ReturnType<AxeBuilder['analyze']>>['violations'],
): string {
  return violations
    .map((violation) => `${violation.id}: ${violation.description}\n${violation.helpUrl}`)
    .join('\n\n')
}

export async function checkAccessibility(page: Page): Promise<void> {
  const results = await new AxeBuilder({ page })
    .include('main')
    .disableRules(['heading-order'])
    .analyze()
  expect(results.violations, formatViolations(results.violations)).toEqual([])
}
