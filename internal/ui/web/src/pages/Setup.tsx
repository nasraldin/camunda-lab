import { useEffect, useMemo, useState } from 'react'
import { getOverview, postJSON, ApiError } from '../api'
import {
  ConfirmActionModal,
  type ConfirmAction,
} from '../components/ConfirmActionModal'
import { LabErrorBanner } from '../components/LabErrorBanner'

function nextVersion(supported: string[], current: string): string {
  const i = supported.indexOf(current)
  if (i >= 0 && i < supported.length - 1) return supported[i + 1]!
  const other = supported.find((v) => v !== current)
  return other || current
}

export function SetupPage() {
  const [versions, setVersions] = useState<string[]>(['8.7', '8.8', '8.9', '8.10'])
  const [currentVersion, setCurrentVersion] = useState('')
  const [version, setVersion] = useState('8.9')
  const [profile, setProfile] = useState('light')
  const [resources, setResources] = useState('small')
  const [wipe, setWipe] = useState(true)
  const [ai, setAI] = useState(false)
  const [openaiKey, setOpenaiKey] = useState('')
  const [anthropicKey, setAnthropicKey] = useState('')
  const [openaiBase, setOpenaiBase] = useState('')
  const [busy, setBusy] = useState('')
  const [msg, setMsg] = useState('')
  const [error, setError] = useState<ApiError | null>(null)
  const [lastOp, setLastOp] = useState<'install' | 'switch' | null>(null)
  const [configured, setConfigured] = useState(false)
  const [confirmAction, setConfirmAction] = useState<ConfirmAction | null>(null)

  useEffect(() => {
    void getOverview().then((o) => {
      const supported = o.supportedVersions?.length ? o.supportedVersions : versions
      setVersions(supported)
      setConfigured(o.configured)
      if (o.config.profile) setProfile(o.config.profile)
      if (o.config.resources) setResources(o.config.resources)
      if (o.config.version) {
        setCurrentVersion(o.config.version)
        setVersion(o.configured ? nextVersion(supported, o.config.version) : o.config.version)
      }
    })
  }, [])

  const sameAsCurrent = useMemo(
    () => configured && !!currentVersion && version === currentVersion,
    [configured, currentVersion, version],
  )

  async function recoverAndRetry(action: () => Promise<void>) {
    setBusy('recover')
    setError(null)
    try {
      await postJSON('/api/v1/recover')
      await action()
    } catch (e) {
      setError(e instanceof ApiError ? e : new ApiError(e instanceof Error ? e.message : String(e)))
    } finally {
      setBusy('')
    }
  }

  async function install() {
    setLastOp('install')
    setBusy('install')
    setError(null)
    setMsg('')
    try {
      await postJSON('/api/v1/install', {
        version,
        profile,
        resources,
        ai,
        openaiKey,
        anthropicKey,
        openaiBaseUrl: openaiBase,
      })
      setMsg(
        'Setup finished. Open Apps once the services are ready, or wait a minute and refresh Home.',
      )
      setConfigured(true)
      setCurrentVersion(version)
      setVersion(nextVersion(versions, version))
    } catch (e) {
      setError(e instanceof ApiError ? e : new ApiError(e instanceof Error ? e.message : String(e)))
    } finally {
      setBusy('')
    }
  }

  async function switchVersion() {
    setLastOp('switch')
    setBusy('switch')
    setError(null)
    setMsg('')
    try {
      await postJSON('/api/v1/switch', {
        version,
        wipe,
        ai,
        openaiKey,
        anthropicKey,
        openaiBaseUrl: openaiBase,
      })
      setMsg(`Moved to Camunda ${version}.`)
      setCurrentVersion(version)
      setVersion(nextVersion(versions, version))
    } catch (e) {
      setError(e instanceof ApiError ? e : new ApiError(e instanceof Error ? e.message : String(e)))
    } finally {
      setBusy('')
    }
  }

  async function applyProfile() {
    setBusy('profile')
    setError(null)
    try {
      await postJSON('/api/v1/profile', { profile })
      setMsg(`Lab size set to ${profile}.`)
    } catch (e) {
      setError(e instanceof ApiError ? e : new ApiError(e instanceof Error ? e.message : String(e)))
    } finally {
      setBusy('')
    }
  }

  async function applyResources() {
    setBusy('resources')
    setError(null)
    try {
      await postJSON('/api/v1/resources', { resources })
      setMsg(`Resource size set to ${resources}. Restart the lab to apply memory settings.`)
    } catch (e) {
      setError(e instanceof ApiError ? e : new ApiError(e instanceof Error ? e.message : String(e)))
    } finally {
      setBusy('')
    }
  }

  return (
    <div className="stack">
      <div className="page-head">
        <h1>Get started</h1>
        <p className="lead">
          Choose a Camunda version and how big the lab should be, then install. This can take
          several minutes the first time.
        </p>
      </div>
      {error && (
        <LabErrorBanner
          error={error}
          busy={busy === 'recover'}
          onRecover={
            error.recoverable && lastOp
              ? () =>
                  void recoverAndRetry(() => (lastOp === 'switch' ? switchVersion() : install()))
              : undefined
          }
        />
      )}
      {msg && <div className="banner ok">{msg}</div>}
      <div className="card stack">
        <label className="field">
          Camunda version
          <select value={version} onChange={(e) => setVersion(e.target.value)}>
            {versions.map((v) => (
              <option key={v} value={v}>
                {v}
                {v === currentVersion ? ' (current)' : ''}
                {v === '8.10' && v !== currentVersion ? ' (preview)' : ''}
              </option>
            ))}
          </select>
        </label>
        <label className="field">
          Lab size
          <select value={profile} onChange={(e) => setProfile(e.target.value)}>
            <option value="light">Light — fewer apps, less memory</option>
            <option value="full">Full — complete Camunda suite</option>
            <option value="modeler">Modeler — modeling tools focus</option>
          </select>
        </label>
        <label className="field">
          Computer resources
          <select value={resources} onChange={(e) => setResources(e.target.value)}>
            <option value="small">Small — laptops</option>
            <option value="balanced">Balanced</option>
            <option value="power">Power — more RAM</option>
          </select>
        </label>
        <label className="check">
          <input type="checkbox" checked={ai} onChange={(e) => setAI(e.target.checked)} />
          Turn on AI helpers (Camunda 8.9+, light or full)
        </label>
        {ai && (
          <>
            <label className="field">
              OpenAI API key
              <input
                type="password"
                value={openaiKey}
                onChange={(e) => setOpenaiKey(e.target.value)}
                autoComplete="off"
              />
            </label>
            <label className="field">
              Anthropic API key
              <input
                type="password"
                value={anthropicKey}
                onChange={(e) => setAnthropicKey(e.target.value)}
                autoComplete="off"
              />
            </label>
            <label className="field">
              Custom OpenAI-compatible URL (optional)
              <input
                value={openaiBase}
                onChange={(e) => setOpenaiBase(e.target.value)}
                placeholder="http://localhost:11434/v1"
              />
            </label>
          </>
        )}
        <div className="row">
          <button className="primary" disabled={!!busy} onClick={() => void install()}>
            {busy === 'install' ? 'Installing…' : configured ? 'Reinstall / start' : 'Install lab'}
          </button>
          <button disabled={!!busy || !configured} onClick={() => void applyProfile()}>
            Save lab size
          </button>
          <button disabled={!!busy || !configured} onClick={() => void applyResources()}>
            Save resources
          </button>
        </div>
      </div>
      <div className="card stack">
        <div className="section-title">Change Camunda version</div>
        <p className="hint">
          {currentVersion
            ? `You are on ${currentVersion}. Pick another version above, then move to it.`
            : 'Install a lab first, then you can change versions here.'}
        </p>
        <label className="check">
          <input type="checkbox" checked={wipe} onChange={(e) => setWipe(e.target.checked)} />
          Clear lab data when changing versions (recommended)
        </label>
        <button
          disabled={!!busy || !configured || sameAsCurrent}
          onClick={() => {
            if (!wipe) {
              void switchVersion()
              return
            }
            setConfirmAction({
              title: 'Clear data and switch version',
              message: `Move to Camunda ${version} and permanently clear the current lab data.`,
              confirmLabel: 'Clear data and switch',
              run: switchVersion,
            })
          }}
        >
          {busy === 'switch'
            ? 'Changing…'
            : sameAsCurrent
              ? `Already on ${version}`
              : `Move to ${version}`}
        </button>
      </div>
      {confirmAction && (
        <ConfirmActionModal action={confirmAction} onClose={() => setConfirmAction(null)} />
      )}
    </div>
  )
}
