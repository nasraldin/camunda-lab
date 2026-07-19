import { useEffect, useState } from 'react'
import { ApiError, getEnv, getK8sStatus, postForm, toolkitJSON, type ToolkitResult } from '../api'
import { getProjectDir, setProjectDir } from '../projectDir'

export function ProjectPage() {
  const [projectPath, setProjectPath] = useState(getProjectDir() || '/tmp/cam-demo')
  const [name, setName] = useState('cam-demo')
  const [profile, setProfile] = useState('full')
  const [resources, setResources] = useState('balanced')
  const [force, setForce] = useState(false)
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')
  const [output, setOutput] = useState('')
  const [cli, setCli] = useState('')
  const [active, setActive] = useState('lab')
  const [profiles, setProfiles] = useState<Array<{ name: string; kind: string }>>([])
  const [envName, setEnvName] = useState('')
  const [orch, setOrch] = useState('')
  const [k8sComponent, setK8sComponent] = useState('orchestration')
  const [replicas, setReplicas] = useState(1)

  async function run(label: string, fn: () => Promise<ToolkitResult>) {
    setBusy(label)
    setError('')
    setOutput('')
    try {
      const r = await fn()
      setOutput(
        r.output || r.hint || (r.path ? `Wrote ${r.path}` : r.ok ? 'OK' : r.error || 'Done'),
      )
      setCli(r.cli || '')
      if (!r.ok && (r.error || r.hint)) setError(r.error || r.hint || '')
      return r
    } catch (e) {
      setError(e instanceof ApiError ? e.message : e instanceof Error ? e.message : String(e))
      return null
    } finally {
      setBusy('')
    }
  }

  async function refreshEnv() {
    const r = await run('env', () => getEnv())
    if (r?.active) setActive(r.active)
    if (r?.profiles) setProfiles(r.profiles.map((p) => ({ name: p.name, kind: p.kind })))
  }

  useEffect(() => {
    void refreshEnv()
  }, [])

  return (
    <div className="stack">
      <div className="page-head">
        <h1>Project</h1>
        <p className="lead">
          Init scaffold, env profiles, backup/restore, and optional Kubernetes helpers.
        </p>
      </div>

      <section className="card panel">
        <header className="panel-head">
          <h2>Init project</h2>
        </header>
        <p className="hint">
          Equivalent CLI: <code>camunda init DIR -y</code>
        </p>
        <label className="field">
          <span>Directory (absolute)</span>
          <input value={projectPath} onChange={(e) => setProjectPath(e.target.value)} />
        </label>
        <label className="field">
          <span>Name</span>
          <input value={name} onChange={(e) => setName(e.target.value)} />
        </label>
        <label className="field">
          <span>Profile hint</span>
          <select value={profile} onChange={(e) => setProfile(e.target.value)}>
            <option value="light">light</option>
            <option value="full">full</option>
            <option value="modeler">modeler</option>
          </select>
        </label>
        <label className="field">
          <span>Resources hint</span>
          <select value={resources} onChange={(e) => setResources(e.target.value)}>
            <option value="small">small</option>
            <option value="balanced">balanced</option>
            <option value="power">power</option>
          </select>
        </label>
        <label className="pref-switch">
          <input type="checkbox" checked={force} onChange={(e) => setForce(e.target.checked)} />
          <span className="pref-switch-track" aria-hidden />
          <span className="pref-switch-label">Force (allow non-empty dir)</span>
        </label>
        <div className="panel-actions">
          <button
            type="button"
            className="primary"
            disabled={!!busy}
            onClick={() => {
              setProjectDir(projectPath.trim())
              void run('init', () =>
                toolkitJSON('/api/v1/project/init', {
                  dir: projectPath.trim(),
                  name,
                  profile,
                  resources,
                  force,
                  version: '8.9',
                }),
              )
            }}
          >
            {busy === 'init' ? 'Creating…' : 'Scaffold project'}
          </button>
        </div>
      </section>

      <section className="card panel">
        <header className="panel-head">
          <h2>Environments</h2>
        </header>
        <p className="hint">
          Active: <code>{active}</code> · CLI: <code>camunda env list</code>
        </p>
        <div className="url-list">
          {profiles.map((p) => (
            <div className="url-row" key={p.name}>
              <div className="url-row-label">
                {p.name} <span className="hint">({p.kind})</span>
              </div>
              <code className="url-row-value">{p.name === active ? 'active' : ''}</code>
              <button
                type="button"
                className="btn-sm"
                disabled={!!busy || p.name === active}
                onClick={() =>
                  void run('use', () => toolkitJSON('/api/v1/env/use', { name: p.name })).then(() =>
                    refreshEnv(),
                  )
                }
              >
                Use
              </button>
              {p.name !== 'lab' && (
                <button
                  type="button"
                  className="btn-sm"
                  disabled={!!busy}
                  onClick={() =>
                    void run('rm', () =>
                      toolkitJSON(`/api/v1/env/${encodeURIComponent(p.name)}`, undefined, 'DELETE'),
                    ).then(() => refreshEnv())
                  }
                >
                  Remove
                </button>
              )}
            </div>
          ))}
        </div>
        <div className="stack-sm" style={{ marginTop: '1rem' }}>
          <label className="field">
            <span>Add remote name</span>
            <input
              value={envName}
              onChange={(e) => setEnvName(e.target.value)}
              placeholder="staging"
            />
          </label>
          <label className="field">
            <span>Orchestration URL</span>
            <input
              value={orch}
              onChange={(e) => setOrch(e.target.value)}
              placeholder="https://…/v2"
            />
          </label>
          <button
            type="button"
            disabled={!!busy || !envName.trim()}
            onClick={() =>
              void run('add', () =>
                toolkitJSON('/api/v1/env', {
                  name: envName.trim(),
                  kind: 'remote',
                  orchestration: orch.trim(),
                }),
              ).then(() => refreshEnv())
            }
          >
            Add remote profile
          </button>
        </div>
      </section>

      <section className="card panel">
        <header className="panel-head">
          <h2>Backup &amp; restore</h2>
        </header>
        <p className="hint">
          CLI: <code>camunda backup -o …</code> / <code>camunda restore … --yes</code>
        </p>
        <div className="panel-actions">
          <button
            type="button"
            className="primary"
            disabled={!!busy}
            onClick={() =>
              void run('backup', () =>
                toolkitJSON('/api/v1/backup', { dir: projectPath.trim() || undefined }),
              )
            }
          >
            {busy === 'backup' ? 'Backing up…' : 'Create backup'}
          </button>
        </div>
        <label className="field">
          <span>Restore archive</span>
          <input
            type="file"
            accept=".gz,.tgz,application/gzip"
            onChange={(e) => {
              const f = e.target.files?.[0]
              if (!f) return
              const fd = new FormData()
              fd.append('archive', f)
              fd.append('yes', 'true')
              if (projectPath.trim()) fd.append('dir', projectPath.trim())
              void run('restore', () => postForm('/api/v1/restore', fd))
            }}
          />
        </label>
      </section>

      <section className="card panel">
        <header className="panel-head">
          <h2>Kubernetes</h2>
        </header>
        <p className="hint">
          Optional — needs kubectl + Camunda Helm. Compose-only labs can skip this.
        </p>
        <div className="panel-actions">
          <button
            type="button"
            disabled={!!busy}
            onClick={() => void run('k8s', () => getK8sStatus())}
          >
            {busy === 'k8s' ? 'Checking…' : 'k8s status'}
          </button>
        </div>
        <label className="field">
          <span>Component</span>
          <input value={k8sComponent} onChange={(e) => setK8sComponent(e.target.value)} />
        </label>
        <label className="field">
          <span>Replicas (scale)</span>
          <input
            type="number"
            min={0}
            value={replicas}
            onChange={(e) => setReplicas(Number(e.target.value))}
          />
        </label>
        <div className="panel-actions">
          <button
            type="button"
            disabled={!!busy}
            onClick={() =>
              void run('k8s-restart', () =>
                toolkitJSON('/api/v1/k8s/restart', { component: k8sComponent, confirm: true }),
              )
            }
          >
            Restart
          </button>
          <button
            type="button"
            disabled={!!busy}
            onClick={() =>
              void run('k8s-scale', () =>
                toolkitJSON('/api/v1/k8s/scale', {
                  component: k8sComponent,
                  replicas,
                  confirm: true,
                }),
              )
            }
          >
            Scale
          </button>
        </div>
      </section>

      {error && <div className="banner error">{error}</div>}
      {output && (
        <section className="card panel">
          {cli && (
            <p className="hint">
              CLI: <code>{cli}</code>
            </p>
          )}
          <pre className="code">{output}</pre>
        </section>
      )}
    </div>
  )
}
