import { NavLink, Route, Routes } from 'react-router-dom'
import { useEffect, useState, type ReactElement } from 'react'
import { OverviewPage } from './pages/Overview'
import { SetupPage } from './pages/Setup'
import { AppsPage } from './pages/Apps'
import { AdminPage } from './pages/Admin'
import { ContainersPage } from './pages/Containers'
import { LogsPage } from './pages/Logs'
import { AIPage } from './pages/AI'
import { MonitoringPage } from './pages/Monitoring'
import { ToolsPage } from './pages/Tools'
import { DangerPage } from './pages/Danger'
import { BpmnPage } from './pages/Bpmn'
import { ClusterPage } from './pages/Cluster'
import { ProjectPage } from './pages/Project'
import {
  IconAI,
  IconAdmin,
  IconApps,
  IconCamundaDocs,
  IconContainers,
  IconDanger,
  IconDocs,
  IconGitHub,
  IconLogs,
  IconMonitoring,
  IconOverview,
  IconReleases,
  IconSetup,
  IconTools,
} from './icons'
import { getOverview } from './api'
import { PROJECT } from './project'
import { useTheme } from './theme'

function IconBpmn(p: { className?: string }) {
  return (
    <svg className={p.className} viewBox="0 0 24 24" fill="none" aria-hidden>
      <circle cx="6" cy="12" r="2.5" stroke="currentColor" strokeWidth="1.75" />
      <rect x="10" y="9.5" width="5" height="5" rx="1" stroke="currentColor" strokeWidth="1.75" />
      <path
        d="M15 12h3.5M18.5 12l-1.2-1.2M18.5 12l-1.2 1.2"
        stroke="currentColor"
        strokeWidth="1.75"
        strokeLinecap="round"
      />
      <path d="M8.5 12H10" stroke="currentColor" strokeWidth="1.75" />
    </svg>
  )
}

function IconCluster(p: { className?: string }) {
  return (
    <svg className={p.className} viewBox="0 0 24 24" fill="none" aria-hidden>
      <circle cx="12" cy="6" r="2.2" stroke="currentColor" strokeWidth="1.75" />
      <circle cx="6" cy="17" r="2.2" stroke="currentColor" strokeWidth="1.75" />
      <circle cx="18" cy="17" r="2.2" stroke="currentColor" strokeWidth="1.75" />
      <path
        d="M12 8.2v3.5M10.2 14.2 7.6 16M13.8 14.2l2.6 1.8"
        stroke="currentColor"
        strokeWidth="1.75"
        strokeLinecap="round"
      />
    </svg>
  )
}

function IconProject(p: { className?: string }) {
  return (
    <svg className={p.className} viewBox="0 0 24 24" fill="none" aria-hidden>
      <path
        d="M4 7.5 12 4l8 3.5v9L12 20l-8-3.5v-9Z"
        stroke="currentColor"
        strokeWidth="1.75"
        strokeLinejoin="round"
      />
      <path
        d="M12 10.5v9.5M4 7.5l8 3 8-3"
        stroke="currentColor"
        strokeWidth="1.75"
        strokeLinejoin="round"
      />
    </svg>
  )
}

const links: {
  to: string
  label: string
  icon: (p: { className?: string }) => ReactElement
  end?: boolean
}[] = [
  { to: '/', label: 'Home', icon: IconOverview, end: true },
  { to: '/setup', label: 'Get started', icon: IconSetup },
  { to: '/apps', label: 'Apps', icon: IconApps },
  { to: '/bpmn', label: 'BPMN', icon: IconBpmn },
  { to: '/cluster', label: 'Cluster', icon: IconCluster },
  { to: '/project', label: 'Project', icon: IconProject },
  { to: '/admin', label: 'Logins', icon: IconAdmin },
  { to: '/containers', label: 'Services', icon: IconContainers },
  { to: '/logs', label: 'Logs', icon: IconLogs },
  { to: '/ai', label: 'AI helpers', icon: IconAI },
  { to: '/monitoring', label: 'Monitoring', icon: IconMonitoring },
  { to: '/tools', label: 'Extras', icon: IconTools },
  { to: '/danger', label: 'Reset lab', icon: IconDanger },
]

