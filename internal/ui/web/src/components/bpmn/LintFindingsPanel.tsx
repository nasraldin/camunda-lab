import { useMemo } from 'react'
import type { LintFindingRow } from './lintModel'
import { groupFindingsByFile, lintRuleLabel } from './lintModel'

type LintFindingsPanelProps = {
  findings: LintFindingRow[]
  activeFile?: string
  activeElementId?: string
  onSelect?: (finding: LintFindingRow) => void
}

function fileLabel(path: string): string {
  const parts = path.split(/[/\\]/)
  return parts[parts.length - 1] || path
}

export function LintFindingsPanel({
  findings,
  activeFile,
  activeElementId,
  onSelect,
}: LintFindingsPanelProps) {
  const groups = useMemo(() => groupFindingsByFile(findings), [findings])
  const fileKeys = useMemo(() => Array.from(groups.keys()), [groups])

  if (!findings.length) {
    return <div className="banner ok">No issues found — this BPMN looks good for the selected rules.</div>
  }

  const errorCount = findings.filter((item) => item.severity === 'error').length
  const warningCount = findings.filter((item) => item.severity === 'warning').length

  return (
    <section className="bpmn-findings-section">
      <header className="bpmn-findings-head">
        <h3>
          {findings.length} issue{findings.length === 1 ? '' : 's'}
          {errorCount > 0 ? ` · ${errorCount} error${errorCount === 1 ? '' : 's'}` : ''}
          {warningCount > 0 ? ` · ${warningCount} warning${warningCount === 1 ? '' : 's'}` : ''}
        </h3>
        <p className="hint">Click a row to zoom the diagram to that element.</p>
      </header>

      {fileKeys.map((file) => {
        const rows = groups.get(file) ?? []
        if (activeFile && file !== activeFile) return null
        return (
          <div key={file} className="bpmn-findings-group">
            {fileKeys.length > 1 && <h4 className="bpmn-findings-file">{fileLabel(file)}</h4>}
            <div className="bpmn-findings-table" role="table">
              <div className="bpmn-findings-row bpmn-findings-header" role="row">
                <span role="columnheader">Severity</span>
                <span role="columnheader">Issue</span>
                <span role="columnheader">Element</span>
                <span role="columnheader">Rule</span>
              </div>
              {rows.map((finding, index) => {
                const active = finding.element && finding.element === activeElementId
                return (
                  <button
                    key={`${finding.rule}-${finding.element ?? 'doc'}-${index}`}
                    type="button"
                    role="row"
                    className={`bpmn-findings-row bpmn-findings-data${active ? ' active' : ''}`}
                    onClick={() => onSelect?.(finding)}
                  >
                    <span className={`bpmn-severity-pill ${finding.severity}`} role="cell">
                      {finding.severity}
                    </span>
                    <span className="bpmn-finding-message" role="cell">
                      {finding.message}
                    </span>
                    <span className="bpmn-finding-element" role="cell">
                      {finding.element || '—'}
                    </span>
                    <span className="bpmn-finding-rule" role="cell">
                      {lintRuleLabel(finding.rule)}
                    </span>
                  </button>
                )
              })}
            </div>
          </div>
        )
      })}
    </section>
  )
}
