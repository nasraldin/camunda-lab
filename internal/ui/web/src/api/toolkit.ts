import {
  downloadBinary,
  getJSON,
  mutationFetch,
  postForm,
  toolkitJSON,
  triggerBrowserDownload,
} from './client'
import { ApiError } from '../apiError'
import type {
  BinaryDownload,
  DiffResult,
  ExplainResult,
  GenerateLanguage,
  GenerateResult,
  IncidentsResult,
  LintResult,
  LintThreshold,
  ReviewResult,
  ScanResult,
  ScanThreshold,
  ToolkitEnvelope,
  TraceOptions,
} from './types'

export async function lintBpmn(
  body: {
    path?: string
    paths?: string[]
    projectDir?: string
    failOn?: LintThreshold
    ignore?: string[]
  },
  signal?: AbortSignal,
): Promise<LintResult> {
  return toolkitJSON<LintResult>('/api/v1/bpmn/lint', body, 'POST', signal)
}

export async function lintBpmnUpload(form: FormData, signal?: AbortSignal): Promise<LintResult> {
  return postForm<LintResult>('/api/v1/bpmn/lint', form, signal)
}

export async function diffBpmn(
  body: Record<string, unknown>,
  signal?: AbortSignal,
): Promise<DiffResult> {
  return toolkitJSON<DiffResult>('/api/v1/bpmn/diff', body, 'POST', signal)
}

export async function diffBpmnUpload(form: FormData, signal?: AbortSignal): Promise<DiffResult> {
  return postForm<DiffResult>('/api/v1/bpmn/diff', form, signal)
}

export async function explainBpmn(
  body: { path?: string },
  signal?: AbortSignal,
): Promise<ExplainResult> {
  return toolkitJSON<ExplainResult>('/api/v1/bpmn/explain', body, 'POST', signal)
}

export async function explainBpmnUpload(
  form: FormData,
  signal?: AbortSignal,
): Promise<ExplainResult> {
  return postForm<ExplainResult>('/api/v1/bpmn/explain', form, signal)
}

export async function reviewBpmn(
  body: Record<string, unknown>,
  signal?: AbortSignal,
): Promise<ReviewResult> {
  return toolkitJSON<ReviewResult>('/api/v1/bpmn/review', body, 'POST', signal)
}

export async function reviewBpmnUpload(
  form: FormData,
  signal?: AbortSignal,
): Promise<ReviewResult> {
  return postForm<ReviewResult>('/api/v1/bpmn/review', form, signal)
}

export async function generateTests(
  body: {
    path?: string
    lang?: GenerateLanguage
    write?: boolean
    output?: string
    force?: boolean
  },
  signal?: AbortSignal,
): Promise<GenerateResult> {
  return toolkitJSON<GenerateResult>('/api/v1/bpmn/test-generate', body, 'POST', signal)
}

export async function generateTestsUpload(
  form: FormData,
  signal?: AbortSignal,
): Promise<GenerateResult> {
  return postForm<GenerateResult>('/api/v1/bpmn/test-generate', form, signal)
}

export async function downloadGeneratedTests(
  body: { path?: string; lang?: GenerateLanguage; projectDir?: string },
): Promise<BinaryDownload> {
  return downloadBinary(
    '/api/v1/bpmn/test-generate/download',
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    },
    'camunda-lab-tests.zip',
  )
}

export async function downloadGeneratedTestsUpload(form: FormData): Promise<BinaryDownload> {
  return downloadBinary(
    '/api/v1/bpmn/test-generate/download',
    { method: 'POST', body: form },
    'camunda-lab-tests.zip',
  )
}

export async function scanProject(body: {
  dir: string
  failOn?: ScanThreshold
  ignore?: string[]
}): Promise<ScanResult> {
  return toolkitJSON<ScanResult>('/api/v1/bpmn/scan', body)
}

export async function getIncidents(opts?: {
  dir?: string
  environment?: string
  limit?: number
}): Promise<IncidentsResult> {
  const params = new URLSearchParams()
  if (opts?.dir) params.set('dir', opts.dir)
  if (opts?.environment) params.set('environment', opts.environment)
  if (opts?.limit != null) params.set('limit', String(opts.limit))
  const qs = params.toString()
  return getJSON<IncidentsResult>(`/api/v1/incidents${qs ? `?${qs}` : ''}`)
}

