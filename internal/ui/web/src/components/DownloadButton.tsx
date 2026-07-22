import { useState } from 'react'
import { ApiError } from '../apiError'
import { triggerBrowserDownload } from '../api/client'

type Props = {
  label: string
  busyLabel?: string
  disabled?: boolean
  className?: string
  onDownload: () => Promise<{ blob: Blob; filename: string }>
  onComplete?: (info: { filename: string }) => void
  onError?: (message: string) => void
}

export function DownloadButton({
  label,
  busyLabel = 'Downloading…',
  disabled,
  className,
  onDownload,
  onComplete,
  onError,
}: Props) {
  const [busy, setBusy] = useState(false)

  async function run() {
    setBusy(true)
    try {
      const { blob, filename } = await onDownload()
      triggerBrowserDownload(blob, filename)
      onComplete?.({ filename })
    } catch (error) {
      const message =
        error instanceof ApiError
          ? error.message
          : error instanceof Error
            ? error.message
            : String(error)
      onError?.(message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <button
      type="button"
      className={className || 'primary'}
      disabled={disabled || busy}
      onClick={() => void run()}
    >
      {busy ? busyLabel : label}
    </button>
  )
}
