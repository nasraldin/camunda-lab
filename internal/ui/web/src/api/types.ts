export type ToolkitStatus = 'completed' | 'partial' | 'failed' | 'skipped'

export type AIStatus = 'disabled' | 'skipped' | 'succeeded' | 'failed'

export type ResultPhase =
  | 'idle'
  | 'loading'
  | 'empty'
  | 'success'
  | 'findings'
  | 'partial'
  | 'unsupported'
  | 'failure'

export type ToolkitWarning = {
  code: string
  message: string
  path?: string
}

export type ToolkitArtifact = {
  path: string
  mediaType: string
  content?: string
}

/** Shared envelope for platform and developer toolkit JSON responses. */
export type ToolkitEnvelope = {
  ok: boolean
  status?: ToolkitStatus
  complete?: boolean
  output?: string
  error?: string
  hint?: string
  cli?: string
  code?: string
  findings?: unknown[]
  changes?: unknown[]
  markdown?: string
  paths?: string[]
  contents?: Record<string, string>
  mode?: 'download' | 'written'
  artifacts?: ToolkitArtifact[]
  warnings?: ToolkitWarning[]
  stats?: { discovered: number; scanned: number; ignored: number; errored: number }
  items?: unknown[]
  issues?: unknown[]
  timeline?: unknown
  plan?: unknown
  drift?: unknown
  report?: string
  sections?: unknown[]
  active?: string
  profiles?: Array<{ name: string; kind: string }>
  path?: string
  dir?: string
  inputs?: string[]
  aiStatus?: AIStatus
}

export type LintThreshold = 'error' | 'warning'
export type ScanThreshold = 'low' | 'medium' | 'high'
export type GenerateLanguage = 'java' | 'js' | 'python'

export type LintResult = ToolkitEnvelope & {
  status: ToolkitStatus
  complete: boolean
  warnings: ToolkitWarning[]
  findings: unknown[]
  inputs?: string[]
}

export type DiffResult = ToolkitEnvelope & {
  status: ToolkitStatus
  complete: boolean
  warnings: ToolkitWarning[]
  changes: unknown[]
}

export type ExplainResult = ToolkitEnvelope & {
  status: ToolkitStatus
  complete: boolean
  warnings: ToolkitWarning[]
  processes?: Array<{ processId: string; markdown: string }>
}

export type ReviewResult = ToolkitEnvelope & {
  status: ToolkitStatus
  complete: boolean
  warnings: ToolkitWarning[]
  findings: unknown[]
  aiStatus?: AIStatus
  reviews?: unknown[]
}

export type GenerateResult = ToolkitEnvelope & {
  status: ToolkitStatus
  complete: boolean
  warnings: ToolkitWarning[]
  mode: 'download' | 'written'
  artifacts: ToolkitArtifact[]
  paths?: string[]
  contents?: Record<string, string>
}

export type ScanResult = ToolkitEnvelope & {
  status: ToolkitStatus
  complete: boolean
  warnings: ToolkitWarning[]
  scannedRoots?: string[]
  failedRoots?: string[]
  findings: unknown[]
  issues?: unknown[]
  stats?: { discovered: number; scanned: number; ignored: number; errored: number }
}

export type IncidentItem = {
  key?: string
  id?: string
  ID?: string
  errorMessage?: string
  error?: string
  Error?: string
  processDefinitionId?: string
  process?: string
  Process?: string
  elementId?: string
  jobWorker?: string
  JobWorker?: string
  state?: string
}

export type IncidentsResult = ToolkitEnvelope & {
  items: IncidentItem[]
}

export type TraceOptions = {
  follow?: boolean
  interval?: string
  timeout?: string
  maxEvents?: number
  dir?: string
  environment?: string
}

export type BinaryDownload = {
  blob: Blob
  filename: string
  count?: number
}

export function hasFindings(result: ToolkitEnvelope | null | undefined): boolean {
  if (!result) return false
  if (Array.isArray(result.findings) && result.findings.length > 0) return true
  if (Array.isArray(result.changes) && result.changes.length > 0) return true
  if (Array.isArray(result.issues) && result.issues.length > 0) return true
  return false
}

export function resultText(result: ToolkitEnvelope | null | undefined): string {
  if (!result) return ''
  if (result.output) return result.output
  if (result.markdown) return result.markdown
  if (result.report) return result.report
  return JSON.stringify(result, null, 2)
}

export function isUnsupportedResult(code?: string | null, error?: string | null): boolean {
  if (code === 'unsupported' || code === 'capability_unsupported') return true
  const lower = (code || error || '').toLowerCase()
  return lower.includes('unsupported') || lower.includes('not supported')
}

export function deriveResultPhase(opts: {
  loading?: boolean
  error?: string | null
  code?: string | null
  result?: ToolkitEnvelope | null
  empty?: boolean
}): ResultPhase {
  if (opts.loading) return 'loading'
  const envelopeCode = opts.code ?? opts.result?.code ?? null
  const envelopeError = opts.error ?? opts.result?.error ?? null
  if (opts.error) {
    if (isUnsupportedResult(envelopeCode, envelopeError)) return 'unsupported'
    return 'failure'
  }
  if (opts.empty) return 'empty'
  if (!opts.result) return 'idle'
  if (opts.result.status === 'partial' || opts.result.complete === false) return 'partial'
  if (!opts.result.ok && hasFindings(opts.result)) return 'findings'
  if (opts.result.ok) return 'success'
  if (!opts.result.ok && isUnsupportedResult(opts.result.code, opts.result.error)) {
    return 'unsupported'
  }
  if (opts.result.status === 'failed') return 'failure'
  if (hasFindings(opts.result)) return 'findings'
  return 'failure'
}
