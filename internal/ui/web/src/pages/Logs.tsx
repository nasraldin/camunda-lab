import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { getContainers } from "../api";
import { friendlyName } from "../serviceNames";

type SearchMode = "filter" | "highlight";

const NEAR_BOTTOM_PX = 80;
const MAX_LINES = 2000;

function escapeRegExp(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function highlightLine(line: string, query: string): ReactNode {
  const q = query.trim();
  if (!q) return line;
  const re = new RegExp(`(${escapeRegExp(q)})`, "gi");
  const parts = line.split(re);
  if (parts.length === 1) return line;
  return parts.map((part, i) =>
    part.toLowerCase() === q.toLowerCase() ? (
      <mark className="log-hit" key={`${i}-${part}`}>
        {part}
      </mark>
    ) : (
      part
    ),
  );
}

function countMatches(lines: string[], query: string): number {
  const q = query.trim().toLowerCase();
  if (!q) return 0;
  let n = 0;
  for (const line of lines) {
    let from = 0;
    const lower = line.toLowerCase();
    while (from < lower.length) {
      const i = lower.indexOf(q, from);
      if (i < 0) break;
      n += 1;
      from = i + q.length;
    }
  }
  return n;
}

export function LogsPage() {
  const [params, setParams] = useSearchParams();
  const [services, setServices] = useState<string[]>([]);
  const [service, setService] = useState(params.get("service") || "");
  const [lines, setLines] = useState<string[]>([]);
  const [following, setFollowing] = useState(false);
  const [query, setQuery] = useState("");
  const [mode, setMode] = useState<SearchMode>("filter");
  const esRef = useRef<EventSource | null>(null);
  const preRef = useRef<HTMLPreElement | null>(null);
  const stickRef = useRef(true);

  useEffect(() => {
    void getContainers()
      .then((r) => {
        const names = (r.containers || []).map((c) => c.service);
        setServices(names);
        setService((prev) => prev || names[0] || "");
      })
      .catch(() => undefined);
  }, []);

  useEffect(() => {
    if (service) setParams({ service });
  }, [service, setParams]);

  useEffect(() => {
    return () => {
      esRef.current?.close();
    };
  }, []);

  function stop() {
    esRef.current?.close();
    esRef.current = null;
    setFollowing(false);
  }

  function clearScreen() {
    setLines([]);
  }

  function onServiceChange(next: string) {
    stop();
    clearScreen();
    setService(next);
  }

  function start(follow: boolean) {
    stop();
    if (!service) return;
    setLines([]);
    stickRef.current = true;
    setFollowing(follow);
    const es = new EventSource(`/api/v1/logs/${encodeURIComponent(service)}?follow=${follow ? "1" : "0"}`);
    esRef.current = es;
    es.onmessage = (ev) => {
      setLines((prev) => {
        const next = [...prev, ev.data];
        return next.length > MAX_LINES ? next.slice(-MAX_LINES) : next;
      });
      requestAnimationFrame(() => {
        const el = preRef.current;
        if (!el) return;
        if (follow ? stickRef.current : true) {
          el.scrollTop = el.scrollHeight;
        }
      });
    };
    es.addEventListener("error", () => {
      if (!follow) stop();
    });
    es.onerror = () => {
      if (!follow) stop();
    };
  }

  function onLogScroll() {
    const el = preRef.current;
    if (!el) return;
    stickRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < NEAR_BOTTOM_PX;
  }

  const q = query.trim();
  const filtered = useMemo(() => {
    if (!q || mode !== "filter") return lines;
    const lower = q.toLowerCase();
    return lines.filter((line) => line.toLowerCase().includes(lower));
  }, [lines, q, mode]);

  const matchCount = useMemo(() => countMatches(lines, q), [lines, q]);

  const displayLines = mode === "filter" ? filtered : lines;

  const meta = (() => {
    if (!lines.length) return "Nothing loaded yet — pick Show recent or Follow live.";
    if (!q) return `${lines.length} line${lines.length === 1 ? "" : "s"}`;
    if (mode === "filter") {
      return `Showing ${filtered.length} of ${lines.length} lines`;
    }
    return `${matchCount} match${matchCount === 1 ? "" : "es"} highlighted · ${lines.length} lines`;
  })();

  return (
    <div className="stack">
      <div className="page-head page-head-row">
        <div>
          <h1>Logs</h1>
          <p className="lead">
            Watch messages from one lab service. Search to filter or highlight lines when something fails.
          </p>
        </div>
        <div className="row page-actions">
          <Link className="btn" to="/containers">
            Back to Services
          </Link>
        </div>
      </div>

      <div className="card stack logs-card">
        <div className="logs-toolbar">
          <label className="field logs-service">
            Which service?
            <select value={service} onChange={(e) => onServiceChange(e.target.value)}>
              {services.length === 0 && <option value="">No services</option>}
              {services.map((s) => (
                <option key={s} value={s}>
                  {friendlyName(s)}
                </option>
              ))}
            </select>
          </label>

          <label className="field logs-search">
            <span className="sr-only">Find in logs</span>
            <input
              name="log-search"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Find in logs…"
              autoComplete="off"
            />
          </label>

          <div className="filter-chips" role="group" aria-label="Search mode">
            <button
              type="button"
              className={`chip${mode === "filter" ? " active" : ""}`}
              onClick={() => setMode("filter")}
            >
              Filter
            </button>
            <button
              type="button"
              className={`chip${mode === "highlight" ? " active" : ""}`}
              onClick={() => setMode("highlight")}
            >
              Highlight
            </button>
          </div>
        </div>

        <div className="row logs-actions">
          <button className="primary" type="button" disabled={!service || following} onClick={() => start(false)}>
            Show recent
          </button>
          <button type="button" disabled={!service || following} onClick={() => start(true)}>
            Follow live
          </button>
          <button type="button" disabled={!following} onClick={stop}>
            Stop following
          </button>
          <button type="button" onClick={clearScreen} disabled={lines.length === 0}>
            Clear screen
          </button>
          {following && <span className="pill ok">Live</span>}
        </div>

        <div className="log-meta" aria-live="polite">
          {meta}
        </div>

        <pre className="log log-viewer" ref={preRef} onScroll={onLogScroll}>
          {displayLines.length === 0 ? (
            <span className="log-empty">
              {lines.length === 0
                ? "Nothing to show yet."
                : q
                  ? "No lines match this search."
                  : "Nothing to show yet."}
            </span>
          ) : mode === "highlight" && q ? (
            displayLines.map((line, i) => (
              <span key={i} className="log-line">
                {highlightLine(line, q)}
                {"\n"}
              </span>
            ))
          ) : (
            displayLines.join("\n")
          )}
        </pre>
      </div>
    </div>
  );
}
