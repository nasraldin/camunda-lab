import { useState } from 'react'
import { postJSON } from '../api'

export function DangerPage() {
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState('')
  const [msg, setMsg] = useState('')
  const [busy, setBusy] = useState(false)

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
          onClick={async () => {
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
          }}
        >
          {busy ? 'Deleting…' : 'Delete everything'}
        </button>
      </div>
    </div>
  )
}
