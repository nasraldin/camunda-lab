import { useState } from 'react'
import { ApiError, postForm, toolkitJSON, type ToolkitResult } from '../api'
import { getProjectDir, setProjectDir } from '../projectDir'

type Tab = 'lint' | 'diff' | 'explain' | 'review' | 'testgen' | 'scan'

const TABS: { id: Tab; label: string; cli: string }[] = [
  { id: 'lint', label: 'Lint', cli: 'camunda lint <file.bpmn>' },
  { id: 'diff', label: 'Diff', cli: 'camunda diff a.bpmn --against b.bpmn' },
  { id: 'explain', label: 'Explain', cli: 'camunda explain <file.bpmn>' },
  { id: 'review', label: 'Review', cli: 'camunda review <file.bpmn>' },
  { id: 'testgen', label: 'Test gen', cli: 'camunda test generate <file.bpmn> -o <dir>' },
  { id: 'scan', label: 'Scan', cli: 'camunda scan <dir>' },
]

export function BpmnPage() {
  const [tab, setTab] = useState<Tab>('lint')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [result, setResult] = useState<ToolkitResult | null>(null)
  const [file, setFile] = useState<File | null>(null)
  const [fileB, setFileB] = useState<File | null>(null)
  const [path, setPath] = useState('')
  const [pathB, setPathB] = useState('')
  const [projectPath, setProjectPath] = useState(getProjectDir())
  const [lang, setLang] = useState('java')

  function show(r: ToolkitResult) {
    setResult(r)
    if (!r.ok && r.error) setError(r.error)
  }

  async function run() {
    setBusy(true)
    setError('')
    setResult(null)
    try {
      if (tab === 'scan') {
        const dir = projectPath.trim()
        if (!dir) throw new Error('Set an absolute project directory')
        setProjectDir(dir)
        show(await toolkitJSON('/api/v1/bpmn/scan', { dir }))
        return
      }
      if (tab === 'diff') {
        if (path && pathB) {
          show(await toolkitJSON('/api/v1/bpmn/diff', { paths: [path, pathB] }))
          return
        }
        if (!file || !fileB) throw new Error('Upload two BPMN files (or set two absolute paths)')
        const fd = new FormData()
        fd.append('from', file)
        fd.append('to', fileB)
        show(await postForm('/api/v1/bpmn/diff', fd))
        return
      }
      const endpoint =
        tab === 'lint'
          ? '/api/v1/bpmn/lint'
          : tab === 'explain'
            ? '/api/v1/bpmn/explain'
            : tab === 'review'
              ? '/api/v1/bpmn/review'
              : '/api/v1/bpmn/test-generate'
      if (path.trim()) {
        const body: Record<string, unknown> = { paths: [path.trim()] }
        if (tab === 'testgen') body.lang = lang
        show(await toolkitJSON(endpoint, body))
        return
      }
      if (!file) throw new Error('Upload a BPMN file or set an absolute path')
      const fd = new FormData()
      fd.append('file', file)
      if (tab === 'testgen') fd.append('lang', lang)
      show(await postForm(endpoint, fd))
    } catch (e) {
      setError(e instanceof ApiError ? e.message : e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  const meta = TABS.find((t) => t.id === tab)!

  return (
    <div className="stack">
      <div className="page-head">
        <h1>BPMN toolkit</h1>
        <p className="lead">
          Lint, diff, explain, review, generate tests, and scan — same engines as the CLI.
        </p>
      </div>

      <div className="tab-row" role="tablist">
        {TABS.map((t) => (
          <button
            key={t.id}
            type="button"
            role="tab"
            aria-selected={tab === t.id}
            className={`tab-btn${tab === t.id ? ' active' : ''}`}
            onClick={() => {
              setTab(t.id)
              setResult(null)
              setError('')
            }}
          >
            {t.label}
          </button>
        ))}
      </div>

      <section className="card panel">
        <p className="hint">
          Equivalent CLI: <code>{meta.cli}</code>
        </p>

        {tab === 'scan' ? (
          <label className="field">
            <span>Project directory (absolute)</span>
            <input
              value={projectPath}
              onChange={(e) => setProjectPath(e.target.value)}
              placeholder="/tmp/cam-demo"
            />
          </label>
        ) : tab === 'diff' ? (
          <div className="stack-sm">
            <label className="field">
              <span>From (upload)</span>
              <input
                type="file"
                accept=".bpmn,application/xml,text/xml"
                onChange={(e) => setFile(e.target.files?.[0] || null)}
              />
            </label>
            <label className="field">
              <span>To (upload)</span>
              <input
                type="file"
                accept=".bpmn,application/xml,text/xml"
                onChange={(e) => setFileB(e.target.files?.[0] || null)}
              />
            </label>
            <p className="hint">Or absolute paths:</p>
            <label className="field">
              <span>From path</span>
              <input
                value={path}
                onChange={(e) => setPath(e.target.value)}
                placeholder="/Users/…/order-v1.bpmn"
              />
            </label>
            <label className="field">
              <span>To path</span>
              <input
                value={pathB}
                onChange={(e) => setPathB(e.target.value)}
                placeholder="/Users/…/order-v2.bpmn"
              />
            </label>
          </div>
        ) : (
          <div className="stack-sm">
            <label className="field">
              <span>BPMN upload</span>
              <input
                type="file"
                accept=".bpmn,application/xml,text/xml"
                onChange={(e) => setFile(e.target.files?.[0] || null)}
              />
            </label>
            <label className="field">
              <span>Or absolute path</span>
              <input
                value={path}
                onChange={(e) => setPath(e.target.value)}
                placeholder="/Users/…/process.bpmn"
              />
            </label>
            {tab === 'testgen' && (
              <label className="field">
                <span>Language</span>
                <select value={lang} onChange={(e) => setLang(e.target.value)}>
                  <option value="java">java</option>
                  <option value="js">js</option>
                </select>
              </label>
            )}
          </div>
        )}

        <div className="panel-actions">
          <button type="button" className="primary" disabled={busy} onClick={() => void run()}>
            {busy ? 'Running…' : 'Run'}
          </button>
        </div>
      </section>

      {error && <div className="banner error">{error}</div>}
      {result && (
        <section className="card panel">
          <header className="panel-head">
            <h2>{result.ok ? 'OK' : 'Findings'}</h2>
          </header>
          {result.cli && (
            <p className="hint">
              CLI: <code>{result.cli}</code>
            </p>
          )}
          <pre className="code">
            {result.output || result.markdown || JSON.stringify(result, null, 2)}
          </pre>
        </section>
      )}
    </div>
  )
}
