import type { ToolkitEnvelope } from '../../src/api/types'

export type MockToolkitState =
  | 'loading'
  | 'empty'
  | 'success'
  | 'findings'
  | 'partial'
  | 'unsupported'
  | 'failure'

export function mockToolkitResponse(state: MockToolkitState): ToolkitEnvelope {
  switch (state) {
    case 'empty':
      return { ok: true, status: 'completed', complete: true, output: '', items: [] }
    case 'success':
      return {
        ok: true,
        status: 'completed',
        complete: true,
        output: 'OK',
        cli: 'camunda lint process.bpmn',
      }
    case 'findings':
      return {
        ok: false,
        status: 'failed',
        complete: true,
        findings: [{ rule: 'process-start-event', message: 'missing start event' }],
        output: '1 finding',
        cli: 'camunda lint process.bpmn',
      }
    case 'partial':
      return {
        ok: false,
        status: 'partial',
        complete: false,
        output: 'partial timeline',
        warnings: [{ code: 'trace_timeout', message: 'follow stopped after 30s' }],
        cli: 'camunda trace 123 --follow',
      }
    case 'unsupported':
      return {
        ok: false,
        code: 'unsupported',
        error: 'This capability is not available in this lab profile',
      }
    case 'failure':
      return {
        ok: false,
        status: 'failed',
        error: 'upstream request failed',
        hint: 'Check orchestration URL and credentials',
      }
    case 'loading':
    default:
      return { ok: true, output: 'pending' }
  }
}
