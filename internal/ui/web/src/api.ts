export { ApiError } from './apiError'
export {
  getCSRFToken,
  getJSON,
  mutationFetch,
  parseJSON,
  postForm,
  postJSON,
  toolkitJSON,
  triggerBrowserDownload,
  triggerTextDownload,
} from './api/client'
export * from './api/toolkit'
export type * from './api/types'

import { getJSON } from './api/client'
import type { ToolkitEnvelope } from './api/types'

export type Overview = {
  cliVersion: string
  labHome: string
  configured: boolean
  config: {
    version: string
    profile: string
    resources: string
    host: string
    project: string
    aiEnabled: boolean
    monitoringEnabled: boolean
  }
  supportedVersions: string[]
  defaultVersion: string
  containers?: Container[]
  running?: number
  total?: number
  containersError?: string
}

export type Container = {
  name: string
  service: string
  image: string
  state: string
  health: string
  status: string
  uptime?: string
  ports?: string
}

export type UrlEntry =
  | { Name: string; URL: string; Notes?: string }
  | { name: string; url: string; notes?: string }

export type ProbeResult = {
  name: string
  ok: boolean
  kind: string
  checkedURL: string
  detail: string
}

export type UpdateInfo = {
  current: string
  latest: string
  updateAvailable: boolean
  channel: 'homebrew' | 'release' | 'dev'
  executable: string
  releaseURL: string
  publishedAt?: string
  error?: string
}

export type UpdateResult = {
  ok: boolean
  channel?: string
  output?: string
  restartHint?: string
  error?: string
}

/** @deprecated Use ToolkitEnvelope from ./api/types */
export type ToolkitResult = ToolkitEnvelope

export async function getOverview(): Promise<Overview> {
  return getJSON<Overview>('/api/v1/overview')
}

export async function getURLs(): Promise<{
  urls: Array<{
    Name?: string
    name?: string
    URL?: string
    url?: string
    Notes?: string
    notes?: string
  }>
}> {
  return getJSON('/api/v1/urls')
}

export async function getContainers(): Promise<{ containers: Container[] }> {
  return getJSON('/api/v1/containers')
}

export async function getDoctor(): Promise<{ ok: boolean; report: string }> {
  return getJSON('/api/v1/doctor')
}

export async function getSmoke(): Promise<{
  OK: boolean
  Checks: Array<{ Name: string; URL: string; OK: boolean; Detail: string }>
}> {
  return getJSON('/api/v1/smoke')
}

export async function probeURL(name: string): Promise<ProbeResult> {
  return getJSON(`/api/v1/probe?name=${encodeURIComponent(name)}`)
}

export async function getAIStatus(): Promise<{
  enabled: boolean
  openaiKey: string
  anthropicKey: string
  openaiBaseUrl: string
  supported: boolean
  supportError: string
}> {
  return getJSON('/api/v1/ai/status')
}

export async function getAIConfig(): Promise<{ config: string }> {
  return getJSON('/api/v1/ai/config')
}

export async function getMonitoringStatus(): Promise<{
  enabled: boolean
  grafana: string
  prometheus: string
}> {
  return getJSON('/api/v1/monitoring/status')
}

export async function getC8ctlStatus(): Promise<{ installed: boolean; path: string }> {
  return getJSON('/api/v1/tools/c8ctl/status')
}

export async function getUpdate(): Promise<UpdateInfo> {
  return getJSON('/api/v1/update')
}

export async function postUpdate(): Promise<UpdateResult> {
  const { mutationFetch } = await import('./api/client')
  const res = await mutationFetch('/api/v1/update', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: '{}',
  })
  const data = (await res.json()) as UpdateResult
  if (!res.ok && !data.error) {
    data.error = res.statusText
    data.ok = false
  }
  return data
}

export async function getDoctorDeep(): Promise<{ ok: boolean; report: string }> {
  return getJSON('/api/v1/doctor/deep')
}

/** @deprecated Use downloadBackupArchive/saveBackupArchive from ./api/toolkit */
export async function downloadBackup(dir?: string): Promise<{ files: number; blob: Blob }> {
  const { downloadBackupArchive } = await import('./api/toolkit')
  const { blob, files } = await downloadBackupArchive({ dir })
  return { files, blob }
}
