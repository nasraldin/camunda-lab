import { useEffect, useRef, useState } from 'react'
import { ApiError } from '../api'
import {
  diffBpmn,
  diffBpmnUpload,
  downloadArtifactContents,
  downloadGeneratedTests,
  downloadGeneratedTestsUpload,
  explainBpmn,
  explainBpmnUpload,
  generateTests,
  generateTestsUpload,
  lintBpmn,
  lintBpmnUpload,
  reviewBpmn,
  reviewBpmnUpload,
  scanProject,
} from '../api/toolkit'
import type { GenerateLanguage, LintThreshold, ScanThreshold, ToolkitEnvelope } from '../api/types'
import { ActionResult } from '../components/ActionResult'
import { DownloadButton } from '../components/DownloadButton'
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
  const [errorCode, setErrorCode] = useState<string | null>(null)
  const [result, setResult] = useState<ToolkitEnvelope | null>(null)
  const [file, setFile] = useState<File | null>(null)
  const [fileB, setFileB] = useState<File | null>(null)
  const [path, setPath] = useState('')
  const [pathB, setPathB] = useState('')
  const [projectPath, setProjectPath] = useState(getProjectDir())
  const [lang, setLang] = useState<GenerateLanguage>('java')
  const [failOn, setFailOn] = useState<string>('error')
  const [ignore, setIgnore] = useState('')
  const [aiEnabled, setAIEnabled] = useState(false)
  const [aiRequired, setAIRequired] = useState(false)
  const [provider, setProvider] = useState('openai')
  const [model, setModel] = useState('gpt-4o-mini')
  const [writeArtifacts, setWriteArtifacts] = useState(false)
  const [writeForce, setWriteForce] = useState(false)
  const [outputDir, setOutputDir] = useState('')
  const [gitBase, setGitBase] = useState('')
  const requestGeneration = useRef(0)
  const activeRequest = useRef<AbortController | null>(null)

  useEffect(
    () => () => {
      requestGeneration.current += 1
      activeRequest.current?.abort()
      activeRequest.current = null
    },
    [],
  )

  function resetResult() {
    setError('')
    setErrorCode(null)
    setResult(null)
  }

  async function run() {
    activeRequest.current?.abort()
    const controller = new AbortController()
    activeRequest.current = controller
    const generation = ++requestGeneration.current
    const initiatingTab = tab
    const show = (response: ToolkitEnvelope) => {
      if (requestGeneration.current !== generation || tab !== initiatingTab) return
      setResult(response)
      if (!response.ok && response.error) {
        setError(response.error)
        setErrorCode(response.code ?? null)
      }
    }
    setBusy(true)
    resetResult()
    try {
      if (tab === 'scan') {
        const dir = projectPath.trim()
        if (!dir) throw new Error('Set an absolute project directory')
        setProjectDir(dir)
        show(
          await scanProject({
            dir,
            failOn: failOn as ScanThreshold,
            ignore: ignore ? ignore.split(',').map((value) => value.trim()).filter(Boolean) : [],
          }),
        )
        return
      }
      if (tab === 'diff') {
        if (gitBase.trim()) {
          if (!projectPath.trim() || !path.trim()) {
            throw new Error('Git diff needs a project directory and project-relative BPMN path')
          }
          show(
            await diffBpmn(
              {
                projectDir: projectPath.trim(),
                path: path.trim(),
                base: gitBase.trim(),
              },
              controller.signal,
            ),
          )
          return
        }
        if (path && pathB) {
          show(await diffBpmn({ paths: [path, pathB] }, controller.signal))
          return
        }
        if (!file || !fileB) throw new Error('Upload two BPMN files (or set two absolute paths)')
        const fd = new FormData()
        fd.append('from', file)
        fd.append('to', fileB)
        show(await diffBpmnUpload(fd, controller.signal))
        return
      }

      if (path.trim()) {
        const ignoreList = ignore
          ? ignore.split(',').map((value) => value.trim()).filter(Boolean)
          : []
        if (tab === 'lint') {
          show(
            await lintBpmn(
              { path: path.trim(), failOn: failOn as LintThreshold, ignore: ignoreList },
              controller.signal,
            ),
          )
          return
        }
        if (tab === 'explain') {
          show(await explainBpmn({ path: path.trim() }, controller.signal))
          return
        }
        if (tab === 'review') {
          show(
            await reviewBpmn(
              {
                path: path.trim(),
                failOn: failOn as LintThreshold,
                ignore: ignoreList,
                ai: aiEnabled,
                aiRequired,
                ...(aiEnabled || aiRequired ? { provider, model } : {}),
              },
              controller.signal,
            ),
          )
          return
        }
        show(
          await generateTests(
            {
              path: path.trim(),
              lang,
              write: writeArtifacts,
              ...(writeArtifacts
                ? {
                    output: outputDir.trim(),
                    ...(writeForce ? { force: true } : {}),
                  }
                : {}),
            },
            controller.signal,
          ),
        )
        return
      }

      if (!file) throw new Error('Upload a BPMN file or set an absolute path')
      const fd = new FormData()
      fd.append('file', file)
      if (tab === 'lint' || tab === 'review') {
        fd.append('failOn', failOn)
        if (ignore) fd.append('ignore', ignore)
      }
      if (tab === 'review') {
        fd.append('ai', String(aiEnabled))
        fd.append('aiRequired', String(aiRequired))
        if (aiEnabled || aiRequired) {
          fd.append('provider', provider)
          fd.append('model', model)
        }
      }
      if (tab === 'testgen') {
        fd.append('lang', lang)
        fd.append('write', String(writeArtifacts))
        if (writeArtifacts) {
          fd.append('output', outputDir.trim())
          if (writeForce) fd.append('force', 'true')
        }
      }

      if (tab === 'lint') {
        show(await lintBpmnUpload(fd, controller.signal))
      } else if (tab === 'explain') {
        show(await explainBpmnUpload(fd, controller.signal))
      } else if (tab === 'review') {
        show(await reviewBpmnUpload(fd, controller.signal))
      } else {
        show(await generateTestsUpload(fd, controller.signal))
      }
    } catch (e) {
      if (controller.signal.aborted) return
      if (requestGeneration.current === generation && tab === initiatingTab) {
        if (e instanceof ApiError) {
          setError(e.message)
          setErrorCode(e.code || null)
        } else {
          setError(e instanceof Error ? e.message : String(e))
        }
      }
    } finally {
      if (requestGeneration.current === generation && tab === initiatingTab) {
        setBusy(false)
      }
      if (activeRequest.current === controller) activeRequest.current = null
    }
  }

  const meta = TABS.find((t) => t.id === tab)!

  async function downloadTestsZip() {
    if (path.trim()) {
      return downloadGeneratedTests({ path: path.trim(), lang })
    }
    if (!file) throw new Error('Upload a BPMN file or set an absolute path')
    const fd = new FormData()
    fd.append('file', file)
    fd.append('lang', lang)
    return downloadGeneratedTestsUpload(fd)
  }

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
              requestGeneration.current += 1
              activeRequest.current?.abort()
              activeRequest.current = null
              setTab(t.id)
              setBusy(false)
              setFailOn(t.id === 'scan' ? 'medium' : 'error')
              resetResult()
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
          <div className="stack-sm" key="diff-inputs">
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
            <label className="field">
              <span>Git base (optional)</span>
              <input
                value={gitBase}
                onChange={(e) => setGitBase(e.target.value)}
                placeholder="HEAD~1"
              />
            </label>
            {gitBase && (
              <label className="field">
                <span>Project directory for Git diff</span>
                <input value={projectPath} onChange={(e) => setProjectPath(e.target.value)} />
              </label>
            )}
          </div>
        ) : (
          <div className="stack-sm" key="single-input">
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
              <>
                <label className="field">
                  <span>Language</span>
                  <select
                    value={lang}
                    onChange={(e) => setLang(e.target.value as GenerateLanguage)}
                  >
                    <option value="java">java</option>
                    <option value="js">js</option>
                    <option value="python">python</option>
                  </select>
                </label>
                <label className="pref-switch">
                  <input
                    type="checkbox"
                    checked={writeArtifacts}
                    onChange={(e) => {
                      const checked = e.target.checked
                      setWriteArtifacts(checked)
                      if (!checked) setWriteForce(false)
                    }}
                  />
                  <span className="pref-switch-track" aria-hidden />
                  <span className="pref-switch-label">
                    Write to an authorized directory instead of downloading
                  </span>
                </label>
                {writeArtifacts && (
                  <>
                    <label className="field">
                      <span>Authorized output directory (absolute)</span>
                      <input value={outputDir} onChange={(e) => setOutputDir(e.target.value)} />
                    </label>
                    <label className="pref-switch">
                      <input
                        type="checkbox"
                        checked={writeForce}
                        onChange={(e) => setWriteForce(e.target.checked)}
                      />
                      <span className="pref-switch-track" aria-hidden />
                      <span className="pref-switch-label">Force (overwrite existing files)</span>
                    </label>
                  </>
                )}
              </>
            )}
          </div>
        )}

        {(tab === 'lint' || tab === 'review' || tab === 'scan') && (
          <div className="stack-sm">
            <label className="field">
              <span>Fail threshold</span>
              <select value={failOn} onChange={(e) => setFailOn(e.target.value)}>
                {tab === 'scan' ? (
                  <>
                    <option value="low">low</option>
                    <option value="medium">medium</option>
                    <option value="high">high</option>
                  </>
                ) : (
                  <>
                    <option value="error">error</option>
                    <option value="warning">warning</option>
                  </>
                )}
              </select>
            </label>
            <label className="field">
              <span>Ignore rules/patterns (comma-separated)</span>
              <input value={ignore} onChange={(e) => setIgnore(e.target.value)} />
            </label>
          </div>
        )}

        {tab === 'review' && (
          <div className="stack-sm">
            <label className="pref-switch">
              <input
                type="checkbox"
                checked={aiEnabled}
                onChange={(e) => setAIEnabled(e.target.checked)}
              />
              <span className="pref-switch-track" aria-hidden />
              <span className="pref-switch-label">Optional AI enrichment</span>
            </label>
            <label className="pref-switch">
              <input
                type="checkbox"
                checked={aiRequired}
                onChange={(e) => setAIRequired(e.target.checked)}
              />
              <span className="pref-switch-track" aria-hidden />
              <span className="pref-switch-label">Require AI success</span>
            </label>
            {(aiEnabled || aiRequired) && (
              <>
                <label className="field">
                  <span>Provider</span>
                  <select value={provider} onChange={(e) => setProvider(e.target.value)}>
                    <option value="openai">openai</option>
                    <option value="anthropic">anthropic</option>
                  </select>
                </label>
                <label className="field">
                  <span>Model</span>
                  <input value={model} onChange={(e) => setModel(e.target.value)} />
                </label>
              </>
            )}
          </div>
        )}

        <div className="panel-actions">
          <button type="button" className="primary" onClick={() => void run()} disabled={busy}>
            {busy ? 'Running…' : 'Run'}
          </button>
          {tab === 'testgen' && !writeArtifacts && (
            <DownloadButton
              label="Download tests (ZIP)"
              disabled={busy || (!path.trim() && !file)}
              onDownload={() => downloadTestsZip()}
              onError={setError}
            />
          )}
        </div>
      </section>

      <ActionResult
        loading={busy}
        error={error}
        code={errorCode}
        result={result}
        loadingLabel={`Running ${meta.label.toLowerCase()}…`}
        downloadFilename={`camunda-lab-${tab}.txt`}
        footer={
          result?.mode === 'download' && result.artifacts?.length ? (
            <div className="panel-actions">
              <button
                type="button"
                className="primary"
                onClick={() => downloadArtifactContents(result.artifacts || [])}
              >
                Download generated artifacts
              </button>
              <DownloadButton
                label="Download tests archive (ZIP)"
                onDownload={() => downloadTestsZip()}
                onError={setError}
              />
            </div>
          ) : undefined
        }
      />
    </div>
  )
}