function IconSun(p: { className?: string }) {
  return (
    <svg className={p.className} viewBox="0 0 24 24" fill="none" aria-hidden>
      <circle cx="12" cy="12" r="4" stroke="currentColor" strokeWidth="1.75" />
      <path
        d="M12 3v2M12 19v2M4.2 4.2l1.5 1.5M18.3 18.3l1.5 1.5M3 12h2M19 12h2M4.2 19.8l1.5-1.5M18.3 5.7l1.5-1.5"
        stroke="currentColor"
        strokeWidth="1.75"
        strokeLinecap="round"
      />
    </svg>
  )
}

function IconMoon(p: { className?: string }) {
  return (
    <svg className={p.className} viewBox="0 0 24 24" fill="none" aria-hidden>
      <path
        d="M20 13.5A7.5 7.5 0 1 1 10.5 4 6 6 0 0 0 20 13.5Z"
        stroke="currentColor"
        strokeWidth="1.75"
        strokeLinejoin="round"
      />
    </svg>
  )
}

export function App() {
  const [cliVersion, setCliVersion] = useState('')
  const [theme, toggleTheme] = useTheme()

  useEffect(() => {
    void getOverview()
      .then((o) => setCliVersion(o.cliVersion))
      .catch(() => undefined)
  }, [])

  return (
    <div className="shell">
      <nav className="nav" aria-label="Lab">
        <div className="brand">
          <img src="/logo-camunda.svg" alt="Camunda" />
          <div className="brand-sub">Camunda Lab Console</div>
        </div>
        {links.map(({ to, label, icon: Icon, end }) => (
          <NavLink
            key={to}
            to={to}
            end={Boolean(end)}
            className={({ isActive }) => `nav-link${isActive ? ' active' : ''}`}
          >
            <Icon />
            {label}
          </NavLink>
        ))}
        <div className="nav-spacer" />
        <div className="nav-foot">
          <button
            type="button"
            className="theme-toggle"
            onClick={toggleTheme}
            aria-label={theme === 'dark' ? 'Switch to light' : 'Switch to dark'}
            title={theme === 'dark' ? 'Light' : 'Dark'}
          >
            {theme === 'dark' ? <IconSun /> : <IconMoon />}
            <span>{theme === 'dark' ? 'Light' : 'Dark'}</span>
          </button>
          <a className="nav-foot-author" href={PROJECT.authorURL} target="_blank" rel="noreferrer">
            {PROJECT.author}
          </a>
          <div className="nav-foot-links" aria-label="Project links">
            <a
              href={PROJECT.repo}
              target="_blank"
              rel="noreferrer"
              title="GitHub"
              aria-label="GitHub repository"
            >
              <IconGitHub />
            </a>
            <a
              href={PROJECT.docs}
              target="_blank"
              rel="noreferrer"
              title="Lab help docs"
              aria-label="Camunda Lab help docs"
            >
              <IconDocs />
            </a>
            <a
              href={PROJECT.camundaDocs}
              target="_blank"
              rel="noreferrer"
              title="Camunda Docs"
              aria-label="Official Camunda documentation"
            >
              <IconCamundaDocs />
            </a>
            <a
              href={PROJECT.releases}
              target="_blank"
              rel="noreferrer"
              title="Releases"
              aria-label="GitHub releases"
            >
              <IconReleases />
            </a>
          </div>
          {cliVersion && (
            <div className="nav-foot-version">Version v{cliVersion.replace(/^v/, '')}</div>
          )}
        </div>
      </nav>
      <main className="main">
        <Routes>
          <Route path="/" element={<OverviewPage />} />
          <Route path="/setup" element={<SetupPage />} />
          <Route path="/apps" element={<AppsPage />} />
          <Route path="/bpmn" element={<BpmnPage />} />
          <Route path="/cluster" element={<ClusterPage />} />
          <Route path="/project" element={<ProjectPage />} />
          <Route path="/admin" element={<AdminPage />} />
          <Route path="/containers" element={<ContainersPage />} />
          <Route path="/logs" element={<LogsPage />} />
          <Route path="/ai" element={<AIPage />} />
          <Route path="/monitoring" element={<MonitoringPage />} />
          <Route path="/tools" element={<ToolsPage />} />
          <Route path="/danger" element={<DangerPage />} />
        </Routes>
      </main>
    </div>
  )
}
