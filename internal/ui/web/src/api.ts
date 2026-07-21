import { ApiError } from './apiError'

export { ApiError }

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
  }
  supportedVersions: string[]
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
  { Name: string; URL: string; Notes?: string } | { name: string; url: string; notes?: string }

async function parse<T>(res: Response): Promise<T> {
  const data = await res.json()
  if (!res.ok) {
    throw ApiError.fromPayload(data, res.statusText)
  }
  return data as T
}

let sessionToken: Promise<string> | undefined

async function getCSRFToken(): Promise<string> {
  sessionToken ??= fetch('/api/v1/session')
    .then((res) => parse<{ csrfToken: string }>(res))
    .then((session) => session.csrfToken)
    .catch((error) => {
      sessionToken = undefined
      throw error
    })
  return sessionToken
}

async function mutationFetch(
  path: string,
  init: RequestInit,
  refreshed = false,
): Promise<Response> {
  const headers = new Headers(init.headers)
  headers.set('X-Camunda-Lab-CSRF', await getCSRFToken())
  const res = await fetch(path, { ...init, headers })

  if (!refreshed && res.status === 403) {
    const payload = (await res.clone().json().catch(() => null)) as { code?: string } | null
    if (payload?.code === 'csrf_invalid') {
      sessionToken = undefined
      return mutationFetch(path, init, true)
    }
  }

  return res
}

export async function getOverview(): Promise<Overview> {
  return parse(await fetch('/api/v1/overview'))
}

export async function postJSON(path: string, body?: unknown): Promise<unknown> {
  return parse(
    await mutationFetch(path, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: body === undefined ? '{}' : JSON.stringify(body),
    }),
  )
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
  return parse(await fetch('/api/v1/urls'))
}

export async function getContainers(): Promise<{ containers: Container[] }> {
  return parse(await fetch('/api/v1/containers'))
}

export async function getDoctor(): Promise<{ ok: boolean; report: string }> {
  return parse(await fetch('/api/v1/doctor'))
}

export async function getSmoke(): Promise<{
  OK: boolean
  Checks: Array<{ Name: string; URL: string; OK: boolean; Detail: string }>
}> {
  return parse(await fetch('/api/v1/smoke'))
}

export type ProbeResult = {
  name: string
  ok: boolean
  kind: string
  checkedURL: string
  detail: string
}

export async function probeURL(name: string): Promise<ProbeResult> {
  return parse(await fetch(`/api/v1/probe?name=${encodeURIComponent(name)}`))
}

export async function getAIStatus(): Promise<{
  enabled: boolean
  openaiKey: string
  anthropicKey: string
  openaiBaseUrl: string
  supported: boolean
  supportError: string
}> {
  return parse(await fetch('/api/v1/ai/status'))
}

export async function getAIConfig(): Promise<{ config: string }> {
  return parse(await fetch('/api/v1/ai/config'))
}

export async function getC8ctlStatus(): Promise<{ installed: boolean; path: string }> {
  return parse(await fetch('/api/v1/tools/c8ctl/status'))
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

export async function getUpdate(): Promise<UpdateInfo> {
  return parse(await fetch('/api/v1/update'))
}

export async function postUpdate(): Promise<UpdateResult> {
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

export type ToolkitResult = {
  ok: boolean
  output?: string
  error?: string
  hint?: string
  cli?: string
  findings?: unknown[]
  changes?: unknown[]
  markdown?: string
  paths?: string[]
  contents?: Record<string, string>
  items?: unknown[]
  timeline?: unknown
  plan?: unknown
  drift?: unknown
  report?: string
  sections?: unknown[]
  active?: string
  profiles?: Array<{ name: string; kind: string }>
  path?: string
  dir?: string
}

export async function getDoctorDeep(): Promise<{ ok: boolean; report: string }> {
  return parse(await fetch('/api/v1/doctor/deep'))
}

export async function postForm(path: string, form: FormData): Promise<ToolkitResult> {
  return parse(await mutationFetch(path, { method: 'POST', body: form }))
}

export async function toolkitJSON(
  path: string,
  body?: unknown,
  method = 'POST',
): Promise<ToolkitResult> {
  const init: RequestInit = { method }
  if (method !== 'GET' && method !== 'HEAD') {
    init.headers = { 'Content-Type': 'application/json' }
    init.body = body === undefined ? '{}' : JSON.stringify(body)
  }
  return parse(await mutationFetch(path, init))
}

export async function getIncidents(): Promise<ToolkitResult> {
  return parse(await fetch('/api/v1/incidents'))
}

export async function getTrace(instanceKey: string): Promise<ToolkitResult> {
  return parse(await fetch(`/api/v1/trace/${encodeURIComponent(instanceKey)}`))
}

export async function getEnv(): Promise<ToolkitResult> {
  return parse(await fetch('/api/v1/env'))
}

export async function getK8sStatus(): Promise<ToolkitResult> {
  return parse(await fetch('/api/v1/k8s/status'))
}
