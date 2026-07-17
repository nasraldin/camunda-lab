import { useEffect, useState } from "react";
import {
  ApiError,
  getIncidents,
  getTrace,
  toolkitJSON,
  type ToolkitResult,
} from "../api";
import { getProjectDir, setProjectDir } from "../projectDir";

type IncidentRow = {
  id?: string;
  ID?: string;
  error?: string;
  Error?: string;
  process?: string;
  Process?: string;
  jobWorker?: string;
  JobWorker?: string;
  key?: string;
  Key?: string;
};

export function ClusterPage() {
  const [projectPath, setProjectPath] = useState(getProjectDir());
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [output, setOutput] = useState("");
  const [cli, setCli] = useState("");
  const [incidents, setIncidents] = useState<IncidentRow[]>([]);
  const [instanceKey, setInstanceKey] = useState("");

  function saveDir() {
    setProjectDir(projectPath.trim());
  }

  async function run(label: string, fn: () => Promise<ToolkitResult>) {
    setBusy(label);
    setError("");
    setOutput("");
    try {
      const r = await fn();
      setOutput(r.output || (r.ok ? "OK" : r.error || "Done"));
      setCli(r.cli || "");
      if (!r.ok && r.error) setError(r.error);
      return r;
    } catch (e) {
      setError(e instanceof ApiError ? e.message : e instanceof Error ? e.message : String(e));
      return null;
    } finally {
      setBusy("");
    }
  }

  async function refreshIncidents() {
    const r = await run("incidents", () => getIncidents());
    if (r?.items) setIncidents(r.items as IncidentRow[]);
  }

  useEffect(() => {
    void refreshIncidents();
  }, []);

  return (
    <div className="stack">
      <div className="page-head">
        <h1>Cluster</h1>
        <p className="lead">Incidents, trace, plan, and drift against the live Orchestration API (same as CLI).</p>
      </div>

      <section className="card panel">
        <header className="panel-head">
          <h2>Project path</h2>
        </header>
        <p className="hint">Used by Plan / Drift. Must contain <code>.camunda.yaml</code> (see Project → Init).</p>
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
        </p>
        <div className="panel-actions">
          <button type="button" disabled={!!busy} onClick={() => void refreshIncidents()}>
            {busy === "incidents" ? "Loading…" : "Refresh"}
          </button>
        </div>
        {incidents.length === 0 ? (
          <p className="hint">No incidents.</p>
        ) : (
          <div className="url-list">
            {incidents.map((it) => {
              const id = it.id || it.ID || "";
              return (
                <div className="url-row" key={id}>
                  <div className="url-row-label">{id}</div>
                  <code className="url-row-value">
                    {(it.error || it.Error || "").slice(0, 120)} · {it.process || it.Process || ""}
                  </code>
                  <button
                    type="button"
                    className="btn-sm"
                    disabled={!!busy}
                    onClick={() => {
                      void (async () => {
                        await run("retry", () =>
                          toolkitJSON(`/api/v1/incidents/${encodeURIComponent(id)}/retry`, { confirm: true }),
                        );
                        await refreshIncidents();
                      })();
                    }}
                  >
                    Retry
                  </button>
                </div>
              );
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
        </p>
        <label className="field">
          <span>Process instance key</span>
          <input value={instanceKey} onChange={(e) => setInstanceKey(e.target.value)} placeholder="2251799813685249" />
        </label>
        <div className="panel-actions">
          <button
            type="button"
            className="primary"
            disabled={!!busy || !instanceKey.trim()}
            onClick={() => void run("trace", () => getTrace(instanceKey.trim()))}
          >
            {busy === "trace" ? "Loading…" : "Show timeline"}
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
              saveDir();
              void run("plan", () => toolkitJSON("/api/v1/plan", { dir: projectPath.trim() }));
            }}
          >
            {busy === "plan" ? "Planning…" : "Run plan"}
          </button>
          <button
            type="button"
            disabled={!!busy || !projectPath.trim()}
            onClick={() => {
              saveDir();
              void run("drift", () => toolkitJSON("/api/v1/drift", { dir: projectPath.trim() }));
            }}
          >
            {busy === "drift" ? "Checking…" : "Check drift"}
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
  );
}
