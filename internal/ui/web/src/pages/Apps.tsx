import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { getURLs } from "../api";
import { useAutoSso } from "../autoSso";
import { Modal } from "../components/Modal";
import { AppGlyph, appMeta } from "../icons";

type Entry = { name: string; url: string; notes?: string };

type Category = {
  id: string;
  title: string;
  hint: string;
  names: string[];
};

const CATEGORIES: Category[] = [
  {
    id: "core",
    title: "Everyday apps",
    hint: "Where you run processes and tasks day to day",
    names: ["operate", "tasklist", "admin", "console", "web-modeler"],
  },
  {
    id: "platform",
    title: "Accounts & analytics",
    hint: "Sign-in, users, and process insights",
    names: ["identity", "keycloak", "optimize"],
  },
  {
    id: "data",
    title: "Data & connections",
    hint: "Search, data browser, and connectors",
    names: ["elasticsearch", "elasticvue", "connectors"],
  },
  {
    id: "apis",
    title: "Developer links",
    hint: "APIs and AI agent endpoints",
    names: ["orchestration", "rest", "zeebe-http", "grpc", "mcp-cluster", "mcp-processes"],
  },
];

function isCredentialNote(notes?: string): boolean {
  if (!notes) return false;
  const n = notes.toLowerCase();
  return n.includes("demo/") || n.includes("admin/") || n.includes("password");
}

/** Apps that use Keycloak — open via Lab SSO warm so cookies land on localhost. */
const SSO_APPS = new Set([
  "operate",
  "tasklist",
  "admin",
  "console",
  "identity",
  "optimize",
  "web-modeler",
  "mcp-cluster",
  "mcp-processes",
]);

function appHref(u: Entry, autoSso: boolean): string {
  if (!u.url.startsWith("http")) return u.url;
  if (!autoSso) return u.url;
  if (!SSO_APPS.has(u.name) && !isCredentialNote(u.notes)) return u.url;
  return `/api/v1/sso/open?url=${encodeURIComponent(u.url)}`;
}

/** Keycloak base is typically http://localhost:18080/auth/ */
export function keycloakLogoutURL(keycloakURL?: string): string | null {
  if (!keycloakURL) return null;
  try {
    const u = new URL(keycloakURL);
    const path = u.pathname.replace(/\/+$/, "") || "/auth";
    const authBase = path.toLowerCase().includes("/auth") ? path : `${path}/auth`;
    return `${u.origin}${authBase}/realms/camunda-platform/protocol/openid-connect/logout`;
  } catch {
    return null;
  }
}