export async function retryIncident(
  id: string,
  body: { confirm: boolean; dir?: string },
): Promise<ToolkitEnvelope> {
  return toolkitJSON<ToolkitEnvelope>(`/api/v1/incidents/${encodeURIComponent(id)}/retry`, body)
}

export async function getTrace(instanceKey: string, opts?: TraceOptions): Promise<ToolkitEnvelope> {
  const params = new URLSearchParams()
  if (opts?.follow) params.set('follow', '1')
  if (opts?.interval) params.set('interval', opts.interval)
  if (opts?.timeout) params.set('timeout', opts.timeout)
  if (opts?.maxEvents != null) params.set('maxEvents', String(opts.maxEvents))
  if (opts?.dir) params.set('dir', opts.dir)
  if (opts?.environment) params.set('environment', opts.environment)
  const qs = params.toString()
  return getJSON<ToolkitEnvelope>(
    `/api/v1/trace/${encodeURIComponent(instanceKey)}${qs ? `?${qs}` : ''}`,
  )
}

export async function runPlan(body: { dir: string }): Promise<ToolkitEnvelope> {
  return toolkitJSON<ToolkitEnvelope>('/api/v1/plan', body)
}

export async function runDrift(body: { dir: string }): Promise<ToolkitEnvelope> {
  return toolkitJSON<ToolkitEnvelope>('/api/v1/drift', body)
}

export async function getEnv(opts?: { dir?: string }): Promise<ToolkitEnvelope> {
  const params = new URLSearchParams()
  if (opts?.dir) params.set('dir', opts.dir)
  const qs = params.toString()
  return getJSON<ToolkitEnvelope>(`/api/v1/env${qs ? `?${qs}` : ''}`)
}

export async function useEnv(body: { name: string; dir?: string }): Promise<ToolkitEnvelope> {
  return toolkitJSON<ToolkitEnvelope>('/api/v1/env/use', body)
}

export async function addEnv(body: {
  name: string
  kind: string
  orchestration?: string
  dir?: string
}): Promise<ToolkitEnvelope> {
  return toolkitJSON<ToolkitEnvelope>('/api/v1/env', body)
}

export async function removeEnv(name: string, dir?: string): Promise<ToolkitEnvelope> {
  const qs = dir ? `?dir=${encodeURIComponent(dir)}` : ''
  return toolkitJSON<ToolkitEnvelope>(
    `/api/v1/env/${encodeURIComponent(name)}${qs}`,
    undefined,
    'DELETE',
  )
}

export async function initProject(body: {
  dir: string
  name: string
  profile: string
  resources: string
  force?: boolean
  version?: string
}): Promise<ToolkitEnvelope> {
  return toolkitJSON<ToolkitEnvelope>('/api/v1/project/init', body)
}

export async function restoreBackup(form: FormData): Promise<ToolkitEnvelope> {
  return postForm<ToolkitEnvelope>('/api/v1/restore', form)
}

export async function downloadBackupArchive(opts?: {
  dir?: string
  includeSecrets?: boolean
}): Promise<BinaryDownload & { files: number }> {
  const res = await mutationFetch('/api/v1/backup/download', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      dir: opts?.dir || undefined,
      includeSecrets: opts?.includeSecrets || undefined,
    }),
  })
  if (!res.ok) {
    const data = await res.json().catch(() => ({}))
    throw ApiError.fromPayload(data, res.statusText)
  }
  const files = Number(res.headers.get('X-Camunda-Lab-Backup-Files') || '0')
  const blob = await res.blob()
  const disposition = res.headers.get('Content-Disposition') || ''
  const match = /filename="?([^";]+)"?/i.exec(disposition)
  const filename =
    match?.[1]?.trim() ||
    `camunda-lab-backup-${new Date().toISOString().slice(0, 19).replace(/[:T]/g, '-')}.tar.gz`
  return { blob, filename, files }
}

export async function saveBackupArchive(opts?: {
  dir?: string
  includeSecrets?: boolean
}): Promise<{ files: number; filename: string }> {
  const { blob, filename, files } = await downloadBackupArchive(opts)
  triggerBrowserDownload(blob, filename)
  return { files, filename }
}

export function downloadArtifactContents(
  artifacts: Array<{ path: string; content?: string; mediaType?: string }>,
): void {
  for (const artifact of artifacts) {
    if (artifact.content === undefined) continue
    const name = artifact.path.split(/[/\\]/).pop() || 'artifact'
    triggerBrowserDownload(
      new Blob([artifact.content], { type: artifact.mediaType || 'text/plain' }),
      name,
    )
  }
}
