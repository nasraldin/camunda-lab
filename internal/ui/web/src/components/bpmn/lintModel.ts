export type LintSeverity = 'error' | 'warning'

export type LintFindingRow = {
  rule: string
  severity: LintSeverity
  message: string
  element?: string
  processId?: string
  file?: string
}

export function normalizeLintFindings(raw: unknown[] | undefined): LintFindingRow[] {
  if (!raw?.length) return []
  return raw.map((item) => {
    const row = item as Record<string, unknown>
    const nested = row.finding as Record<string, unknown> | undefined
    const finding = nested ?? row
    const severity = finding.severity === 'warning' ? 'warning' : 'error'
    return {
      rule: String(finding.rule ?? ''),
      severity,
      message: String(finding.message ?? ''),
      element: finding.element ? String(finding.element) : undefined,
      processId: row.processId
        ? String(row.processId)
        : finding.processId
          ? String(finding.processId)
          : undefined,
      file: finding.file ? String(finding.file) : undefined,
    }
  })
}

export function lintRuleLabel(rule: string): string {
  return rule.replaceAll('.', ' · ').replaceAll('-', ' ')
}

export function groupFindingsByFile(findings: LintFindingRow[]): Map<string, LintFindingRow[]> {
  const groups = new Map<string, LintFindingRow[]>()
  for (const finding of findings) {
    const key = finding.file || 'Uploaded BPMN'
    const bucket = groups.get(key) ?? []
    bucket.push(finding)
    groups.set(key, bucket)
  }
  return groups
}

export function findingsForFile(findings: LintFindingRow[], fileKey: string): LintFindingRow[] {
  const base = fileKey.split(/[/\\]/).pop() ?? fileKey
  return findings.filter((finding) => {
    const file = finding.file || 'Uploaded BPMN'
    if (file === fileKey || file === base) return true
    const findingBase = file.split(/[/\\]/).pop() ?? file
    return findingBase === base
  })
}
