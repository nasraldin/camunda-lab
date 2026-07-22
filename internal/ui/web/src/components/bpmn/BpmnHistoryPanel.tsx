import type { BpmnHistoryEntry } from '../../bpmnHistory'
import { IconTrash } from '../../icons'

type BpmnHistoryPanelProps = {
  entries: BpmnHistoryEntry[]
  activeId?: string
  onSelect: (entry: BpmnHistoryEntry) => void
  onRemove: (id: string) => void
  onClear: () => void
}

function formatWhen(openedAt: number): string {
  try {
    return new Intl.DateTimeFormat(undefined, {
      month: 'short',
      day: 'numeric',
      hour: 'numeric',
      minute: '2-digit',
    }).format(openedAt)
  } catch {
    return new Date(openedAt).toLocaleString()
  }
}

function kindLabel(entry: BpmnHistoryEntry): string {
  if (entry.kind === 'project') return 'Project'
  if (entry.kind === 'path') return 'Path'
  return 'Upload'
}

function detailLine(entry: BpmnHistoryEntry): string {
  if (entry.kind === 'project') return entry.projectDir ?? ''
  if (entry.kind === 'path') return entry.path ?? ''
  return entry.fileName ?? 'Uploaded file'
}

export function BpmnHistoryPanel({
  entries,
  activeId,
  onSelect,
  onRemove,
  onClear,
}: BpmnHistoryPanelProps) {
  return (
    <aside className="bpmn-history" aria-label="Recently opened BPMN">
      <header className="bpmn-history-head">
        <div>
          <h2>History</h2>
          <p className="hint">Files and paths you opened recently. Click to open again.</p>
        </div>
        <button
          type="button"
          className="bpmn-history-clear"
          onClick={onClear}
          disabled={!entries.length}
          aria-label="Clear all history"
          title="Clear all"
        >
          <IconTrash className="bpmn-history-clear-icon" />
        </button>
      </header>

      {entries.length === 0 ? (
        <p className="bpmn-history-empty">Upload a BPMN file or run with a path — it will appear here.</p>
      ) : (
        <ul className="bpmn-history-list">
          {entries.map((entry) => (
            <li key={entry.id}>
              <div className={`bpmn-history-item${activeId === entry.id ? ' active' : ''}`}>
                <button
                  type="button"
                  className="bpmn-history-open"
                  onClick={() => onSelect(entry)}
                  title={detailLine(entry)}
                >
                  <span className="bpmn-history-label">{entry.label}</span>
                  <span className="bpmn-history-meta">
                    <span className="bpmn-history-kind">{kindLabel(entry)}</span>
                    <span className="bpmn-history-tab">{entry.tab}</span>
                    <span className="bpmn-history-time">{formatWhen(entry.openedAt)}</span>
                  </span>
                  <span className="bpmn-history-detail">{detailLine(entry)}</span>
                </button>
                <button
                  type="button"
                  className="bpmn-history-remove"
                  onClick={() => onRemove(entry.id)}
                  aria-label={`Remove ${entry.label} from history`}
                >
                  ×
                </button>
              </div>
            </li>
          ))}
        </ul>
      )}
    </aside>
  )
}
