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
        findings: [],
        contents: {
          'process.bpmn':
            '<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p"><startEvent id="s"/><endEvent id="e"/><sequenceFlow sourceRef="s" targetRef="e"/></process></definitions>',
        },
        inputs: ['process.bpmn'],
        cli: 'camunda lint process.bpmn',
      }
    case 'findings':
      return {
        ok: false,
        status: 'failed',
        complete: true,
        findings: [
          {
            processId: 'orderProcess',
            finding: {
              rule: 'process-start-event',
              severity: 'error',
              message: 'Process has no start event',
              element: 'orderProcess',
              file: 'process.bpmn',
            },
          },
        ],
        output: 'process-start-event error   orderProcess Process has no start event',
        contents: {
          'process.bpmn':
            '<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="orderProcess"><task id="task"/><endEvent id="end"/></process></definitions>',
        },
        inputs: ['process.bpmn'],
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
