import { useCallback, useEffect, useState } from "react";
import { getAIConfig, getAIStatus, postJSON } from "../api";

export function AIPage() {
  const [status, setStatus] = useState<Awaited<ReturnType<typeof getAIStatus>> | null>(null);
  const [config, setConfig] = useState("");
  const [openaiKey, setOpenaiKey] = useState("");
  const [anthropicKey, setAnthropicKey] = useState("");
  const [openaiBase, setOpenaiBase] = useState("");
  const [error, setError] = useState("");
  const [msg, setMsg] = useState("");
  const [busy, setBusy] = useState(false);

  const refresh = useCallback(async () => {
    setStatus(await getAIStatus());
  }, []);

  useEffect(() => {
    void refresh().catch((e) => setError(e instanceof Error ? e.message : String(e)));
  }, [refresh]);

  async function enable() {
    setBusy(true);
    setError("");
    setMsg("");
    try {
      await postJSON("/api/v1/ai/enable", { openaiKey, anthropicKey, openaiBaseUrl: openaiBase });
      setMsg("AI helpers are on. You can connect tools like Cursor next.");
      setOpenaiKey("");
      setAnthropicKey("");
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }

  async function disable(wipe: boolean) {
    setBusy(true);
    setError("");
    try {
      await postJSON("/api/v1/ai/disable", { wipeSecrets: wipe });
      setMsg(wipe ? "AI helpers off and keys removed." : "AI helpers turned off.");
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }

  async function loadConfig() {
    setError("");
    try {
      const r = await getAIConfig();
      setConfig(r.config);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }

  return (
    <div className="stack">
      <div className="page-head">
        <h1>AI helpers</h1>
        <p className="lead">
          Connect AI tools (such as Cursor) to your lab. You need an API key from OpenAI or Anthropic — no local AI model
          required.
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
            {!status.supported && <span className="hint">{status.supportError}</span>}
          </div>
          <div className="kv-list">
            <div className="kv">
              <span className="kv-label">OpenAI</span>
              <code className="kv-value">{status.openaiKey}</code>
            </div>
            <div className="kv">
              <span className="kv-label">Anthropic</span>
              <code className="kv-value">{status.anthropicKey}</code>
            </div>
            <div className="kv">
              <span className="kv-label">Custom URL</span>
              <code className="kv-value">{status.openaiBaseUrl || "(not set)"}</code>
            </div>
          </div>
        </div>
      )}
      <div className="card stack">
        <div className="section-title">Your keys</div>
        <label className="field">
          OpenAI API key
          <input type="password" value={openaiKey} onChange={(e) => setOpenaiKey(e.target.value)} autoComplete="off" />
        </label>
        <label className="field">
          Anthropic API key
          <input type="password" value={anthropicKey} onChange={(e) => setAnthropicKey(e.target.value)} autoComplete="off" />
        </label>
        <label className="field">
          Custom OpenAI-compatible URL (optional)
          <input value={openaiBase} onChange={(e) => setOpenaiBase(e.target.value)} />
        </label>
        <div className="row">
          <button type="button" className="primary" disabled={busy} onClick={() => void enable()}>
            Turn on
          </button>
          <button type="button" disabled={busy} onClick={() => void disable(false)}>
            Turn off
          </button>
          <button type="button" className="danger" disabled={busy} onClick={() => void disable(true)}>
            Turn off and remove keys
          </button>
          <button type="button" disabled={busy} onClick={() => void loadConfig()}>
            Show connection settings
          </button>
        </div>
        {config && (
          <>
            <pre className="code">{config}</pre>
            <button
              type="button"
              onClick={() => {
                void navigator.clipboard.writeText(config);
                setMsg("Connection settings copied.");
              }}
            >
              Copy settings
            </button>
          </>
        )}
      </div>
    </div>
  );
}
