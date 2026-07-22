import type BpmnViewer from 'bpmn-js/lib/NavigatedViewer'

type DiagramElement = {
  id?: string
  x?: number
  y?: number
  width?: number
  height?: number
}

type CanvasApi = {
  zoom: (level?: number | 'fit-viewport', center?: { x: number; y: number }) => number
  resized: () => void
  scrollToElement: (
    element: DiagramElement | string,
    padding?: number | { top?: number; right?: number; bottom?: number; left?: number },
  ) => void
  viewbox: (box?: {
    x: number
    y: number
    width: number
    height: number
  }) => {
    outer: { width: number; height: number }
    inner: { x: number; y: number; width: number; height: number }
    scale: number
    x: number
    y: number
    width: number
    height: number
  }
  addMarker: (id: string, marker: string) => void
  removeMarker: (id: string, marker: string) => void
  getRootElement: () => DiagramElement
  setRootElement: (element: DiagramElement) => void
}

type RegistryApi = {
  get: (id: string) => DiagramElement | undefined
  forEach: (cb: (element: DiagramElement) => void) => void
}

const FOCUS_PADDING = 48
const MAX_FOCUS_ZOOM = 2.4
const MAX_FIT_ZOOM = 4
const FIT_PADDING = 48

export function canvasApi(viewer: BpmnViewer): CanvasApi {
  return viewer.get('canvas') as CanvasApi
}

export function registryApi(viewer: BpmnViewer): RegistryApi {
  return viewer.get('elementRegistry') as RegistryApi
}

export function currentZoom(viewer: BpmnViewer): number {
  return canvasApi(viewer).zoom()
}

export function applyLintMarkers(
  viewer: BpmnViewer,
  highlightIds: string[],
  severity: 'error' | 'warning',
): void {
  const canvas = canvasApi(viewer)
  const registry = registryApi(viewer)
  const marker = severity === 'warning' ? 'lint-warning' : 'lint-error'

  registry.forEach((element) => {
    if (!element.id) return
    canvas.removeMarker(element.id, 'lint-error')
    canvas.removeMarker(element.id, 'lint-warning')
  })

  for (const id of highlightIds) {
    if (registry.get(id)) {
      canvas.addMarker(id, marker)
    }
  }
}

export function fitDiagramToViewport(viewer: BpmnViewer): number {
  const canvas = canvasApi(viewer)
  canvas.resized()
  const { outer, inner } = canvas.viewbox()
  const innerWidth = Math.max(inner.width, 1)
  const innerHeight = Math.max(inner.height, 1)
  const scale = Math.min(
    (outer.width - FIT_PADDING * 2) / innerWidth,
    (outer.height - FIT_PADDING * 2) / innerHeight,
    MAX_FIT_ZOOM,
  )
  const centerX = inner.x + innerWidth / 2
  const centerY = inner.y + innerHeight / 2
  canvas.viewbox({
    x: centerX - outer.width / scale / 2,
    y: centerY - outer.height / scale / 2,
    width: outer.width / scale,
    height: outer.height / scale,
  })
  return canvas.zoom()
}

export function scheduleFitDiagram(viewer: BpmnViewer, onZoom: (level: number) => void): void {
  const run = () => {
    const level = fitDiagramToViewport(viewer)
    onZoom(level)
  }

  requestAnimationFrame(() => {
    requestAnimationFrame(run)
  })
}

function viewportPixelCenter(viewer: BpmnViewer): { x: number; y: number } {
  const outer = canvasApi(viewer).viewbox().outer
  return { x: outer.width / 2, y: outer.height / 2 }
}

/** Zoom in/out around the center of the visible canvas (same behavior as mouse wheel). */
export function stepZoomDiagram(viewer: BpmnViewer, direction: 1 | -1): number {
  const canvas = canvasApi(viewer)
  canvas.resized()
  const zoomScroll = viewer.get('zoomScroll') as {
    stepZoom: (delta: number, position?: { x: number; y: number }) => void
  }
  zoomScroll.stepZoom(direction, viewportPixelCenter(viewer))
  return canvas.zoom()
}

function elementCenter(element: DiagramElement): { x: number; y: number } | null {
  if (
    typeof element.x !== 'number' ||
    typeof element.y !== 'number' ||
    typeof element.width !== 'number' ||
    typeof element.height !== 'number'
  ) {
    return null
  }
  return { x: element.x + element.width / 2, y: element.y + element.height / 2 }
}

function zoomToElementBounds(viewer: BpmnViewer, element: DiagramElement): number {
  const canvas = canvasApi(viewer)
  canvas.scrollToElement(element, FOCUS_PADDING)
  const current = canvas.zoom()
  if (current > MAX_FOCUS_ZOOM) {
    const center = elementCenter(element)
    if (center) canvas.zoom(MAX_FOCUS_ZOOM, center)
  }
  return canvas.zoom()
}

export function focusDiagramElement(
  viewer: BpmnViewer,
  elementId: string,
  severity: 'error' | 'warning' = 'error',
): boolean {
  const canvas = canvasApi(viewer)
  const registry = registryApi(viewer)
  const element = registry.get(elementId)
  if (!element) return false

  canvas.resized()
  zoomToElementBounds(viewer, element)
  applyLintMarkers(viewer, [elementId], severity)
  return true
}
