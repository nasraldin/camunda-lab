export class ApiError extends Error {
  hint?: string
  code?: string
  recoverable?: boolean

  static fromPayload(data: unknown, fallback: string): ApiError {
    const row = data as { error?: string; hint?: string; code?: string; recoverable?: boolean }
    const err = new ApiError(row.error || fallback)
    err.hint = row.hint
    err.code = row.code
    err.recoverable = row.recoverable
    return err
  }
}
