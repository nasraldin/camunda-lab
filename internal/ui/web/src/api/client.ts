import { ApiError } from '../apiError'

export { ApiError }

export async function parseJSON<T>(res: Response): Promise<T> {
  const data = await res.json()
  if (!res.ok) {
    throw ApiError.fromPayload(data, res.statusText)
  }
  return data as T
}

let sessionToken: Promise<string> | undefined

export async function getCSRFToken(): Promise<string> {
  sessionToken ??= fetch('/api/v1/session')
    .then((res) => parseJSON<{ csrfToken: string }>(res))
    .then((session) => session.csrfToken)
    .catch((error) => {
      sessionToken = undefined
      throw error
    })
  return sessionToken
}

export async function mutationFetch(
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

export async function getJSON<T>(path: string): Promise<T> {
  return parseJSON<T>(await fetch(path))
}

export async function postJSON<T>(path: string, body?: unknown): Promise<T> {
  return parseJSON<T>(
    await mutationFetch(path, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: body === undefined ? '{}' : JSON.stringify(body),
    }),
  )
}

export async function toolkitJSON<T>(
  path: string,
  body?: unknown,
  method = 'POST',
  signal?: AbortSignal,
): Promise<T> {
  const init: RequestInit = { method, signal }
  if (method !== 'GET' && method !== 'HEAD') {
    init.headers = { 'Content-Type': 'application/json' }
    init.body = body === undefined ? '{}' : JSON.stringify(body)
  }
  return parseJSON<T>(await mutationFetch(path, init))
}

export async function postForm<T>(
  path: string,
  form: FormData,
  signal?: AbortSignal,
): Promise<T> {
  return parseJSON<T>(await mutationFetch(path, { method: 'POST', body: form, signal }))
}

function filenameFromDisposition(header: string | null, fallback: string): string {
  if (!header) return fallback
  const match = /filename="?([^";]+)"?/i.exec(header)
  return match?.[1]?.trim() || fallback
}

export async function downloadBinary(
  path: string,
  init: RequestInit,
  fallbackFilename: string,
): Promise<{ blob: Blob; filename: string }> {
  const res = await mutationFetch(path, init)
  if (!res.ok) {
    const data = await res.json().catch(() => ({}))
    throw ApiError.fromPayload(data, res.statusText)
  }
  const blob = await res.blob()
  const filename = filenameFromDisposition(res.headers.get('Content-Disposition'), fallbackFilename)
  return { blob, filename }
}

export function triggerBrowserDownload(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = filename
  anchor.click()
  URL.revokeObjectURL(url)
}

export function triggerTextDownload(text: string, filename: string, mediaType = 'text/plain'): void {
  triggerBrowserDownload(new Blob([text], { type: mediaType }), filename)
}
