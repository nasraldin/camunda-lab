import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { getURLs } from "../api";
import { Modal } from "../components/Modal";

type Cred = {
  id: string;
  title: string;
  apps: string;
  username: string;
  password: string;
  openLabel?: string;
  openHref?: string;
};

const SEEDED: Cred[] = [
  {
    id: "apps",
    title: "Camunda apps",
    apps: "Operate, Tasklist, Console, Optimize, Web Modeler",
    username: "demo",
    password: "demo",
    openLabel: "Open user management",
  },
  {
    id: "keycloak",
    title: "Keycloak (full lab)",
    apps: "Sign-in server used by the full lab size",
    username: "admin",
    password: "admin",
    openLabel: "Open Keycloak",
  },
];

export function AdminPage() {
  const [keycloakURL, setKeycloakURL] = useState("");
  const [identityURL, setIdentityURL] = useState("");
  const [copied, setCopied] = useState("");
  const [showReset, setShowReset] = useState(false);

  useEffect(() => {
    void getURLs()
      .then((r) => {
        for (const u of r.urls || []) {
          const name = u.name || u.Name || "";
          const url = u.url || u.URL || "";
          if (name === "keycloak") setKeycloakURL(url);
          if (name === "identity") setIdentityURL(url);
        }
      })
      .catch(() => undefined);
  }, []);

  const creds: Cred[] = SEEDED.map((c) => {
    if (c.id === "keycloak" && keycloakURL) {
      return { ...c, openHref: keycloakURL };
    }
    if (c.id === "apps" && identityURL) {
      return { ...c, openHref: identityURL };
    }
    return c;
  });

  async function copy(text: string, id: string) {
    await navigator.clipboard.writeText(text);
    setCopied(id);
    setTimeout(() => setCopied(""), 1500);
  }

  return (
    <div className="stack">
      <div className="page-head">
        <h1>Logins</h1>
        <p className="lead">
          Default usernames and passwords for this local lab. After one Keycloak sign-in, Camunda apps usually stay open
          in this browser — manage that from <Link to="/apps">Apps</Link> (Sign out / Fix broken session).
        </p>
      </div>

      <div className="banner info">
        For practice on your computer only. Do not reuse <code>demo</code> / <code>admin</code> on real systems.
      </div>

      <div className="grid grid-creds">
        {creds.map((c) => (
          <div className="card stack" key={c.id}>
            <div className="section-title">{c.title}</div>
            <p className="hint">{c.apps}</p>
            <div className="kv-list">
              <div className="kv">
                <span className="kv-label">Username</span>
                <code className="kv-value">{c.username}</code>
                <button type="button" className="btn-sm" onClick={() => void copy(c.username, `${c.id}-u`)}>
                  {copied === `${c.id}-u` ? "Copied" : "Copy"}
                </button>
              </div>
              <div className="kv">
                <span className="kv-label">Password</span>
                <code className="kv-value">{c.password}</code>
                <button type="button" className="btn-sm" onClick={() => void copy(c.password, `${c.id}-p`)}>
                  {copied === `${c.id}-p` ? "Copied" : "Copy"}
                </button>
              </div>
            </div>
            {c.openHref && c.openLabel && (
              <a className="btn" href={c.openHref} target="_blank" rel="noreferrer">
                {c.openLabel}
              </a>
            )}
          </div>
        ))}
      </div>

      <div className="card stack">
        <div className="section-title">Get the defaults back</div>
        <p className="hint">
          Camunda Lab does not keep your passwords itself. The install creates <strong>demo / demo</strong> for apps and{" "}
          <strong>admin / admin</strong> for Keycloak on a full lab. If you changed them, reset the lab and install again.
        </p>
        <ol className="steps">
          <li>
            Prefer changing the password in{" "}
            {keycloakURL ? (
              <a href={keycloakURL} target="_blank" rel="noreferrer">
                Keycloak
              </a>
            ) : (
              "Keycloak"
            )}{" "}
            or user management when you only need a new password.
          </li>
          <li>To recreate the original demo users, delete the lab and install again from Get started.</li>
        </ol>
        <div className="row">
          <button type="button" className="primary" onClick={() => setShowReset(true)}>
            How to restore defaults…
          </button>
          <Link className="btn" to="/danger">
            Reset lab
          </Link>
          <Link className="btn" to="/setup">
            Get started
          </Link>
        </div>
      </div>

      {showReset && (
        <Modal title="Restore default logins" onClose={() => setShowReset(false)}>
          <p className="hint">
            This deletes lab data (including users). Process data in the lab will be lost.
          </p>
          <ol className="steps">
            <li>
              Open <Link to="/danger">Reset lab</Link> and delete everything (type DELETE), or in a terminal run{" "}
              <code>camunda nuke --yes</code>
            </li>
            <li>
              Install again from <Link to="/setup">Get started</Link>.
            </li>
            <li>
              Sign in with <code>demo / demo</code> (and Keycloak <code>admin / admin</code> on a full lab).
            </li>
          </ol>
          <div className="row modal-actions">
            <Link className="btn primary" to="/danger" onClick={() => setShowReset(false)}>
              Go to Reset lab
            </Link>
            <button type="button" onClick={() => setShowReset(false)}>
              Cancel
            </button>
          </div>
        </Modal>
      )}
    </div>
  );
}
