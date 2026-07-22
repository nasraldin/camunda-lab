/** Nav + Camunda product icons for the Lab UI. */

import { useState, type ReactElement } from 'react'

type IconProps = { className?: string }

export function IconOverview(p: IconProps) {
  return (
    <svg className={p.className} viewBox="0 0 24 24" fill="none" aria-hidden>
      <path
        d="M4 11.5 12 4l8 7.5"
        stroke="currentColor"
        strokeWidth="1.75"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <path
        d="M6.5 10.5V20h4.2v-5.2h2.6V20h4.2v-9.5"
        stroke="currentColor"
        strokeWidth="1.75"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  )
}

export function IconSetup(p: IconProps) {
  return (
    <svg className={p.className} viewBox="0 0 24 24" fill="none" aria-hidden>
      <circle cx="12" cy="12" r="3.25" stroke="currentColor" strokeWidth="1.75" />
      <path
        d="M19.4 13a8 8 0 0 0 0-2l2.1-1.6-2-3.4-2.5 1a7.6 7.6 0 0 0-1.7-1L15 3h-4l-.3 2.9a7.6 7.6 0 0 0-1.7 1l-2.5-1-2 3.4L6.6 11a8 8 0 0 0 0 2l-2.1 1.6 2 3.4 2.5-1a7.6 7.6 0 0 0 1.7 1L11 21h4l.3-2.9a7.6 7.6 0 0 0 1.7-1l2.5 1 2-3.4L19.4 13Z"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinejoin="round"
      />
    </svg>
  )
}

export function IconApps(p: IconProps) {
  return (
    <svg className={p.className} viewBox="0 0 24 24" fill="none" aria-hidden>
      <rect x="3" y="3" width="7" height="7" rx="1.5" stroke="currentColor" strokeWidth="1.75" />
      <rect x="14" y="3" width="7" height="7" rx="1.5" stroke="currentColor" strokeWidth="1.75" />
      <rect x="3" y="14" width="7" height="7" rx="1.5" stroke="currentColor" strokeWidth="1.75" />
      <rect x="14" y="14" width="7" height="7" rx="1.5" stroke="currentColor" strokeWidth="1.75" />
    </svg>
  )
}

export function IconContainers(p: IconProps) {
  return (
    <svg className={p.className} viewBox="0 0 24 24" fill="none" aria-hidden>
      <path
        d="M12 3 3 7.5 12 12l9-4.5L12 3Z"
        stroke="currentColor"
        strokeWidth="1.75"
        strokeLinejoin="round"
      />
      <path
        d="M3 12.5 12 17l9-4.5M3 17.5 12 22l9-4.5"
        stroke="currentColor"
        strokeWidth="1.75"
        strokeLinejoin="round"
      />
    </svg>
  )
}

export function IconLogs(p: IconProps) {
  return (
    <svg className={p.className} viewBox="0 0 24 24" fill="none" aria-hidden>
      <path
        d="M6 5h12M6 10h12M6 15h8M6 20h5"
        stroke="currentColor"
        strokeWidth="1.75"
        strokeLinecap="round"
      />
    </svg>
  )
}

export function IconAI(p: IconProps) {
  return (
    <svg className={p.className} viewBox="0 0 24 24" fill="none" aria-hidden>
      <circle cx="12" cy="12" r="3.4" stroke="currentColor" strokeWidth="1.75" />
      <path
        d="M12 3v2.5M12 18.5V21M4.2 6.2l1.8 1.8M18 16l1.8 1.8M3 12h2.5M18.5 12H21M4.2 17.8 6 16M18 8l1.8-1.8"
        stroke="currentColor"
        strokeWidth="1.75"
        strokeLinecap="round"
      />
    </svg>
  )
}

export function IconMonitoring(p: IconProps) {
  return (
    <svg className={p.className} viewBox="0 0 24 24" fill="none" aria-hidden>
      <rect x="3" y="4" width="18" height="14" rx="1.5" stroke="currentColor" strokeWidth="1.75" />
      <path d="M6 13.5l2.5-3 2 2.2L14 8l4 5" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M9 21h6M12 18v3" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" />
    </svg>
  )
}

