import { useEffect, useMemo, useState } from 'react'
import type { DiffResult, ExplainResult, LintResult, ReviewResult, ToolkitEnvelope } from '../../api/types'
import { BpmnDiagram } from './BpmnDiagram'
import { LintFindingsPanel } from './LintFindingsPanel'
import { findingsForFile, normalizeLintFindings } from './lintModel'
import './bpmn.css'

type BpmnVisualResultsProps = {
  tab: 'lint' | 'diff' | 'explain' | 'review'
  result: ToolkitEnvelope
  localXml?: string
  localXmlBefore?: string
  localXmlAfter?: string
  showRaw: boolean
}

function DiffChanges({ changes }: { changes: unknown[] }) {
  if (!changes.length) {
    return <div className="banner ok">No semantic changes between the two versions.</div>
  }
  return (
    <div className="bpmn-diff-list">
      {changes.map((item, index) => {
        const change = item as Record<string, unknown>
        const nested = change.change as Record<string, unknown> | undefined
        const summary = String(
          nested?.summary ?? change.summary ?? nested?.kind ?? change.kind ?? 'Change detected',
        )
        return (
          <div key={`${summary}-${index}`} className="bpmn-diff-item">
            {summary}
          </div>
        )
      })}
    </div>
  )
}

function ExplainMarkdown({ result }: { result: ExplainResult }) {
  const markdown =
    result.output ||
    result.processes?.map((process) => process.markdown).join('\n\n') ||
    ''
  if (!markdown.trim()) {
    return <div className="banner info">No explanation text was returned.</div>
  }
  return <div className="bpmn-markdown">{markdown}</div>
}

export function BpmnVisualResults({
  tab,
  result,
  localXml,
  localXmlBefore,
  localXmlAfter,
  showRaw,
}: BpmnVisualResultsProps) {
  const lintFindings = useMemo(
    () => normalizeLintFindings((result as LintResult).findings),
    [result],
  )
  const [selectedElement, setSelectedElement] = useState<string | undefined>()
  const [focusTarget, setFocusTarget] = useState<string | undefined>()
  const [focusRequest, setFocusRequest] = useState(0)

  const contents = result.contents ?? {}
  const fileKeys = Object.keys(contents)
  const [selectedFile, setSelectedFile] = useState(fileKeys[0] ?? '')

  useEffect(() => {
    if (fileKeys.length && !fileKeys.includes(selectedFile)) {
      setSelectedFile(fileKeys[0] ?? '')
    }
  }, [fileKeys, selectedFile])

  const lintXml =
    localXml ||
    contents[selectedFile] ||
    (fileKeys.length === 1 ? contents[fileKeys[0] ?? ''] : '') ||
    contents[result.inputs?.[0] ?? ''] ||
    ''

  const highlightIds = useMemo(() => {
    const ids = new Set<string>()
    for (const finding of lintFindings) {
      if (finding.element) ids.add(finding.element)
    }
    return Array.from(ids)
  }, [lintFindings])

  if (showRaw) {
    return <pre className="code">{JSON.stringify(result, null, 2)}</pre>
  }

  if (tab === 'lint' || tab === 'review') {
    const findings = tab === 'lint' ? lintFindings : normalizeLintFindings((result as ReviewResult).findings)
    const fileFindings = selectedFile
      ? findingsForFile(findings, selectedFile)
      : findings
    const fileHighlightIds = fileFindings
      .map((finding) => finding.element)
      .filter((id): id is string => Boolean(id))

    return (
      <div className="bpmn-workspace stacked">
        {fileKeys.length > 1 && (
          <div className="bpmn-file-tabs" role="tablist" aria-label="BPMN file">
            {fileKeys.map((file) => {
              const count = findingsForFile(findings, file).length
              const errors = findingsForFile(findings, file).filter((item) => item.severity === 'error').length
              return (
                <button
                  key={file}
                  type="button"
                  role="tab"
                  aria-selected={selectedFile === file}
                  className={`bpmn-file-tab${selectedFile === file ? ' active' : ''}`}
                  onClick={() => {
                    setSelectedFile(file)
                    setSelectedElement(undefined)
                    setFocusTarget(undefined)
                  }}
                >
                  {file.split('/').pop()} ({errors || count})
                </button>
              )
            })}
          </div>
        )}

        <BpmnDiagram
          xml={lintXml ?? ''}
          highlightIds={
            selectedElement
              ? [selectedElement]
              : fileHighlightIds.length
                ? fileHighlightIds
                : highlightIds
          }
          highlightSeverity={fileFindings.some((item) => item.severity === 'error') ? 'error' : 'warning'}
          focusElementId={focusTarget}
          focusRequest={focusRequest}
          onElementClick={setSelectedElement}
        />

        <LintFindingsPanel
          findings={findings}
          activeFile={selectedFile || undefined}
          activeElementId={selectedElement ?? focusTarget}
          onSelect={(finding) => {
            if (!finding.element) return
            setSelectedElement(finding.element)
            setFocusTarget(finding.element)
            setFocusRequest((value) => value + 1)
          }}
        />
      </div>
    )
  }

  if (tab === 'explain') {
    return (
      <div className="bpmn-workspace stacked">
        <BpmnDiagram xml={localXml || contents[Object.keys(contents)[0] ?? ''] || ''} />
        <section className="bpmn-findings-section">
          <header className="bpmn-findings-head">
            <h3>Explanation</h3>
          </header>
          <ExplainMarkdown result={result as ExplainResult} />
        </section>
      </div>
    )
  }

  if (tab === 'diff') {
    const diff = result as DiffResult
    const before = localXmlBefore || contents.before || ''
    const after = localXmlAfter || contents.after || ''
    return (
      <div className="bpmn-workspace stacked">
        <DiffChanges changes={diff.changes ?? []} />
        <section>
          <header className="bpmn-findings-head">
            <h3>Before</h3>
          </header>
          <BpmnDiagram xml={before} />
        </section>
        <section>
          <header className="bpmn-findings-head">
            <h3>After</h3>
          </header>
          <BpmnDiagram xml={after} />
        </section>
      </div>
    )
  }

  return null
}
