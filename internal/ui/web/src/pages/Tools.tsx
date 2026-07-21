import { useEffect, useState } from 'react'
import { getC8ctlStatus, postForm, postJSON } from '../api'

export function ToolsPage() {
  const [c8, setC8] = useState<{ installed: boolean; path: string } | null>(null)
  const [error, setError] = useState('')
  const [msg, setMsg] = useState('')
  const [busy, setBusy] = useState('')
  const [deployOut, setDeployOut] = useState('')

  async function refresh() {
    setC8(await getC8ctlStatus())
  }

  useEffect(() => {
    void refresh().catch((e) => setError(e instanceof Error ? e.message : String(e)))
  }, [])

  return (
    <div className="stack">
      <div className="page-head">
        <h1>Extras</h1>
        <p className="lead">
          Optional helpers for Desktop Modeler and deploying process diagrams. Skip this if you only
          use the web apps.
        </p>
      </div>
      {error && <div className="banner error">{error}</div>}
      {msg && <div className="banner ok">{msg}</div>}
      <div className="card stack">
        <div className="section-title">Command-line deploy tool (c8ctl)</div>
        <p className="hint">
          {c8?.installed ? `Ready at ${c8.path}` : 'Not installed on this computer yet.'}
        </p>
        <div className="row">
          <button
            type="button"
            disabled={!!busy}
            onClick={async () => {
              setBusy('c8')
              setError('')
              try {
                await postJSON('/api/v1/tools/c8ctl/install')
                setMsg('Deploy tool installed.')
                await refresh()
              } catch (e) {
                setError(e instanceof Error ? e.message : String(e))
              } finally {
                setBusy('')
              }
            }}
          >
            {busy === 'c8' ? 'Installing…' : 'Install deploy tool'}
          </button>
          <button type="button" disabled={!!busy} onClick={() => void refresh()}>
            Refresh
          </button>
        </div>
      </div>
      <div className="card stack">
        <div className="section-title">Desktop Modeler</div>
        <p className="hint">
          Create a connection named camunda-lab so Desktop Modeler can talk to this lab.
        </p>
        <button
          type="button"
          disabled={!!busy}
          onClick={async () => {
            setBusy('modeler')
            setError('')
            try {
              const r = (await postJSON('/api/v1/tools/modeler/profile')) as { path: string }
              setMsg(`Connection saved → ${r.path}`)
            } catch (e) {
              setError(e instanceof Error ? e.message : String(e))
            } finally {
              setBusy('')
            }
          }}
        >
          Save Modeler connection
        </button>
      </div>
      <div className="card stack">
        <div className="section-title">Upload a process</div>
        <p className="hint">
          Needs the deploy tool above. Choose a .bpmn file to send it to your lab.
        </p>
        <input
          type="file"
          accept=".bpmn,.BPMN"
          disabled={!c8?.installed || !!busy}
          onChange={async (e) => {
            const file = e.target.files?.[0]
            if (!file) return
            setBusy('deploy')
            setError('')
            setDeployOut('')
            try {
              const fd = new FormData()
              fd.append('file', file)
              const data = await postForm('/api/v1/tools/deploy', fd)
              setDeployOut(data.output || 'ok')
              setMsg('Process uploaded.')
            } catch (err) {
              setError(err instanceof Error ? err.message : String(err))
            } finally {
              setBusy('')
            }
          }}
        />
        {deployOut && <pre className="code">{deployOut}</pre>}
      </div>
    </div>
  )
}
