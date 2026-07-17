import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import {
  getDoctor,
  getOverview,
  getSmoke,
  getUpdate,
  postJSON,
  postUpdate,
  ApiError,
  type Overview,
  type UpdateInfo,
} from "../api";
import { LabErrorBanner } from "../components/LabErrorBanner";
import { PROJECT } from "../project";

function pathTail(p?: string): string {
  if (!p) return "";
  const parts = p.split(/[/\\]/);
  return parts.slice(-3).join("/");
}

export function OverviewPage() {
  const [data, setData] = useState<Overview | null>(null);
  const [update, setUpdate] = useState<UpdateInfo | null>(null);
  const [error, setError] = useState<ApiError | null>(null);
  const [lastAction, setLastAction] = useState<{ label: string; path: string } | null>(null);
  const [msg, setMsg] = useState("");
  const [busy, setBusy] = useState("");
  const [doctor, setDoctor] = useState("");
  const [smoke, setSmoke] = useState("");
  const [updateOut, setUpdateOut] = useState("");

  const refresh = useCallback(async () => {
    setError(null);
    try {
      const [o, u] = await Promise.all([getOverview(), getUpdate()]);
      setData(o);
      setUpdate(u);
    } catch (e) {
      setError(e instanceof ApiError ? e : new ApiError(e instanceof Error ? e.message : String(e)));
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  async function run(label: string, path: string) {
    setLastAction({ label, path });
    setBusy(label);
    setError(null);
    setMsg("");
    try {
      await postJSON(path);
      await refresh();
    } catch (e) {
      setError(e instanceof ApiError ? e : new ApiError(e instanceof Error ? e.message : String(e)));
    } finally {
      setBusy("");
    }
  }

  async function recoverAndRetry() {
    if (!lastAction) return;
    setBusy("recover");
    setError(null);
    try {
      await postJSON("/api/v1/recover");
      await postJSON(lastAction.path);
      await refresh();
    } catch (e) {
      setError(e instanceof ApiError ? e : new ApiError(e instanceof Error ? e.message : String(e)));
    } finally {
      setBusy("");
    }
  }

  async function applyUpdate() {
    setBusy("update");
    setError(null);
    setMsg("");
    setUpdateOut("");
    try {
      const r = await postUpdate();
      setUpdateOut(r.output || "");
      if (!r.ok) setError(new ApiError(r.error || "Update failed"));
      else setMsg(r.restartHint || "Update finished. Close this window and open Camunda Lab again.");
      await refresh();
    } catch (e) {
      setError(e instanceof ApiError ? e : new ApiError(e instanceof Error ? e.message : String(e)));
    } finally {
      setBusy("");
    }
  }

  if (!data && !error) {
    return <p className="lead">Loading…</p>;
  }

  const labReady = Boolean(data?.configured);
  const channelLabel =
    update?.channel === "homebrew"
      ? "Installed with Homebrew"
      : update?.channel === "release"
        ? "Installed from a release"
        : "Development build";
  const cliVer = String(data?.cliVersion || "").replace(/^v/, "");

  return (
    <div className="stack overview">
      <div className="page-head page-head-row">
        <div>
          <h1>Home</h1>
          <p className="lead">
            Your local Camunda practice environment. Start it here, then open the apps when everything is ready.
          </p>
        </div>
        <div className="row page-actions">
          <button type="button" disabled={!!busy} onClick={() => void refresh()}>
            Refresh status
          </button>
          <a className="btn" href={PROJECT.docs} target="_blank" rel="noreferrer">
            Help docs
          </a>
          <a className="btn" href={PROJECT.camundaDocs} target="_blank" rel="noreferrer">
            Camunda Docs
          </a>
        </div>
      </div>

      {error && (
        <LabErrorBanner
          error={error}
          busy={busy === "recover"}
          onRecover={error.recoverable ? () => void recoverAndRetry() : undefined}
        />
      )}
      {msg && <div className="banner ok">{msg}</div>}

      {data && !labReady && (
        <div className="banner info">
          Nothing is installed yet. Go to <Link to="/setup">Get started</Link> to set up Camunda, or read the{" "}
          <a href={PROJECT.docs} target="_blank" rel="noreferrer">
            help docs
          </a>
          .
        </div>
      )}

      {data && (
        <>
          <div className="metric-strip" role="list">
            <div className="metric" role="listitem">
              <div className="metric-label">Your lab</div>
              <div className="metric-value">
                {labReady ? `${data.config.version} · ${data.config.profile}` : "—"}
              </div>
              <div className="metric-meta">
                {labReady ? `${data.config.resources} size` : "not set up yet"}
                {data.config.aiEnabled ? " · AI helpers on" : ""}
              </div>
            </div>
            <div className="metric" role="listitem">
              <div className="metric-label">Services</div>
              <div className="metric-value">
                {labReady
                  ? data.containersError
                    ? "Error"
                    : `${data.running ?? 0} / ${data.total ?? 0}`
                  : "—"}
              </div>
              <div className="metric-meta">{labReady ? "running now" : "set up a lab first"}</div>
            </div>
            <div className="metric" role="listitem">
              <div className="metric-label">App version</div>
              <div className="metric-value">v{cliVer}</div>
              <div className="metric-meta">
                {update?.latest ? `newest ${update.latest}` : channelLabel}
                {update?.updateAvailable ? " · update ready" : ""}
              </div>
            </div>
          </div>

          <div className="panel-grid">
            <section className="card panel">
              <header className="panel-head">
                <h2>Start & stop</h2>
                {labReady ? <span className="pill ok">Ready</span> : <span className="pill warn">Needs setup</span>}
              </header>
              <p className="hint">
                {labReady
                  ? data.containersError ||
                    `${data.running ?? 0} services running · saved under ${data.labHome}`
                  : "Use Get started to install Camunda on this computer."}
              </p>
              <div className="panel-actions">
                {labReady ? (
                  <>
                    <button type="button" className="primary" disabled={!!busy} onClick={() => void run("up", "/api/v1/up")}>
                      {busy === "up" ? "Starting…" : "Start lab"}
                    </button>
                    <button type="button" disabled={!!busy} onClick={() => void run("down", "/api/v1/down")}>
                      {busy === "down" ? "Stopping…" : "Stop lab"}
                    </button>
                    <button type="button" disabled={!!busy} onClick={() => void run("restart", "/api/v1/restart")}>
                      {busy === "restart" ? "Restarting…" : "Restart lab"}
                    </button>
                    <Link className="btn" to="/apps">
                      Open apps
                    </Link>
                  </>
                ) : (
                  <Link className="btn primary" to="/setup">
                    Get started
                  </Link>
                )}
              </div>
            </section>

            <section className="card panel">
              <header className="panel-head">
                <h2>Software updates</h2>
                <span className="pill">{channelLabel}</span>
              </header>
              <div className="version-compare">
                <div>
                  <span className="metric-label">On this computer</span>
                  <strong>v{cliVer}</strong>
                </div>
                <div className="version-arrow" aria-hidden>
                  →
                </div>
                <div>
                  <span className="metric-label">Newest release</span>
                  <strong>{update?.latest || "—"}</strong>
                </div>
              </div>
              <p className="hint path-hint" title={update?.executable}>
                {update?.channel === "dev"
                  ? "This is a development build. Install a published release to enable one-click updates."
                  : update?.updateAvailable
                    ? `Version ${update.latest} is available${update.publishedAt ? ` (${new Date(update.publishedAt).toLocaleDateString()})` : ""}.`
                    : update?.latest
                      ? "You already have the newest release."
                      : update?.error
                        ? `Could not check for updates: ${update.error}`
                        : "Checking for updates…"}
                {update?.executable ? ` · ${pathTail(update.executable)}` : ""}
              </p>
              <div className="panel-actions">
                <button
                  type="button"
                  className="primary"
                  disabled={!!busy || !update?.updateAvailable}
                  onClick={() => void applyUpdate()}
                >
                  {busy === "update"
                    ? "Updating…"
                    : update?.channel === "homebrew"
                      ? "Update with Homebrew"
                      : "Update now"}
                </button>
                <a className="btn" href={update?.releaseURL || PROJECT.releases} target="_blank" rel="noreferrer">
                  What’s new
                </a>
              </div>
              {updateOut && <pre className="code">{updateOut}</pre>}
            </section>
          </div>

          <section className="card panel">
            <header className="panel-head">
              <h2>Health checks</h2>
            </header>
            <p className="hint">Optional checks to see if your lab and its apps look healthy.</p>
            <div className="panel-actions">
              <button
                type="button"
                disabled={!!busy || !labReady}
                onClick={async () => {
                  setBusy("doctor");
                  try {
                    setDoctor((await getDoctor()).report);
                  } catch (e) {
                    setError(e instanceof ApiError ? e : new ApiError(e instanceof Error ? e.message : String(e)));
                  } finally {
                    setBusy("");
                  }
                }}
              >
                {busy === "doctor" ? "Checking…" : "Check environment"}
              </button>
              <button
                type="button"
                disabled={!!busy || !labReady}
                onClick={async () => {
                  setBusy("smoke");
                  try {
                    const r = await getSmoke();
                    setSmoke(r.Checks.map((c) => `${c.OK ? "✓" : "✗"} ${c.Name} ${c.Detail || c.URL}`).join("\n"));
                  } catch (e) {
                    setError(e instanceof ApiError ? e : new ApiError(e instanceof Error ? e.message : String(e)));
                  } finally {
                    setBusy("");
                  }
                }}
              >
                {busy === "smoke" ? "Testing…" : "Test apps"}
              </button>
            </div>
            {doctor && <pre className="code">{doctor}</pre>}
            {smoke && <pre className="code">{smoke}</pre>}
          </section>

          <div className="link-bar">
            <a href={PROJECT.repo} target="_blank" rel="noreferrer">
              Source on GitHub
            </a>
            <span className="link-bar-sep" aria-hidden>
              ·
            </span>
            <a href={PROJECT.docs} target="_blank" rel="noreferrer">
              Help docs
            </a>
            <span className="link-bar-sep" aria-hidden>
              ·
            </span>
            <a href={PROJECT.camundaDocs} target="_blank" rel="noreferrer">
              Camunda Docs
            </a>
            <span className="link-bar-sep" aria-hidden>
              ·
            </span>
            <a href={PROJECT.releases} target="_blank" rel="noreferrer">
              Releases
            </a>
            <span className="link-bar-sep" aria-hidden>
              ·
            </span>
            <a href={PROJECT.authorURL} target="_blank" rel="noreferrer">
              {PROJECT.author}
            </a>
          </div>
        </>
      )}
    </div>
  );
}
