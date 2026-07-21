import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { getContainers, postJSON, type Container } from '../api'
import {
  ConfirmActionModal,
  type ConfirmAction,
} from '../components/ConfirmActionModal'
import { friendlyName } from '../serviceNames'

type Filter = 'all' | 'running' | 'attention'

function imageTag(image: string): string {
  const i = image.lastIndexOf(':')
  if (i <= 0) return image
  return image.slice(i + 1)
}

function imageRepo(image: string): string {
  const i = image.lastIndexOf(':')
  return i > 0 ? image.slice(0, i) : image
}

function pillClass(c: Container): string {
  const h = (c.health || '').toLowerCase()
  const s = (c.state || '').toLowerCase()
  const st = (c.status || '').toLowerCase()
  if (h === 'unhealthy' || s === 'exited' || s === 'dead' || st.includes('unhealthy')) return 'warn'
  if (h === 'starting' || st === 'starting' || s === 'created' || s === 'restarting') return 'warn'
  if (h === 'healthy' || s === 'running' || st === 'healthy' || st === 'running') return 'ok'
  return 'warn'
}

function needsAttention(c: Container): boolean {
  const h = (c.health || '').toLowerCase()
  const s = (c.state || '').toLowerCase()
  if (s !== 'running') return true
  if (h === 'unhealthy' || h === 'starting') return true
  const st = (c.status || '').toLowerCase()
  return st === 'starting' || st.includes('unhealthy')
}

function isRunning(c: Container): boolean {
  return (c.state || '').toLowerCase() === 'running'
}

export function ContainersPage() {
  const [list, setList] = useState<Container[]>([])
  const [error, setError] = useState('')
  const [msg, setMsg] = useState('')
  const [busy, setBusy] = useState('')
  const [loading, setLoading] = useState(true)
  const [filter, setFilter] = useState<Filter>('all')
  const [query, setQuery] = useState('')
  const [confirmAction, setConfirmAction] = useState<ConfirmAction | null>(null)

  const refresh = useCallback(async () => {
    setError('')
    try {
      const r = await getContainers()
      setList(r.containers || [])
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
      setList([])
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void refresh()
  }, [refresh])

  const counts = useMemo(() => {
    const running = list.filter(isRunning).length
    const attention = list.filter(needsAttention).length
    return { total: list.length, running, attention }
  }, [list])

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    return list.filter((c) => {
      if (filter === 'running' && !isRunning(c)) return false
      if (filter === 'attention' && !needsAttention(c)) return false
      if (!q) return true
      const hay =
        `${c.service} ${friendlyName(c.service)} ${c.image} ${c.ports || ''}`.toLowerCase()
      return hay.includes(q)
    })
  }, [list, filter, query])

  async function restart(service: string) {
    setBusy(service)
    setError('')
    setMsg('')
    try {
      await postJSON(`/api/v1/containers/${encodeURIComponent(service)}/restart`)
      setMsg(`Restarted ${friendlyName(service)}.`)
      await refresh()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy('')
    }
  }

  return (
    <div className="stack">
      <div className="page-head page-head-row">
        <div>
          <h1>Services</h1>
          <p className="lead">
            Each part of your lab runs as a service. Restart one if it looks stuck, or open its logs
            for details.
          </p>
        </div>
        <div className="row page-actions">
          <button type="button" disabled={loading || !!busy} onClick={() => void refresh()}>
            {loading ? 'Loading…' : 'Refresh list'}
          </button>
          <Link className="btn" to="/logs">
            Open logs
          </Link>
        </div>
      </div>

      {error && <div className="banner error">{error}</div>}
      {msg && <div className="banner ok">{msg}</div>}

      {!loading && list.length === 0 && !error && (
        <div className="banner info">
          No services yet. Start the lab from <Link to="/">Home</Link>, or install one under{' '}
          <Link to="/setup">Get started</Link>.
        </div>
      )}

      {list.length > 0 && (
        <>
          <div className="metric-strip" role="list">
            <div className="metric" role="listitem">
              <div className="metric-label">Total</div>
              <div className="metric-value">{counts.total}</div>
              <div className="metric-meta">services in this lab</div>
            </div>
            <div className="metric" role="listitem">
              <div className="metric-label">Running</div>
              <div className="metric-value">{counts.running}</div>
              <div className="metric-meta">up right now</div>
            </div>
            <div className="metric" role="listitem">
              <div className="metric-label">Needs attention</div>
              <div className="metric-value">{counts.attention}</div>
              <div className="metric-meta">starting, stopped, or unhealthy</div>
            </div>
          </div>

          <div className="services-toolbar">
            <div className="filter-chips" role="group" aria-label="Filter services">
              <button
                type="button"
                className={`chip${filter === 'all' ? ' active' : ''}`}
                onClick={() => setFilter('all')}
              >
                All ({counts.total})
              </button>
              <button
                type="button"
                className={`chip${filter === 'running' ? ' active' : ''}`}
                onClick={() => setFilter('running')}
              >
                Running ({counts.running})
              </button>
              <button
                type="button"
                className={`chip${filter === 'attention' ? ' active' : ''}`}
                onClick={() => setFilter('attention')}
              >
                Needs attention ({counts.attention})
              </button>
            </div>
            <label className="field services-search">
              <span className="sr-only">Search services</span>
              <input
                name="service-search"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="Search by name…"
                autoComplete="off"
              />
            </label>
          </div>

          <div className="card services-card">
            <div className="services-list">
              {filtered.map((c) => (
                <article className="service-row" key={c.service + c.name}>
                  <div className="service-main">
                    <div className="service-title-row">
                      <h3 className="service-title">{friendlyName(c.service)}</h3>
                      <span className={`pill ${pillClass(c)}`}>{c.status || c.state}</span>
                    </div>
                    <p className="service-id" title={c.image}>
                      <code>{c.service}</code>
                      <span className="service-sep">·</span>
                      <span title={c.image}>
                        {imageRepo(c.image)}
                        <span className="service-tag">:{imageTag(c.image)}</span>
                      </span>
                    </p>
                    <div className="service-meta-row">
                      {c.uptime && <span className="hint">Started {c.uptime}</span>}
                      <span className="hint service-ports" title={c.ports || undefined}>
                        {c.ports || 'No published ports'}
                      </span>
                    </div>
                  </div>
                  <div className="service-actions">
                    <button
                      type="button"
                      disabled={!!busy}
                      onClick={() =>
                        setConfirmAction({
                          title: 'Restart service',
                          message: `Restart ${friendlyName(c.service)}. The service will be briefly unavailable.`,
                          confirmLabel: 'Restart service',
                          run: async () => {
                            await restart(c.service)
                          },
                        })
                      }
                    >
                      {busy === c.service ? 'Restarting…' : 'Restart'}
                    </button>
                    <Link className="btn" to={`/logs?service=${encodeURIComponent(c.service)}`}>
                      View logs
                    </Link>
                  </div>
                </article>
              ))}
              {filtered.length === 0 && (
                <p className="hint services-empty">No services match this filter.</p>
              )}
            </div>
          </div>
        </>
      )}
      {confirmAction && (
        <ConfirmActionModal action={confirmAction} onClose={() => setConfirmAction(null)} />
      )}
    </div>
  )
}
