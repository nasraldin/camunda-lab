import { useEffect, useRef, useState } from 'react'
import { ApiError, getOverview } from '../api'
import {
  addEnv,
  downloadBackupArchive,
  getEnv,
  initProject,
  removeEnv,
  restoreBackup,
  useEnv as activateEnv,
} from '../api/toolkit'
import type { ToolkitEnvelope } from '../api/types'
import { ActionResult } from '../components/ActionResult'
import { ConfirmDialog, type ConfirmAction } from '../components/ConfirmDialog'
import { DownloadButton } from '../components/DownloadButton'
import { getProjectDir, setProjectDir } from '../projectDir'

export function ProjectPage() {
  const [projectPath, setProjectPath] = useState(getProjectDir() || '/tmp/cam-demo')
  const [name, setName] = useState('cam-demo')
  const [profile, setProfile] = useState('full')
  const [resources, setResources] = useState('balanced')
  const [version, setVersion] = useState('')
  const [supportedVersions, setSupportedVersions] = useState<string[]>([])
  const [force, setForce] = useState(false)
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')
  const [errorCode, setErrorCode] = useState<string | null>(null)
  const [result, setResult] = useState<ToolkitEnvelope | null>(null)
  const [active, setActive] = useState('lab')
  const [profiles, setProfiles] = useState<Array<{ name: string; kind: string }>>([])
  const [envName, setEnvName] = useState('')
  const [orch, setOrch] = useState('')
  const [restoreForce, setRestoreForce] = useState(false)
  const [confirmAction, setConfirmAction] = useState<ConfirmAction | null>(null)
  const restoreInputRef = useRef<HTMLInputElement>(null)

  async function run(label: string, fn: () => Promise<ToolkitEnvelope>) {
    setBusy(label)
    setError('')
    setErrorCode(null)
    setResult(null)
    try {
      const r = await fn()
      setResult(r)
      if (!r.ok && (r.error || r.hint)) {
        setError(r.error || r.hint || '')
        setErrorCode(r.code ?? null)
      }
      return r
    } catch (e) {
      if (e instanceof ApiError) {
        setError(e.message)
        setErrorCode(e.code || null)
      } else {
        setError(e instanceof Error ? e.message : String(e))
      }
      return null
    } finally {
      setBusy('')
    }
  }

  async function refreshEnv() {
    const dir = projectPath.trim()
    const r = await run('env', () => getEnv(dir ? { dir } : undefined))
    if (r?.active) setActive(r.active)
    if (r?.profiles) setProfiles(r.profiles.map((p) => ({ name: p.name, kind: p.kind })))
  }

  useEffect(() => {
    void refreshEnv()
    void getOverview()
      .then((overview) => {
        setSupportedVersions(overview.supportedVersions)
        setVersion(
          overview.config.version ||
            overview.defaultVersion ||
            overview.supportedVersions[0] ||
            '',
        )
      })
      .catch((e) =>
        setError(e instanceof ApiError ? e.message : e instanceof Error ? e.message : String(e)),
      )
  }, [])

  return (
    <div className="stack">
      <div className="page-head">
        <h1>Project</h1>
        <p className="lead">
          Init scaffold, env profiles, and backup/restore for local Docker Compose labs.
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
          <span>Camunda version hint</span>
          <select value={version} onChange={(e) => setVersion(e.target.value)}>
            {supportedVersions.map((supported) => (
              <option key={supported} value={supported}>
                {supported}
              </option>
            ))}
          </select>
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
                initProject({
                  dir: projectPath.trim(),
                  name,
                  profile,
                  resources,
                  force,
                  version,
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
          {projectPath.trim() ? ' · uses scaffolded project path when set' : ''}
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
                onClick={() => {
                  const dir = projectPath.trim()
                  void run('use', () =>
                    activateEnv({
                      name: p.name,
                      ...(dir ? { dir } : {}),
                    }),
                  ).then(() => refreshEnv())
                }}
              >
                Use
              </button>
              {p.name !== 'lab' && (
                <button
                  type="button"
                  className="btn-sm"
                  disabled={!!busy}
                  onClick={() =>
                    setConfirmAction({
                      title: 'Remove environment',
                      message: `Remove the ${p.name} environment profile? This does not delete the remote environment.`,
                      confirmLabel: 'Remove environment',
                      run: async () => {
                        const dir = projectPath.trim()
                        await run('rm', () => removeEnv(p.name, dir || undefined))
                        await refreshEnv()
                      },
                    })
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
            onClick={() => {
              const dir = projectPath.trim()
              void run('add', () =>
                addEnv({
                  name: envName.trim(),
                  kind: 'remote',
                  orchestration: orch.trim(),
                  ...(dir ? { dir } : {}),
                }),
              ).then(() => refreshEnv())
            }}
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
          <DownloadButton
            label="Download backup (gzip)"
            busyLabel="Backing up…"
            disabled={!!busy}
            onDownload={() =>
              downloadBackupArchive({ dir: projectPath.trim() || undefined }).then(
                ({ blob, filename }) => ({ blob, filename }),
              )
            }
            onComplete={({ filename }) => {
              setResult({
                ok: true,
                output: `Downloaded ${filename}`,
                cli: 'camunda backup -o ./lab-backup.tar.gz',
              })
              setError('')
            }}
            onError={setError}
          />
        </div>
        <label className="pref-switch">
          <input
            type="checkbox"
            checked={restoreForce}
            onChange={(e) => setRestoreForce(e.target.checked)}
          />
          <span className="pref-switch-track" aria-hidden />
          <span className="pref-switch-label">Force restore while lab is running</span>
        </label>
        <label className="field">
          <span>Restore archive</span>
          <input
            ref={restoreInputRef}
            type="file"
            accept=".gz,.tgz,application/gzip"
            onChange={(e) => {
              const f = e.target.files?.[0]
              if (!f) return
              setConfirmAction({
                title: 'Restore backup',
                message:
                  'Restoring replaces the current lab files with the selected archive. Type RESTORE to continue.',
                requiredText: 'RESTORE',
                confirmLabel: 'Restore backup',
                run: async () => {
                  const fd = new FormData()
                  fd.append('archive', f)
                  fd.append('yes', 'true')
                  if (restoreForce) fd.append('force', 'true')
                  if (projectPath.trim()) fd.append('dir', projectPath.trim())
                  await run('restore', () => restoreBackup(fd))
                },
              })
            }}
          />
        </label>
      </section>

      {(busy || error || result) && (
        <ActionResult
          loading={!!busy}
          error={error}
          code={errorCode}
          result={result}
          loadingLabel={
            busy === 'init'
              ? 'Creating project…'
              : busy === 'backup'
                ? 'Preparing backup…'
                : busy === 'restore'
                  ? 'Restoring backup…'
                  : 'Working…'
          }
          idleLabel=""
          downloadFilename={`camunda-lab-${busy || 'project'}.txt`}
        />
      )}

      {confirmAction && (
        <ConfirmDialog
          action={confirmAction}
          onClose={() => {
            if (confirmAction.requiredText === 'RESTORE' && restoreInputRef.current) {
              restoreInputRef.current.value = ''
            }
            setConfirmAction(null)
          }}
        />
      )}
    </div>
  )
}
