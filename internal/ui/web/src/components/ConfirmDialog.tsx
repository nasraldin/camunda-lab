import { useEffect, useRef, useState } from 'react'

export type ConfirmAction = {
  title: string
  message: string
  requiredText?: string
  confirmLabel: string
  run: () => Promise<void>
}

type Props = {
  action: ConfirmAction
  onClose: () => void
}

const focusableSelector =
  'button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), a[href]'

export function ConfirmDialog({ action, onClose }: Props) {
  const dialogRef = useRef<HTMLDivElement>(null)
  const invokingElement = useRef<HTMLElement | null>(
    document.activeElement instanceof HTMLElement ? document.activeElement : null,
  )
  const [typedText, setTypedText] = useState('')
  const [busy, setBusy] = useState(false)

  const canConfirm = !action.requiredText || typedText === action.requiredText

  useEffect(() => {
    const restoreFocus = invokingElement.current
    const dialog = dialogRef.current
    const first = dialog?.querySelector<HTMLElement>(focusableSelector)
    first?.focus()

    function onKeyDown(event: KeyboardEvent) {
      if (event.key === 'Escape' && !busy) {
        event.preventDefault()
        onClose()
        return
      }
      if (event.key !== 'Tab' || !dialog) return

      const focusable = Array.from(dialog.querySelectorAll<HTMLElement>(focusableSelector))
      if (focusable.length === 0) {
        event.preventDefault()
        dialog.focus()
        return
      }
      const firstItem = focusable[0]!
      const lastItem = focusable[focusable.length - 1]!
      if (event.shiftKey && document.activeElement === firstItem) {
        event.preventDefault()
        lastItem.focus()
      } else if (!event.shiftKey && document.activeElement === lastItem) {
        event.preventDefault()
        firstItem.focus()
      }
    }

    window.addEventListener('keydown', onKeyDown)
    return () => {
      window.removeEventListener('keydown', onKeyDown)
      restoreFocus?.focus()
    }
  }, [busy, onClose])

  async function confirm() {
    if (!canConfirm || busy) return
    setBusy(true)
    try {
      await action.run()
      onClose()
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="modal-backdrop" role="presentation">
      <div
        ref={dialogRef}
        className="modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="confirm-dialog-title"
        tabIndex={-1}
      >
        <div className="modal-head">
          <h2 id="confirm-dialog-title">{action.title}</h2>
        </div>
        <p>{action.message}</p>
        {action.requiredText && (
          <label className="field">
            Type {action.requiredText} to confirm
            <input
              value={typedText}
              onChange={(event) => setTypedText(event.target.value)}
              autoComplete="off"
              disabled={busy}
              autoFocus
            />
          </label>
        )}
        <div className="row modal-actions">
          <button type="button" onClick={onClose} disabled={busy}>
            Cancel
          </button>
          <button
            type="button"
            className="danger"
            disabled={busy || !canConfirm}
            onClick={() => void confirm()}
          >
            {busy ? 'Working…' : action.confirmLabel}
          </button>
        </div>
      </div>
    </div>
  )
}
