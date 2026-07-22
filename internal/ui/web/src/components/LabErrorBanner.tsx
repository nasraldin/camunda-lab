import { ApiError } from '../apiError'

type Props = {
  error: ApiError | string | null
  busy?: boolean
  onRecover?: () => void
}

export function LabErrorBanner({ error, busy, onRecover }: Props) {
  if (!error) return null
  const err = typeof error === 'string' ? new ApiError(error) : error
  return (
    <div className="banner error stack-tight">
      <p>{err.message}</p>
      {err.hint && <p className="hint">{err.hint}</p>}
      {err.recoverable && onRecover && (
        <div className="row">
          <button type="button" className="primary" disabled={!!busy} onClick={() => onRecover()}>
            {busy ? 'Cleaning up…' : 'Clean up and try again'}
          </button>
        </div>
      )}
    </div>
  )
}