export function IconTools(p: IconProps) {
  return (
    <svg className={p.className} viewBox="0 0 24 24" fill="none" aria-hidden>
      <path
        d="m14.7 6.3 2.5-2.5 3 3-2.5 2.5M4 20l5-5"
        stroke="currentColor"
        strokeWidth="1.75"
        strokeLinecap="round"
      />
      <path
        d="M14.5 6.5a4.2 4.2 0 0 0-5.9 5.9L4 17l3 3 4.6-4.6a4.2 4.2 0 0 0 5.9-5.9"
        stroke="currentColor"
        strokeWidth="1.75"
      />
    </svg>
  )
}

export function IconAdmin(p: IconProps) {
  return (
    <svg className={p.className} viewBox="0 0 24 24" fill="none" aria-hidden>
      <circle cx="12" cy="8" r="3.2" stroke="currentColor" strokeWidth="1.75" />
      <path
        d="M5 19c1.5-3.2 3.8-4.8 7-4.8s5.5 1.6 7 4.8"
        stroke="currentColor"
        strokeWidth="1.75"
        strokeLinecap="round"
      />
    </svg>
  )
}

export function IconDanger(p: IconProps) {
  return (
    <svg className={p.className} viewBox="0 0 24 24" fill="none" aria-hidden>
      <path
        d="M12 3 2 21h20L12 3Z"
        stroke="currentColor"
        strokeWidth="1.75"
        strokeLinejoin="round"
      />
      <path d="M12 10v5M12 18h.01" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" />
    </svg>
  )
}

export function IconExternal(p: IconProps) {
  return (
    <svg className={p.className} viewBox="0 0 24 24" fill="none" aria-hidden>
      <path
        d="M14 5h5v5M19 5 10 14"
        stroke="currentColor"
        strokeWidth="1.75"
        strokeLinecap="round"
      />
      <path
        d="M11 5H6a2 2 0 0 0-2 2v11a2 2 0 0 0 2 2h11a2 2 0 0 0 2-2v-5"
        stroke="currentColor"
        strokeWidth="1.75"
      />
    </svg>
  )
}

export function IconGitHub(p: IconProps) {
  return (
    <svg className={p.className} viewBox="0 0 24 24" fill="currentColor" aria-hidden>
      <path d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.531 1.032 1.531 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0 1 12 6.844a9.59 9.59 0 0 1 2.504.337c1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.02 10.02 0 0 0 22 12.017C22 6.484 17.522 2 12 2Z" />
    </svg>
  )
}

export function IconDocs(p: IconProps) {
  return (
    <svg className={p.className} viewBox="0 0 24 24" fill="none" aria-hidden>
      <path
        d="M5 4.5A1.5 1.5 0 0 1 6.5 3H14l5 5v11.5a1.5 1.5 0 0 1-1.5 1.5h-11A1.5 1.5 0 0 1 5 19.5v-15Z"
        stroke="currentColor"
        strokeWidth="1.75"
        strokeLinejoin="round"
      />
      <path
        d="M14 3v5h5M8 12h8M8 16h6"
        stroke="currentColor"
        strokeWidth="1.75"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  )
}

/** Official product docs (open book). */
export function IconCamundaDocs(p: IconProps) {
  return (
    <svg className={p.className} viewBox="0 0 24 24" fill="none" aria-hidden>
      <path
        d="M4 5.5c0-.8.7-1.5 1.5-1.5H11v15.5H5.5A1.5 1.5 0 0 1 4 18V5.5Z"
        stroke="currentColor"
        strokeWidth="1.75"
        strokeLinejoin="round"
      />
      <path
        d="M20 5.5c0-.8-.7-1.5-1.5-1.5H13v15.5h5.5a1.5 1.5 0 0 0 1.5-1.5V5.5Z"
        stroke="currentColor"
        strokeWidth="1.75"
        strokeLinejoin="round"
      />
      <path d="M12 4v15.5" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" />
    </svg>
  )
}

export function IconReleases(p: IconProps) {
  return (
    <svg className={p.className} viewBox="0 0 24 24" fill="none" aria-hidden>
      <path
        d="M12 3v12M8 11l4 4 4-4"
        stroke="currentColor"
        strokeWidth="1.75"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <path d="M5 19h14" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" />
    </svg>
  )
}

type AppDef = {
  label: string
  color: string
  Icon: (p: IconProps) => ReactElement
}

