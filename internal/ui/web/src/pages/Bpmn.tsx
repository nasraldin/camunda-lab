import { useCallback, useEffect, useRef, useState } from 'react'
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
import {
  addBpmnHistory,
  clearBpmnHistory,
  fileFromHistory,
  loadBpmnHistory,
  removeBpmnHistoryEntry,
  type BpmnHistoryEntry,
} from '../bpmnHistory'
import { ActionResult } from '../components/ActionResult'
import { BpmnHistoryPanel } from '../components/bpmn/BpmnHistoryPanel'
import { BpmnVisualResults } from '../components/bpmn/BpmnVisualResults'
import { bpmnFilesFromDirectoryPicker, folderLabelFromFiles } from '../components/bpmn/bpmnFolder'
import { DownloadButton } from '../components/DownloadButton'
import { getProjectDir, setProjectDir } from '../projectDir'
import '../components/bpmn/bpmn.css'

type Tab = 'lint' | 'diff' | 'explain' | 'review' | 'testgen' | 'scan'
type InputMode = 'file' | 'project'

const TABS: { id: Tab; label: string; cli: string }[] = [
  { id: 'lint', label: 'Lint', cli: 'camunda lint <file.bpmn>' },
  { id: 'diff', label: 'Diff', cli: 'camunda diff a.bpmn --against b.bpmn' },
  { id: 'explain', label: 'Explain', cli: 'camunda explain <file.bpmn>' },
  { id: 'review', label: 'Review', cli: 'camunda review <file.bpmn>' },
  { id: 'testgen', label: 'Test gen', cli: 'camunda test generate <file.bpmn> -o <dir>' },
  { id: 'scan', label: 'Scan', cli: 'camunda scan <dir>' },
]

