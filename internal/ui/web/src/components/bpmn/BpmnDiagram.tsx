import { useCallback, useEffect, useRef, useState } from 'react'
import BpmnViewer from 'bpmn-js/lib/NavigatedViewer'
import { layoutProcess } from 'bpmn-auto-layout'
import 'bpmn-js/dist/assets/diagram-js.css'
import 'bpmn-js/dist/assets/bpmn-js.css'
import {
  applyLintMarkers,
  canvasApi,
  currentZoom,
  fitDiagramToViewport,
  focusDiagramElement,
  scheduleFitDiagram,
  stepZoomDiagram,
} from './bpmnCanvas'
import './bpmn.css'

export type BpmnDiagramProps = {
  xml: string
  highlightIds?: string[]
  highlightSeverity?: 'error' | 'warning'
  focusElementId?: string
  focusRequest?: number
  onElementClick?: (elementId: string) => void
}

async function layoutXml(xml: string): Promise<string> {
  try {
    return await layoutProcess(xml)
  } catch {
    return xml
  }
}

function formatZoom(level: number): string {
  return `${Math.round(level * 100)}%`
}

export function BpmnDiagram({
  xml,
  highlightIds = [],
  highlightSeverity = 'error',
  focusElementId,
  focusRequest = 0,
  onElementClick,
}: BpmnDiagramProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const viewerRef = useRef<BpmnViewer | null>(null)
  const focusRef = useRef<string | undefined>(undefined)
  const [diagramError, setDiagramError] = useState('')
  const [ready, setReady] = useState(false)
  const [zoomLabel, setZoomLabel] = useState('100%')

  const updateZoomLabel = useCallback((viewer: BpmnViewer) => {
    setZoomLabel(formatZoom(currentZoom(viewer)))
  }, [])

  const fitDiagram = useCallback(() => {
    const viewer = viewerRef.current
    if (!viewer) return
    focusRef.current = undefined
    scheduleFitDiagram(viewer, (level) => setZoomLabel(formatZoom(level)))
  }, [])

  const zoomBy = useCallback((direction: 1 | -1) => {
    const viewer = viewerRef.current
    if (!viewer) return
    focusRef.current = undefined
    const level = stepZoomDiagram(viewer, direction)
    setZoomLabel(formatZoom(level))
  }, [])

  const focusElement = useCallback(
    (elementId: string) => {
      const viewer = viewerRef.current
      if (!viewer || !elementId) return
      focusRef.current = elementId
      const focused = focusDiagramElement(viewer, elementId, highlightSeverity)
      if (!focused) {
        fitDiagramToViewport(viewer)
      }
      updateZoomLabel(viewer)
    },
    [highlightSeverity, updateZoomLabel],
  )

  useEffect(() => {
    if (!containerRef.current) return
    const viewer = new BpmnViewer({ container: containerRef.current })
    viewerRef.current = viewer

    const eventBus = viewer.get('eventBus') as {
      on: (event: string, cb: (event: { element?: { id?: string } }) => void) => void
    }
    eventBus.on('element.click', (event) => {
      const element = event.element as { id?: string; type?: string; labelTarget?: unknown } | undefined
      const id = element?.id
      if (!id || element?.labelTarget) return
      const type = element.type ?? ''
      if (
        type === 'label' ||
        type.endsWith(':Process') ||
        type.endsWith(':Collaboration') ||
        type.endsWith(':Definitions')
      ) {
        return
      }
      onElementClick?.(id)
    })

    return () => {
      viewer.destroy()
      viewerRef.current = null
    }
  }, [onElementClick])

  useEffect(() => {
    const viewer = viewerRef.current
    if (!viewer || !xml.trim()) {
      setReady(false)
      setDiagramError('')
      return
    }

    let cancelled = false
    setDiagramError('')
    setReady(false)
    focusRef.current = undefined

    void (async () => {
      try {
        const layouted = await layoutXml(xml)
        await viewer.importXML(layouted)
        if (cancelled) return

        scheduleFitDiagram(viewer, (level) => {
          if (cancelled) return
          setZoomLabel(formatZoom(level))
          setReady(true)
        })
      } catch (error) {
        if (!cancelled) {
          setDiagramError(error instanceof Error ? error.message : String(error))
        }
      }
    })()

    return () => {
      cancelled = true
    }
  }, [xml])

  useEffect(() => {
    const viewer = viewerRef.current
    if (!viewer || !ready) return
    applyLintMarkers(viewer, highlightIds, highlightSeverity)
  }, [ready, highlightIds, highlightSeverity])

  useEffect(() => {
    const viewer = viewerRef.current
    if (!viewer || !ready || !focusElementId) return
    focusRef.current = undefined
    focusElement(focusElementId)
  }, [ready, focusElementId, focusRequest, focusElement])

  useEffect(() => {
    const node = containerRef.current
    if (!node || !ready) return

    const observer = new ResizeObserver(() => {
      const viewer = viewerRef.current
      if (!viewer) return
      canvasApi(viewer).resized()
      updateZoomLabel(viewer)
    })
    observer.observe(node)
    return () => observer.disconnect()
  }, [ready, updateZoomLabel])

  if (!xml.trim()) {
    return <div className="bpmn-diagram-empty">Select or upload a BPMN file to preview the diagram.</div>
  }

  return (
    <div className="bpmn-diagram-shell bpmn-viewer">
      <div className="bpmn-diagram-toolbar" role="toolbar" aria-label="Diagram zoom controls">
        <button type="button" className="btn-sm" onClick={() => zoomBy(1)} disabled={!ready} aria-label="Zoom in">
          +
        </button>
        <button type="button" className="btn-sm" onClick={() => zoomBy(-1)} disabled={!ready} aria-label="Zoom out">
          −
        </button>
        <button type="button" className="btn-sm" onClick={fitDiagram} disabled={!ready}>
          Fit
        </button>
        <span className="bpmn-zoom-label">{zoomLabel}</span>
        <span className="bpmn-diagram-hint">Scroll to pan · drag background to move · click a finding to zoom there</span>
      </div>
      <div ref={containerRef} className="bpmn-diagram-canvas" aria-label="BPMN diagram preview" />
      {diagramError && (
        <div className="bpmn-diagram-note">
          Could not render a visual diagram for this file. Findings below still apply.
          <span className="muted"> ({diagramError})</span>
        </div>
      )}
    </div>
  )
}
