export type BpmnHistoryKind = 'upload' | 'path' | 'project'

export type BpmnHistoryEntry = {
  id: string
  kind: BpmnHistoryKind
  label: string
  openedAt: number
  tab: string
  path?: string
  projectDir?: string
  fileName?: string
  xml?: string
}

const STORAGE_KEY = 'camunda-lab-bpmn-history'
const MAX_ENTRIES = 24
const MAX_XML_CHARS = 400_000

function readRaw(): BpmnHistoryEntry[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return []
    const parsed = JSON.parse(raw) as BpmnHistoryEntry[]
    return Array.isArray(parsed) ? parsed : []
  } catch {
    return []
  }
}

function writeRaw(entries: BpmnHistoryEntry[]): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(entries.slice(0, MAX_ENTRIES)))
  } catch {
    /* ignore quota errors */
  }
}

function entryKey(entry: Pick<BpmnHistoryEntry, 'kind' | 'path' | 'projectDir' | 'fileName' | 'xml'>): string {
  if (entry.kind === 'path') return `path:${entry.path ?? ''}`
  if (entry.kind === 'project') return `project:${entry.projectDir ?? ''}`
  return `upload:${entry.fileName ?? 'file'}:${(entry.xml ?? '').slice(0, 120)}`
}

function basename(value: string): string {
  const parts = value.split(/[/\\]/)
  return parts[parts.length - 1] || value
}

export function loadBpmnHistory(): BpmnHistoryEntry[] {
  return readRaw().sort((a, b) => b.openedAt - a.openedAt)
}

export function clearBpmnHistory(): void {
  writeRaw([])
}

export function removeBpmnHistoryEntry(id: string): BpmnHistoryEntry[] {
  const next = readRaw().filter((entry) => entry.id !== id)
  writeRaw(next)
  return next
}

export function addBpmnHistory(
  entry: Omit<BpmnHistoryEntry, 'id' | 'openedAt' | 'label'> & { label?: string },
): BpmnHistoryEntry[] {
  const openedAt = Date.now()
  const label =
    entry.label ??
    (entry.kind === 'path'
      ? basename(entry.path ?? 'BPMN file')
      : entry.kind === 'project'
        ? basename(entry.projectDir ?? 'Project')
        : entry.fileName || 'Uploaded BPMN')

  const xml =
    entry.kind === 'upload' && entry.xml && entry.xml.length <= MAX_XML_CHARS ? entry.xml : undefined

  const normalized: BpmnHistoryEntry = {
    id: `${openedAt}-${Math.random().toString(36).slice(2, 8)}`,
    kind: entry.kind,
    label,
    openedAt,
    tab: entry.tab,
    path: entry.path,
    projectDir: entry.projectDir,
    fileName: entry.fileName,
    xml,
  }

  const key = entryKey(normalized)
  const withoutDupes = readRaw().filter((item) => entryKey(item) !== key)
  const next = [normalized, ...withoutDupes].slice(0, MAX_ENTRIES)
  writeRaw(next)
  return next
}

export function fileFromHistory(entry: BpmnHistoryEntry): File | null {
  if (entry.kind !== 'upload' || !entry.xml) return null
  const blob = new Blob([entry.xml], { type: 'application/xml' })
  return new File([blob], entry.fileName || 'process.bpmn', { type: 'application/xml' })
}