const MAX_PROJECT_FOLDER_FILES = 8

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
  const [projectFolderFiles, setProjectFolderFiles] = useState<File[]>([])
  const [projectFolderLabel, setProjectFolderLabel] = useState('')
  const [projectFolderTruncated, setProjectFolderTruncated] = useState(false)
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
  const [inputMode, setInputMode] = useState<InputMode>('file')
  const [showRaw, setShowRaw] = useState(false)
  const [localXml, setLocalXml] = useState('')
  const [localXmlBefore, setLocalXmlBefore] = useState('')
  const [localXmlAfter, setLocalXmlAfter] = useState('')
  const requestGeneration = useRef(0)
  const activeRequest = useRef<AbortController | null>(null)
  const pendingHistoryRun = useRef<BpmnHistoryEntry | null>(null)
  const [history, setHistory] = useState(() => loadBpmnHistory())
  const [activeHistoryId, setActiveHistoryId] = useState<string | undefined>()

  const rememberHistory = useCallback(
    (entry: Parameters<typeof addBpmnHistory>[0]) => {
      const next = addBpmnHistory(entry)
      setHistory(next)
      setActiveHistoryId(next[0]?.id)
    },
    [],
  )

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
    setLocalXml('')
    setLocalXmlBefore('')
    setLocalXmlAfter('')
  }

  async function readFileText(upload: File | null): Promise<string> {
    if (!upload) return ''
    return upload.text()
  }

  function recordRunInHistory(initiatingTab: Tab) {
    if (initiatingTab === 'scan') {
      const dir = projectPath.trim()
      if (dir) rememberHistory({ kind: 'project', tab: 'scan', projectDir: dir })
      return
    }
    if (initiatingTab === 'lint' && inputMode === 'project') {
      if (projectFolderFiles.length) {
        rememberHistory({
          kind: 'project',
          tab: 'lint',
          projectDir: projectFolderLabel || 'Uploaded folder',
          label: projectFolderLabel || 'Uploaded folder',
        })
        return
      }
      const dir = projectPath.trim()
      if (dir) rememberHistory({ kind: 'project', tab: 'lint', projectDir: dir })
      return
    }
    if (path.trim()) {
      rememberHistory({ kind: 'path', tab: initiatingTab, path: path.trim() })
      return
    }
    if (file) {
      void readFileText(file).then((xml) => {
        if (!xml) return
        rememberHistory({ kind: 'upload', tab: initiatingTab, fileName: file.name, xml })
      })
    }
  }

  async function run() {
    const historyEntry = pendingHistoryRun.current
    pendingHistoryRun.current = null

    const runTab = (historyEntry?.tab as Tab | undefined) ?? tab
    const runInputMode: InputMode =
      historyEntry?.kind === 'project' ? 'project' : inputMode
    const runProjectPath =
      historyEntry?.kind === 'project' ? (historyEntry.projectDir ?? '') : projectPath
    const runPath = historyEntry?.kind === 'path' ? (historyEntry.path ?? '') : path
    const runFile =
      historyEntry?.kind === 'upload' ? fileFromHistory(historyEntry) : file
    const runProjectFolderFiles = projectFolderFiles

    activeRequest.current?.abort()
    const controller = new AbortController()
    activeRequest.current = controller
    const generation = ++requestGeneration.current
    const initiatingTab = runTab
    const skipHistoryRecord = Boolean(historyEntry)
    const show = (response: ToolkitEnvelope) => {
      if (requestGeneration.current !== generation) return
      setResult(response)
      if (!skipHistoryRecord) recordRunInHistory(initiatingTab)
      if (!response.ok && response.error) {
        setError(response.error)
        setErrorCode(response.code ?? null)
      }
    }
    setBusy(true)
    resetResult()
    try {
      if (runTab === 'scan') {
        const dir = runProjectPath.trim()
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
      if (runTab === 'lint' && runInputMode === 'project') {
        const ignoreList = ignore
          ? ignore.split(',').map((value) => value.trim()).filter(Boolean)
          : []
        if (runProjectFolderFiles.length) {
          const fd = new FormData()
          for (const picked of runProjectFolderFiles) {
            const relative = (picked as File & { webkitRelativePath?: string }).webkitRelativePath
            fd.append('files', picked, relative || picked.name)
          }
          fd.append('failOn', failOn)
          if (ignoreList.length) {
            for (const rule of ignoreList) fd.append('ignore', rule)
          }
          if (runProjectFolderFiles[0]) {
            setLocalXml(await readFileText(runProjectFolderFiles[0]))
          }
          show(await lintBpmnUpload(fd, controller.signal))
          return
        }
        const dir = runProjectPath.trim()
        if (!dir) throw new Error('Choose a project folder or set an absolute project directory')
        setProjectDir(dir)
        setLocalXml('')
        show(
          await lintBpmn(
            { projectDir: dir, failOn: failOn as LintThreshold, ignore: ignoreList },
            controller.signal,
          ),
        )
        return
      }
      if (runTab === 'diff') {
        if (gitBase.trim()) {
          if (!runProjectPath.trim() || !runPath.trim()) {
            throw new Error('Git diff needs a project directory and project-relative BPMN path')
          }
          show(
            await diffBpmn(
              {
                projectDir: runProjectPath.trim(),
                path: runPath.trim(),
                base: gitBase.trim(),
              },
              controller.signal,
            ),
          )
          return
        }
        if (runPath && pathB) {
          show(await diffBpmn({ paths: [runPath, pathB] }, controller.signal))
          return
        }
        if (!runFile || !fileB) throw new Error('Upload two BPMN files (or set two absolute paths)')
        const fd = new FormData()
        fd.append('from', runFile)
        fd.append('to', fileB)
        setLocalXmlBefore(await readFileText(runFile))
        setLocalXmlAfter(await readFileText(fileB))
        show(await diffBpmnUpload(fd, controller.signal))
        return
      }

      if (runPath.trim()) {
        const ignoreList = ignore
          ? ignore.split(',').map((value) => value.trim()).filter(Boolean)
          : []
        if (runTab === 'lint') {
          setLocalXml('')
          show(
            await lintBpmn(
              { path: runPath.trim(), failOn: failOn as LintThreshold, ignore: ignoreList },
              controller.signal,
            ),
          )
          return
        }
        if (runTab === 'explain') {
          show(await explainBpmn({ path: runPath.trim() }, controller.signal))
          return
        }
        if (runTab === 'review') {
          show(
            await reviewBpmn(
              {
                path: runPath.trim(),
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
              path: runPath.trim(),
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

      if (!runFile) throw new Error('Upload a BPMN file or set an absolute path')
      setLocalXml(await readFileText(runFile))
      const fd = new FormData()
      fd.append('file', runFile)
      if (runTab === 'lint' || runTab === 'review') {
        fd.append('failOn', failOn)
        if (ignore) fd.append('ignore', ignore)
      }
      if (runTab === 'review') {
        fd.append('ai', String(aiEnabled))
        fd.append('aiRequired', String(aiRequired))
        if (aiEnabled || aiRequired) {
          fd.append('provider', provider)
          fd.append('model', model)
        }
      }
      if (runTab === 'testgen') {
        fd.append('lang', lang)
        fd.append('write', String(writeArtifacts))
        if (writeArtifacts) {
          fd.append('output', outputDir.trim())
          if (writeForce) fd.append('force', 'true')
        }
      }

      if (runTab === 'lint') {
        show(await lintBpmnUpload(fd, controller.signal))
      } else if (runTab === 'explain') {
        show(await explainBpmnUpload(fd, controller.signal))
      } else if (runTab === 'review') {
        show(await reviewBpmnUpload(fd, controller.signal))
      } else {
        show(await generateTestsUpload(fd, controller.signal))
      }
    } catch (e) {
      if (controller.signal.aborted) return
      if (requestGeneration.current === generation) {
        if (e instanceof ApiError) {
          setError(e.message)
          setErrorCode(e.code || null)
        } else {
          setError(e instanceof Error ? e.message : String(e))
        }
      }
    } finally {
      if (requestGeneration.current === generation) {
        setBusy(false)
      }
      if (activeRequest.current === controller) activeRequest.current = null
    }
  }

  function openFromHistory(entry: BpmnHistoryEntry) {
    rememberHistory({
      kind: entry.kind,
      tab: entry.tab,
      path: entry.path,
      projectDir: entry.projectDir,
      fileName: entry.fileName,
      xml: entry.xml,
      label: entry.label,
    })
    if (entry.tab) setTab(entry.tab as Tab)
    if (entry.kind === 'project') {
      setInputMode('project')
      setProjectPath(entry.projectDir ?? '')
      setProjectFolderFiles([])
      setProjectFolderLabel('')
      setProjectFolderTruncated(false)
      setPath('')
      setFile(null)
      setLocalXml('')
    } else if (entry.kind === 'path') {
      setInputMode('file')
      setPath(entry.path ?? '')
      setFile(null)
      setLocalXml('')
    } else {
      setInputMode('file')
      setPath('')
      const restored = fileFromHistory(entry)
      setFile(restored)
      setLocalXml(entry.xml ?? '')
    }
    pendingHistoryRun.current = entry
    void run()
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
          Lint, diff, explain, and review with a visual diagram and plain-language findings. Raw JSON
          is available when you need it.
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

      <section className="card bpmn-input-layout">
        <div className="bpmn-input-main">
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
            {tab === 'lint' && (
              <div className="field">
                <span>Input scope</span>
                <div className="tab-row">
                  <button
                    type="button"
                    className={`tab-btn${inputMode === 'file' ? ' active' : ''}`}
                    onClick={() => {
                      setInputMode('file')
                      setProjectFolderFiles([])
                      setProjectFolderLabel('')
                      setProjectFolderTruncated(false)
                    }}
                  >
                    Single file
                  </button>
                  <button
                    type="button"
                    className={`tab-btn${inputMode === 'project' ? ' active' : ''}`}
                    onClick={() => setInputMode('project')}
                  >
                    Project directory
                  </button>
                </div>
              </div>
            )}
            {tab === 'lint' && inputMode === 'project' ? (
              <>
                <label className="field">
                  <span>Project folder</span>
                  <input
                    type="file"
                    multiple
                    // @ts-expect-error non-standard directory picker attributes
                    webkitdirectory=""
                    directory=""
                    onChange={(e) => {
                      const all = bpmnFilesFromDirectoryPicker(e.target.files)
                      const picked = all.slice(0, MAX_PROJECT_FOLDER_FILES)
                      const label = folderLabelFromFiles(picked)
                      setProjectFolderFiles(picked)
                      setProjectFolderLabel(label)
                      setProjectFolderTruncated(all.length > MAX_PROJECT_FOLDER_FILES)
                      if (picked.length) setProjectPath('')
                      if (picked[0]) {
                        void readFileText(picked[0]).then(setLocalXml)
                        rememberHistory({
                          kind: 'project',
                          tab: 'lint',
                          projectDir: label,
                          label,
                        })
                      }
                      e.target.value = ''
                    }}
                  />
                  {projectFolderLabel ? (
                    <p className="hint">
                      {projectFolderLabel}: {projectFolderFiles.length} BPMN file
                      {projectFolderFiles.length === 1 ? '' : 's'} selected.
                      {projectFolderTruncated
                        ? ` Only the first ${MAX_PROJECT_FOLDER_FILES} files are linted — use an absolute path for full project discovery.`
                        : ''}
                    </p>
                  ) : (
                    <p className="hint">
                      Pick a folder from this computer. Up to {MAX_PROJECT_FOLDER_FILES} BPMN files
                      are uploaded for linting.
                    </p>
                  )}
                </label>
                <label className="field">
                  <span>Or absolute path</span>
                  <input
                    value={projectPath}
                    onChange={(e) => {
                      setProjectPath(e.target.value)
                      if (e.target.value.trim()) {
                        setProjectFolderFiles([])
                        setProjectFolderLabel('')
                        setProjectFolderTruncated(false)
                      }
                    }}
                    placeholder="/tmp/cam-demo"
                  />
                  <p className="hint">
                    Uses <code>.camunda.yaml</code> discovery on this machine (same as{' '}
                    <code>camunda lint</code> with no file args).
                  </p>
                </label>
              </>
            ) : (
              <>
                <label className="field">
                  <span>BPMN upload</span>
                  <input
                    type="file"
                    accept=".bpmn,application/xml,text/xml"
                    onChange={(e) => {
                      const next = e.target.files?.[0] || null
                      setFile(next)
                      void readFileText(next).then((xml) => {
                        setLocalXml(xml)
                        if (next && xml) {
                          rememberHistory({ kind: 'upload', tab, fileName: next.name, xml })
                        }
                      })
                    }}
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
              </>
            )}
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
          {(tab === 'lint' || tab === 'diff' || tab === 'explain' || tab === 'review') && (
            <button type="button" onClick={() => setShowRaw((value) => !value)}>
              {showRaw ? 'Show visual view' : 'Show raw JSON'}
            </button>
          )}
          {tab === 'testgen' && !writeArtifacts && (
            <DownloadButton
              label="Download tests (ZIP)"
              disabled={busy || (!path.trim() && !file)}
              onDownload={() => downloadTestsZip()}
              onError={setError}
            />
          )}
        </div>
        </div>

        <BpmnHistoryPanel
          entries={history}
          activeId={activeHistoryId}
          onSelect={openFromHistory}
          onRemove={(id) => {
            setHistory(removeBpmnHistoryEntry(id))
            if (activeHistoryId === id) setActiveHistoryId(undefined)
          }}
          onClear={() => {
            clearBpmnHistory()
            setHistory([])
            setActiveHistoryId(undefined)
          }}
        />
      </section>

      <ActionResult
        loading={busy}
        error={error}
        code={errorCode}
        result={result}
        loadingLabel={`Running ${meta.label.toLowerCase()}…`}
        downloadFilename={`camunda-lab-${tab}.txt`}
        children={
          result &&
          (tab === 'lint' || tab === 'diff' || tab === 'explain' || tab === 'review') ? (
            <BpmnVisualResults
              tab={tab}
              result={result}
              localXml={localXml}
              localXmlBefore={localXmlBefore}
              localXmlAfter={localXmlAfter}
              showRaw={showRaw}
            />
          ) : undefined
        }
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
