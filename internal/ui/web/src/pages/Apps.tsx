import { useEffect, useMemo, useState } from "react";
import { getURLs } from "../api";
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

export function AppsPage() {
  const [urls, setUrls] = useState<Entry[]>([]);
  const [error, setError] = useState("");
  const [showUrls, setShowUrls] = useState(false);

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

  return (
    <div className="stack">
      <div className="page-head page-head-row">
        <div>
          <h1>Apps</h1>
          <p className="lead">Click a card to open that Camunda screen in a new browser tab.</p>
        </div>
        <div className="row page-actions">
          <button type="button" onClick={() => setShowUrls(true)} disabled={urls.length === 0}>
            Show all addresses
          </button>
        </div>
      </div>

      {error && <div className="banner error">{error}</div>}
      {!error && urls.length === 0 && (
        <div className="banner info">
          No apps yet — install a lab from <strong>Get started</strong> first.
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
                  href={u.url}
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