export function AppsPage() {
  const [urls, setUrls] = useState<Entry[]>([]);
  const [error, setError] = useState("");
  const [msg, setMsg] = useState("");
  const [showUrls, setShowUrls] = useState(false);
  const [autoSso, setAutoSso] = useAutoSso();

  useEffect(() => {
    void getURLs()
      .then((r) => {
        setUrls(
          (r.urls || []).map((u) => ({
            name: u.name || u.Name || "",
            url: u.url || u.URL || "",
            notes: u.notes || u.Notes,
          })),
        );
      })
      .catch((e) => setError(e instanceof Error ? e.message : String(e)));
  }, []);

  const byName = useMemo(() => {
    const m = new Map<string, Entry>();
    for (const u of urls) m.set(u.name, u);
    return m;
  }, [urls]);

  const logoutURL = useMemo(() => keycloakLogoutURL(byName.get("keycloak")?.url), [byName]);

  const categorized = useMemo(() => {
    const used = new Set<string>();
    const sections = CATEGORIES.map((cat) => {
      const items = cat.names.map((n) => byName.get(n)).filter(Boolean) as Entry[];
      items.forEach((i) => used.add(i.name));
      return { ...cat, items };
    }).filter((s) => s.items.length > 0);

    const other = urls.filter((u) => !used.has(u.name));
    if (other.length) {
      sections.push({
        id: "other",
        title: "Other",
        hint: "More links for this lab setup",
        names: other.map((o) => o.name),
        items: other,
      });
    }
    return sections;
  }, [byName, urls]);

  function openLogout(kind: "signout" | "fix" | "optout") {
    if (!logoutURL) {
      setMsg("");
      setError("No Keycloak in this lab profile — there is no shared Camunda sign-in session to clear.");
      return;
    }
    setError("");
    window.open(logoutURL, "_blank", "noopener,noreferrer");
    if (kind === "signout") {
      setMsg("Opened Keycloak sign-out. Close that tab, then open an app again if you need to sign in fresh.");
    } else if (kind === "optout") {
      setMsg(
        "Auto sign-in is off. Cleared the Camunda session so apps will show the login page — close the sign-out tab, then open an app.",
      );
    } else {
      setMsg(
        "Opened Keycloak sign-out to clear a stuck session. Always open apps from this page (localhost links) — mixing 127.0.0.1 can cause login loops or odd 404s.",
      );
    }
  }

  function onAutoSsoChange(next: boolean) {
    setAutoSso(next);
    if (!next) {
      // Opt-out only skips Lab warm-up; leftover Keycloak cookies still SSO silently.
      openLogout("optout");
    } else {
      setMsg("Auto sign-in is on. Opening an app will warm the Keycloak session when possible.");
    }
  }

  return (
    <div className="stack">
      <div className="page-head page-head-row">
        <div>
          <h1>Apps</h1>
          <p className="lead">
            {autoSso
              ? "Click a card to open that Camunda screen. Lab signs you into Keycloak automatically when possible."
              : "Click a card to open that Camunda screen. Auto sign-in is off — you’ll use the app’s own login when needed."}
          </p>
        </div>
        <div className="row page-actions">
          <label
            className="pref-switch"
            title={
              autoSso
                ? "Lab warms Keycloak session (demo/demo) before opening apps"
                : "Open apps directly; turning this off also signs you out of Camunda"
            }
          >
            <input
              type="checkbox"
              role="switch"
              checked={autoSso}
              onChange={(e) => onAutoSsoChange(e.target.checked)}
              aria-checked={autoSso}
            />
            <span className="pref-switch-track" aria-hidden="true" />
            <span className="pref-switch-label">Auto sign-in</span>
          </label>
          <button type="button" onClick={() => openLogout("signout")} disabled={urls.length === 0}>
            Sign out of Camunda
          </button>
          <button type="button" onClick={() => openLogout("fix")} disabled={urls.length === 0}>
            Fix broken session
          </button>
          <button type="button" onClick={() => setShowUrls(true)} disabled={urls.length === 0}>
            Show all addresses
          </button>
        </div>
      </div>

      {error && <div className="banner error">{error}</div>}
      {msg && <div className="banner ok">{msg}</div>}
      {!error && urls.length === 0 && (
        <div className="banner info">
          No apps yet — install a lab from <strong>Get started</strong> first.
        </div>
      )}
      {urls.length > 0 && autoSso && (
        <div className="banner info">
          Auto sign-in needs Lab UI on <code>http://localhost:…</code> (default). Camunda apps share Keycloak cookies on{" "}
          <code>localhost</code>. Default user: <code>demo</code> / <code>demo</code> (see <Link to="/admin">Logins</Link>
          ). Turn off Auto sign-in above if you prefer to log in yourself.
        </div>
      )}
      {urls.length > 0 && !autoSso && (
        <div className="banner info">
          Auto sign-in is off (and your Camunda session is cleared when you switch it off). Open an app to see the login
          page — credentials on <Link to="/admin">Logins</Link>.
        </div>
      )}

      {categorized.map((section) => (
        <section key={section.id} className="app-section">
          <div className="app-section-head">
            <h2>{section.title}</h2>
            <p>{section.hint}</p>
          </div>
          <div className="grid grid-apps">
            {section.items.map((u) => {
              const meta = appMeta(u.name);
              const http = u.url.startsWith("http");
              const body = (
                <>
                  <AppGlyph name={u.name} />
                  <div className="app-card-copy">
                    <h3>{meta.label}</h3>
                    {u.notes && !isCredentialNote(u.notes) && <p className="app-card-note">{u.notes}</p>}
                    {!http && <p className="app-card-note">Not a website link — use Show all addresses</p>}
                  </div>
                </>
              );
              return http ? (
                <a
                  className="card app-card app-card-link"
                  key={u.name}
                  href={appHref(u, autoSso)}
                  target="_blank"
                  rel="noreferrer"
                >
                  {body}
                </a>
              ) : (
                <div className="card app-card app-card-static" key={u.name}>
                  {body}
                </div>
              );
            })}
          </div>
        </section>
      ))}

      {showUrls && (
        <Modal title="All addresses" onClose={() => setShowUrls(false)} wide>
          <p className="hint">Copy these if you need them for Desktop Modeler, clients, or AI tools.</p>
          <div className="url-list">
            {urls.map((u) => {
              const meta = appMeta(u.name);
              return (
                <div className="url-row" key={u.name}>
                  <div className="url-row-label">{meta.label}</div>
                  <code className="url-row-value">{u.url}</code>
                  <button type="button" className="btn-sm" onClick={() => void navigator.clipboard.writeText(u.url)}>
                    Copy
                  </button>
                </div>
              );
            })}
          </div>
        </Modal>
      )}
    </div>
  );
}
