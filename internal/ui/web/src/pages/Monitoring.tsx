import { useCallback, useEffect, useState } from "react";
import { getMonitoringStatus, postJSON } from "../api";

export function MonitoringPage() {
  const [status, setStatus] = useState<Awaited<ReturnType<typeof getMonitoringStatus>> | null>(null);
  const [error, setError] = useState("");
  const [msg, setMsg] = useState("");
  const [busy, setBusy] = useState(false);

  const refresh = useCallback(async () => {
    setStatus(await getMonitoringStatus());
  }, []);

  useEffect(() => {
    void refresh().catch((e) => setError(e instanceof Error ? e.message : String(e)));
  }, [refresh]);

  async function enable() {
    setBusy(true);
    setError("");
    setMsg("");
    try {
      await postJSON("/api/v1/monitoring/enable", {});
      setMsg("Monitoring is on. Grafana may take a few seconds to come up.");
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }

  async function disable() {
    setBusy(true);
    setError("");
    setMsg("");
    try {
      await postJSON("/api/v1/monitoring/disable", {});
      setMsg("Monitoring turned off and its containers removed.");
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="stack">
      <div className="page-head">
        <h1>Monitoring</h1>
        <p className="lead">
          Turn on Prometheus + Grafana with dashboards for Zeebe, Elasticsearch, and connectors already wired up. Local
          lab only — Grafana signs in with <code>admin</code> / <code>admin</code>.
        </p>
      </div>
      {error && <div className="banner error">{error}</div>}
      {msg && <div className="banner ok">{msg}</div>}
      {status && (
        <div className="card stack">
          <div className="section-title">Status</div>
          <div className="row">
            <span className={`pill ${status.enabled ? "ok" : "warn"}`}>
              {status.enabled ? "turned on" : "turned off"}
            </span>
          </div>
          <div className="kv-list">
            <div className="kv">
              <span className="kv-label">Grafana</span>
              {status.enabled ? (
                <a className="kv-value" href={status.grafana} target="_blank" rel="noreferrer">
                  {status.grafana}
                </a>
              ) : (
                <code className="kv-value">{status.grafana}</code>
              )}
            </div>
            <div className="kv">
              <span className="kv-label">Prometheus</span>
              {status.enabled ? (
                <a className="kv-value" href={status.prometheus} target="_blank" rel="noreferrer">
                  {status.prometheus}
                </a>
              ) : (
                <code className="kv-value">{status.prometheus}</code>
              )}
            </div>
          </div>
        </div>
      )}
      <div className="card stack">
        <div className="section-title">Controls</div>
        <p className="hint">
          Dashboards are best-effort per Camunda minor — some panels stay empty if a metric isn’t exposed on your
          version. Edit <code>~/.camunda-lab/overlays/monitoring/prometheus.yml</code> to tweak scrape targets.
        </p>
        <div className="row">
          <button type="button" className="primary" disabled={busy || status?.enabled} onClick={() => void enable()}>
            Turn on
          </button>
          <button type="button" disabled={busy || !status?.enabled} onClick={() => void disable()}>
            Turn off
          </button>
        </div>
      </div>
    </div>
  );
}
