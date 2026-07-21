import { useState } from 'react'
import { postJSON } from '../api'
import {
  ConfirmActionModal,
  type ConfirmAction,
} from '../components/ConfirmActionModal'

export function DangerPage() {
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState('')
  const [msg, setMsg] = useState('')
  const [busy, setBusy] = useState(false)
  const [confirmAction, setConfirmAction] = useState<ConfirmAction | null>(null)

  async function resetLab() {
    setBusy(true)
    setError('')
    setMsg('')
    try {
      await postJSON('/api/v1/nuke', { confirm: 'DELETE' })
      setMsg('Lab deleted. Open Get started to install again.')
      setConfirm('')
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="stack">
      <div className="page-head">
        <h1>Reset lab</h1>
        <p className="lead">
          Permanently delete this lab’s files and data on your computer. You cannot undo this.
          Install again from Get started afterward.
        </p>
      </div>
      {error && <div className="banner error">{error}</div>}
      {msg && <div className="banner ok">{msg}</div>}
      <div className="card stack">
        <label className="field">
          Type DELETE to confirm
          <input
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
            placeholder="DELETE"
            autoComplete="off"
          />
        </label>
        <button
          type="button"
          className="danger"
          disabled={busy || confirm !== 'DELETE'}
          onClick={() =>
            setConfirmAction({
              title: 'Permanently delete lab',
              message:
                'This permanently deletes the lab files and data from this computer. This cannot be undone.',
              requiredText: 'DELETE',
              confirmLabel: 'Permanently delete lab',
              run: resetLab,
            })
          }
        >
          {busy ? 'Deleting…' : 'Delete everything'}
        </button>
      </div>
      {confirmAction && (
        <ConfirmActionModal action={confirmAction} onClose={() => setConfirmAction(null)} />
      )}
    </div>
  )
}