function OperateIcon(p: IconProps) {
  return (
    <svg className={p.className} viewBox="0 0 24 24" fill="none" aria-hidden>
      <circle cx="12" cy="12" r="8" stroke="currentColor" strokeWidth="1.8" />
      <path d="M8 12h8M12 8v8" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" />
    </svg>
  )
}
function TasklistIcon(p: IconProps) {
  return (
    <svg className={p.className} viewBox="0 0 24 24" fill="none" aria-hidden>
      <rect x="5" y="4" width="14" height="16" rx="2" stroke="currentColor" strokeWidth="1.8" />
      <path
        d="M8 9h8M8 13h8M8 17h5"
        stroke="currentColor"
        strokeWidth="1.8"
        strokeLinecap="round"
      />
    </svg>
  )
}
function ConsoleIcon(p: IconProps) {
  return (
    <svg className={p.className} viewBox="0 0 24 24" fill="none" aria-hidden>
      <rect x="3" y="5" width="18" height="14" rx="2" stroke="currentColor" strokeWidth="1.8" />
      <path d="M7 10h4M7 14h10" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" />
    </svg>
  )
}
function OptimizeIcon(p: IconProps) {
  return (
    <svg className={p.className} viewBox="0 0 24 24" fill="none" aria-hidden>
      <path
        d="M4 18V8l5 4 4-7 7 11"
        stroke="currentColor"
        strokeWidth="1.8"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  )
}
function IdentityIcon(p: IconProps) {
  return (
    <svg className={p.className} viewBox="0 0 24 24" fill="none" aria-hidden>
      <circle cx="12" cy="9" r="3.2" stroke="currentColor" strokeWidth="1.8" />
      <path
        d="M5.5 19c1.6-3 4-4.5 6.5-4.5S17 16 18.5 19"
        stroke="currentColor"
        strokeWidth="1.8"
        strokeLinecap="round"
      />
    </svg>
  )
}
function ModelerIcon(p: IconProps) {
  return (
    <svg className={p.className} viewBox="0 0 24 24" fill="none" aria-hidden>
      <rect x="4" y="6" width="6" height="5" rx="1" stroke="currentColor" strokeWidth="1.8" />
      <rect x="14" y="13" width="6" height="5" rx="1" stroke="currentColor" strokeWidth="1.8" />
      <path
        d="M10 8.5h4M14 8.5v4.5"
        stroke="currentColor"
        strokeWidth="1.8"
        strokeLinecap="round"
      />
    </svg>
  )
}
function KeycloakIcon(p: IconProps) {
  return (
    <svg className={p.className} viewBox="0 0 24 24" fill="none" aria-hidden>
      <circle cx="9" cy="12" r="3.2" stroke="currentColor" strokeWidth="1.8" />
      <path
        d="M12.2 12H20v2.2h-2V17h-2.2v-2.8H12.2"
        stroke="currentColor"
        strokeWidth="1.8"
        strokeLinejoin="round"
      />
    </svg>
  )
}
function ConnectorsIcon(p: IconProps) {
  return (
    <svg className={p.className} viewBox="0 0 24 24" fill="none" aria-hidden>
      <path
        d="M8 8h3a3 3 0 0 1 0 6H8M16 8h-3a3 3 0 0 0 0 6h3"
        stroke="currentColor"
        strokeWidth="1.8"
        strokeLinecap="round"
      />
    </svg>
  )
}
function ElasticIcon(p: IconProps) {
  return (
    <svg className={p.className} viewBox="0 0 24 24" fill="none" aria-hidden>
      <ellipse cx="12" cy="7" rx="7" ry="2.6" stroke="currentColor" strokeWidth="1.7" />
      <path
        d="M5 7v5c0 1.4 3.1 2.6 7 2.6s7-1.2 7-2.6V7M5 12v5c0 1.4 3.1 2.6 7 2.6s7-1.2 7-2.6v-5"
        stroke="currentColor"
        strokeWidth="1.7"
      />
    </svg>
  )
}
function ApiIcon(p: IconProps) {
  return (
    <svg className={p.className} viewBox="0 0 24 24" fill="none" aria-hidden>
      <path
        d="M8 8 4 12l4 4M16 8l4 4-4 4M13 6l-2 12"
        stroke="currentColor"
        strokeWidth="1.8"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  )
}
function McpIcon(p: IconProps) {
  return (
    <svg className={p.className} viewBox="0 0 24 24" fill="none" aria-hidden>
      <rect x="4" y="4" width="7" height="7" rx="1.5" stroke="currentColor" strokeWidth="1.8" />
      <rect x="13" y="13" width="7" height="7" rx="1.5" stroke="currentColor" strokeWidth="1.8" />
      <path
        d="M11 7.5h2.5V11M13 16.5H10.5V13"
        stroke="currentColor"
        strokeWidth="1.8"
        strokeLinecap="round"
      />
    </svg>
  )
}
function AdminIcon(p: IconProps) {
  return (
    <svg className={p.className} viewBox="0 0 24 24" fill="none" aria-hidden>
      <path
        d="M12 3 4 6v5c0 4.5 3.2 8.4 8 9.5 4.8-1.1 8-5 8-9.5V6l-8-3Z"
        stroke="currentColor"
        strokeWidth="1.8"
        strokeLinejoin="round"
      />
    </svg>
  )
}
function DefaultIcon(p: IconProps) {
  return (
    <svg className={p.className} viewBox="0 0 24 24" fill="none" aria-hidden>
      <rect x="5" y="5" width="14" height="14" rx="3" stroke="currentColor" strokeWidth="1.8" />
    </svg>
  )
}

