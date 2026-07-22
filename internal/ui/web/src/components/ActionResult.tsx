import type { ReactNode } from 'react'
import { triggerTextDownload } from '../api/client'
import {
  deriveResultPhase,
  resultText,
  type ResultPhase,
  type ToolkitEnvelope,
} from '../api/types'

const phaseLabels: Record<ResultPhase, string> = {
  idle: 'Ready',
  loading: 'Running…',
  empty: 'Nothing to show',
  success: 'Success',
  findings: 'Findings',
  partial: 'Partial result',
  unsupported: 'Not supported',
  failure: 'Failed',
}

const phaseBannerClass: Record<ResultPhase, string> = {
  idle: 'info',
  loading: 'info',
  empty: 'info',
  success: 'ok',
  findings: 'warn',
  partial: 'warn',
  unsupported: 'warn',
  failure: 'error',
}

export type ActionResultProps = {
  loading?: boolean
  error?: string | null
  code?: string | null
  result?: ToolkitEnvelope | null
  empty?: boolean
  loadingLabel?: string
  idleLabel?: string
  emptyLabel?: string
  title?: string
  downloadFilename?: string
  children?: ReactNode
  footer?: ReactNode
}

export function ActionResult({
  loading,
  error,
  code,
  result,
  empty,
  loadingLabel = 'Running…',
  idleLabel = 'Choose inputs and run the selected developer tool.',
  emptyLabel = 'Nothing to show yet.',
  title,
  downloadFilename = 'camunda-lab-output.txt',
  children,
  footer,
}: ActionResultProps) {
  const phase = deriveResultPhase({ loading, error, code, result, empty })

  if (phase === 'idle') {
    return <div className="banner info">{idleLabel}</div>
  }
  if (phase === 'loading') {
    return <div className="banner info">{loadingLabel}</div>
  }
  if (phase === 'empty') {
    return <div className="banner info">{emptyLabel}</div>
  }
  if (phase === 'failure' && error) {
    return <div className="banner error">{error}</div>
  }
  if (phase === 'unsupported') {
    return (
      <div className="banner warn">
        {error || 'This capability is not supported in the current environment.'}
      </div>
    )
  }

  const text = resultText(result)
  const heading = title || phaseLabels[phase]

  return (
    <section className="card panel">
      <header className="panel-head">
        <h2>{heading}</h2>
        <span className={`pill ${phaseBannerClass[phase]}`}>{phase}</span>
      </header>
      {result?.cli && (
        <p className="hint">
          CLI: <code>{result.cli}</code>
        </p>
      )}
      {result?.hint && phase !== 'success' && <p className="hint">{result.hint}</p>}
      {error && phase !== 'failure' && <div className="banner error">{error}</div>}
      {children ?? (text ? <pre className="code">{text}</pre> : null)}
      {(text || children) && (
        <div className="panel-actions">
          <button
            type="button"
            onClick={() => {
              void navigator.clipboard.writeText(text)
            }}
            disabled={!text}
          >
            Copy text
          </button>
          <button
            type="button"
            onClick={() => triggerTextDownload(text, downloadFilename)}
            disabled={!text}
          >
            Download text
          </button>
        </div>
      )}
      {result?.mode === 'written' && (
        <div className="banner ok">Artifacts were written to the authorized directory.</div>
      )}
      {footer}
    </section>
  )
}

export { deriveResultPhase, phaseLabels }
