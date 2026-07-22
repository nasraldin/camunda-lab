import { useEffect, useState } from 'react'
import { ApiError } from '../api'
import {
  getIncidents,
  getTrace,
  retryIncident,
  runDrift,
  runPlan,
} from '../api/toolkit'
import type { IncidentItem, ToolkitEnvelope } from '../api/types'
import { ActionResult } from '../components/ActionResult'
import { ConfirmDialog, type ConfirmAction } from '../components/ConfirmDialog'
import { getProjectDir, setProjectDir } from '../projectDir'

function incidentKey(it: IncidentItem): string {
  return (it.key || it.id || it.ID || '').trim()
}

function incidentError(it: IncidentItem): string {
  return it.errorMessage || it.error || it.Error || ''
}

function incidentProcess(it: IncidentItem): string {
  return it.processDefinitionId || it.process || it.Process || ''
}

export function ClusterPage() {
  const [projectPath, setProjectPath] = useState(getProjectDir())
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')
  const [errorCode, setErrorCode] = useState<string | null>(null)
  const [result, setResult] = useState<ToolkitEnvelope | null>(null)
  const [incidents, setIncidents] = useState<IncidentItem[]>([])
  const [instanceKey, setInstanceKey] = useState('')
  const [traceFollow, setTraceFollow] = useState(false)
  const [confirmAction, setConfirmAction] = useState<ConfirmAction | null>(null)

  function saveDir() {
    setProjectDir(projectPath.trim())
  }

  async function run(label: string, fn: () => Promise<ToolkitEnvelope>) {
    setBusy(label)
    setError('')
    setErrorCode(null)
    setResult(null)
    try {
      const r = await fn()
      setResult(r)
      if (!r.ok && r.error) {
        setError(r.error)
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

  async function refreshIncidents() {
    const dir = projectPath.trim()
    const r = await run('incidents', () => getIncidents(dir ? { dir } : undefined))
    if (r?.items) setIncidents(r.items as IncidentItem[])
  }

  useEffect(() => {
    void refreshIncidents()
  }, [])

  const incidentsBusy = busy === 'incidents'

  return (
    <div className="stack">
      <div className="page-head">
        <h1>Cluster</h1>
        <p className="lead">
          Incidents, trace, plan, and drift against the live Orchestration API (same as CLI).
        </p>
      </div>

      <section className="card panel">
        <header className="panel-head">
          <h2>Project path</h2>
        </header>
        <p className="hint">
          Used by Incidents / Trace / Plan / Drift for project-local active env. Must contain{' '}
          <code>.camunda.yaml</code> for plan and drift (see Project → Init).
        </p>
        <label className="field">
          <span>Absolute directory</span>
          <input
            value={projectPath}
            onChange={(e) => setProjectPath(e.target.value)}
            onBlur={saveDir}
            placeholder="/tmp/cam-demo"
          />
        </label>
      </section>

      <section className="card panel">
        <header className="panel-head">
          <h2>Incidents</h2>
        </header>
        <p className="hint">
          Equivalent CLI: <code>camunda incidents list</code>
          {projectPath.trim() ? ' (uses project path for active env)' : ''}
        </p>
        <div className="panel-actions">
          <button
            type="button"
            disabled={!!busy}
            onClick={() => {
              saveDir()
              void refreshIncidents()
            }}
          >
            {incidentsBusy ? 'Loading…' : 'Refresh'}
          </button>
        </div>
        {incidentsBusy ? (
          <ActionResult loading loadingLabel="Loading incidents…" />
        ) : incidents.length === 0 ? (
          <ActionResult empty emptyLabel="No incidents." />
        ) : (
          <div className="url-list">
            {incidents.map((it) => {
              const id = incidentKey(it)
              if (!id) return null
              return (
                <div className="url-row" key={id}>
                  <div className="url-row-label">{id}</div>
                  <code className="url-row-value">
                    {incidentError(it).slice(0, 120)} · {incidentProcess(it)}
                  </code>
                  <button
                    type="button"
                    className="btn-sm"
                    disabled={!!busy || !id}
                    onClick={() =>
                      setConfirmAction({
                        title: 'Retry incident',
                        message: `Retry incident ${id}. This resumes the affected workflow execution.`,
                        confirmLabel: 'Retry incident',
                        run: async () => {
                          const dir = projectPath.trim()
                          saveDir()
                          await run('retry', () =>
                            retryIncident(id, {
                              confirm: true,
                              ...(dir ? { dir } : {}),
                            }),
                          )
                          await refreshIncidents()
                        },
                      })
                    }
                  >
                    Retry
                  </button>
                </div>
              )
            })}
          </div>
        )}
      </section>

      <section className="card panel">
        <header className="panel-head">
          <h2>Trace</h2>
        </header>
        <p className="hint">
          Equivalent CLI: <code>camunda trace INSTANCE_KEY</code>
          {traceFollow ? ' --follow' : ''}
          {projectPath.trim() ? ' (uses project path for active env)' : ''}
        </p>
        <label className="field">
          <span>Process instance key</span>
          <input
            value={instanceKey}
            onChange={(e) => setInstanceKey(e.target.value)}
            placeholder="2251799813685249"
          />
        </label>
        <label className="pref-switch">
          <input
            type="checkbox"
            checked={traceFollow}
            onChange={(e) => setTraceFollow(e.target.checked)}
          />
          <span className="pref-switch-track" aria-hidden />
          <span className="pref-switch-label">Bounded follow (max 20 events / 30s)</span>
        </label>
        <div className="panel-actions">
          <button
            type="button"
            className="primary"
            disabled={!!busy || !instanceKey.trim()}
            onClick={() => {
              const dir = projectPath.trim()
              saveDir()
              void run('trace', () =>
                getTrace(instanceKey.trim(), {
                  ...(dir ? { dir } : {}),
                  ...(traceFollow
                    ? { follow: true, interval: '2s', timeout: '30s', maxEvents: 20 }
                    : {}),
                }),
              )
            }}
          >
            {busy === 'trace' ? 'Loading…' : traceFollow ? 'Follow timeline' : 'Show timeline'}
          </button>
        </div>
      </section>

      <section className="card panel">
        <header className="panel-head">
          <h2>Plan &amp; drift</h2>
        </header>
        <p className="hint">
          Equivalent CLI: <code>camunda plan --dir …</code> / <code>camunda drift --dir …</code>
        </p>
        <div className="panel-actions">
          <button
            type="button"
            className="primary"
            disabled={!!busy || !projectPath.trim()}
            onClick={() => {
              saveDir()
              void run('plan', () => runPlan({ dir: projectPath.trim() }))
            }}
          >
            {busy === 'plan' ? 'Planning…' : 'Run plan'}
          </button>
          <button
            type="button"
            disabled={!!busy || !projectPath.trim()}
            onClick={() => {
              saveDir()
              void run('drift', () => runDrift({ dir: projectPath.trim() }))
            }}
          >
            {busy === 'drift' ? 'Checking…' : 'Check drift'}
          </button>
        </div>
      </section>

      {(!!busy && busy !== 'incidents') || error || result ? (
        <ActionResult
          loading={!!busy && busy !== 'incidents'}
          error={error}
          code={errorCode}
          result={result}
          loadingLabel={
            busy === 'trace'
              ? traceFollow
                ? 'Following timeline…'
                : 'Loading trace…'
              : busy === 'plan'
                ? 'Running plan…'
                : busy === 'drift'
                  ? 'Checking drift…'
                  : busy === 'retry'
                    ? 'Retrying incident…'
                    : 'Working…'
          }
          idleLabel=""
          downloadFilename={`camunda-lab-${busy || 'cluster'}.txt`}
        />
      ) : null}

      {confirmAction && (
        <ConfirmDialog action={confirmAction} onClose={() => setConfirmAction(null)} />
      )}
    </div>
  )
}