const APP_META: Record<string, AppDef> = {
  operate: { label: 'Operate', color: '#FC5D0D', Icon: OperateIcon },
  tasklist: { label: 'Tasklist', color: '#2685C7', Icon: TasklistIcon },
  admin: { label: 'Admin', color: '#5B6B7C', Icon: AdminIcon },
  console: { label: 'Console', color: '#FC5D0D', Icon: ConsoleIcon },
  optimize: { label: 'Optimize', color: '#26A69A', Icon: OptimizeIcon },
  identity: { label: 'Identity', color: '#6C5CE7', Icon: IdentityIcon },
  'web-modeler': { label: 'Web Modeler', color: '#E4572E', Icon: ModelerIcon },
  keycloak: { label: 'Keycloak', color: '#4D4D4D', Icon: KeycloakIcon },
  connectors: { label: 'Connectors', color: '#0D7377', Icon: ConnectorsIcon },
  elasticvue: { label: 'ElasticVue', color: '#D4A017', Icon: ElasticIcon },
  elasticsearch: { label: 'Elasticsearch', color: '#D4A017', Icon: ElasticIcon },
  rest: { label: 'REST API', color: '#2685C7', Icon: ApiIcon },
  orchestration: { label: 'Orchestration', color: '#FC5D0D', Icon: OperateIcon },
  grpc: { label: 'gRPC', color: '#2685C7', Icon: ApiIcon },
  'zeebe-http': { label: 'Zeebe HTTP', color: '#FC5D0D', Icon: ApiIcon },
  'mcp-cluster': { label: 'MCP Cluster', color: '#0B7285', Icon: McpIcon },
  'mcp-processes': { label: 'MCP Processes', color: '#0B7285', Icon: McpIcon },
  grafana: { label: 'Grafana', color: '#F46800', Icon: OptimizeIcon },
  prometheus: { label: 'Prometheus', color: '#E6522C', Icon: ApiIcon },
}

export function appMeta(name: string): AppDef {
  return (
    APP_META[name] || {
      label: name
        .split('-')
        .map((w) => w.charAt(0).toUpperCase() + w.slice(1))
        .join(' '),
      color: '#5B6B7C',
      Icon: DefaultIcon,
    }
  )
}

export function AppGlyph({ name, size = 44 }: { name: string; size?: number }) {
  const meta = appMeta(name)
  const src = `/icons/${name}.svg`
  const Icon = meta.Icon
  const [failed, setFailed] = useState(false)

  return (
    <span
      className="app-glyph"
      style={{ width: size, height: size, background: meta.color }}
      aria-hidden
    >
      {!failed ? (
        <img
          className="app-glyph-img"
          src={src}
          alt=""
          width={22}
          height={22}
          onError={() => setFailed(true)}
        />
      ) : (
        <Icon className="app-glyph-svg" />
      )}
    </span>
  )
}
