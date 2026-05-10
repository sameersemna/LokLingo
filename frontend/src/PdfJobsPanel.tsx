import { useEffect, useRef, useState } from 'react'
import { fetchJob, type JobResponse } from './api/translate'
import {
  clearStoredJobs,
  loadStoredJobs,
  removeStoredJob,
  type StoredPdfJob,
} from './pdfJobsStorage'

type PdfStageKey = 'queued' | 'extract' | 'languages' | 'translate' | 'render' | 'complete'

const PDF_STAGE_LABELS: Array<{ key: PdfStageKey; label: string }> = [
  { key: 'queued', label: 'Queued' },
  { key: 'extract', label: 'Extracting text' },
  { key: 'languages', label: 'Detecting languages' },
  { key: 'translate', label: 'Translating' },
  { key: 'render', label: 'Rendering typography' },
  { key: 'complete', label: 'Ready' },
]

function getFriendlyFailureMessage(statusError?: string): string {
  const raw = (statusError ?? '').toLowerCase()
  if (!raw) return 'Processing paused. Retry from PDF Jobs in a moment.'
  if (raw.includes('timeout')) return 'Rendering timed out. Recovering strategy is available on retry.'
  if (raw.includes('network') || raw.includes('fetch')) return 'Connection to translation service was interrupted. Please retry shortly.'
  if (raw.includes('retry') || raw.includes('circuit')) return 'Translation engine is stabilizing. Retry this job in a moment.'
  return 'Translation failed for this document. Retry from PDF Jobs to resume.'
}

function inferPdfStage(job: JobResponse | null): PdfStageKey {
  if (!job) return 'queued'
  if (job.status === 'completed') return 'complete'
  if (job.status === 'failed') return 'render'
  const stage = (job.stage ?? '').toLowerCase()
  const message = (job.stage_message ?? '').toLowerCase()
  const marker = `${stage} ${message}`

  if (stage === 'detecting_languages' || marker.includes('language')) return 'languages'
  if (marker.includes('render') || marker.includes('typography') || marker.includes('layout')) return 'render'
  if (marker.includes('translat')) return 'translate'
  if (marker.includes('extract') || marker.includes('ocr') || marker.includes('detect')) return 'extract'

  const progress = typeof job.stage_progress === 'number' ? Math.max(0, Math.min(1, job.stage_progress)) : 0
  if (progress >= 0.95) return 'render'
  if (progress >= 0.65) return 'translate'
  if (progress >= 0.35) return 'languages'
  if (progress > 0.05) return 'extract'
  return 'queued'
}

// ---------- live job row ----------

const STATUS_LABEL: Record<JobResponse['status'], string> = {
  pending: 'Queued',
  processing: 'Translating...',
  completed: 'Done',
  failed: 'Failed',
}

const STATUS_CLASS: Record<JobResponse['status'], string> = {
  pending: 'pdf-job-pending',
  processing: 'pdf-job-processing',
  completed: 'pdf-job-done',
  failed: 'pdf-job-failed',
}

interface JobRowProps {
  stored: StoredPdfJob
  onCopy: (msg: string, type: 'success' | 'error') => void
  onRemove: (jobId: string) => void
}

function JobRow({ stored, onCopy, onRemove }: JobRowProps) {
  const [live, setLive] = useState<JobResponse | null>(null)
  const [expanded, setExpanded] = useState(false)
  const [pollIssue, setPollIssue] = useState(false)
  const stopRef = useRef(false)

  useEffect(() => {
    stopRef.current = false
    let timer: ReturnType<typeof setTimeout>

    async function poll() {
      if (stopRef.current) return
      try {
        const job = await fetchJob(stored.job_id)
        setPollIssue(false)
        setLive(job)
        if (job.status === 'pending' || job.status === 'processing') {
          timer = setTimeout(poll, 1500)
        }
      } catch {
        setPollIssue(true)
        // network error — retry after a longer delay
        timer = setTimeout(poll, 5000)
      }
    }

    poll()
    return () => {
      stopRef.current = true
      clearTimeout(timer)
    }
  }, [stored.job_id])

  const status = live?.status ?? 'pending'
  const isTerminal = status === 'completed' || status === 'failed'
  const text = live?.translated_text ?? ''
  const statusLabel = status === 'processing' ? (live?.stage_message ?? STATUS_LABEL.processing) : STATUS_LABEL[status]
  const stageProgress = status === 'processing' && typeof live?.stage_progress === 'number'
    ? Math.max(0, Math.min(1, live.stage_progress))
    : null
  const currentStage = inferPdfStage(live)
  const activeStageIdx = PDF_STAGE_LABELS.findIndex(stage => stage.key === currentStage)
  const normalizedProgress = status === 'completed'
    ? 1
    : status === 'failed'
    ? Math.max(0.05, stageProgress ?? activeStageIdx / PDF_STAGE_LABELS.length)
    : stageProgress ?? Math.max(0.05, (activeStageIdx + 0.35) / PDF_STAGE_LABELS.length)
  const stageLeadText = status === 'processing'
    ? `${statusLabel}${typeof live?.processed_pages === 'number' && typeof live?.total_pages === 'number' && live.total_pages > 0 ? ` · page ${Math.min(live.processed_pages, live.total_pages)}/${live.total_pages}` : ''}`
    : status === 'failed'
    ? 'Job interrupted'
    : status === 'completed'
    ? 'Pipeline complete'
    : 'Waiting for worker'
  const liveMessage = (live?.stage_message ?? '').toLowerCase()
  const reliabilityHint = pollIssue
    ? 'Translation service reconnecting...'
    : status === 'processing' && (liveMessage.includes('retry') || liveMessage.includes('recover'))
    ? 'Translation engine recovering...'
    : status === 'processing' && (liveMessage.includes('fallback') || liveMessage.includes('switch'))
    ? 'Switching rendering strategy...'
    : status === 'processing' && normalizedProgress < 0.22
    ? 'Warming OCR pipeline...'
    : status === 'processing' && normalizedProgress < 0.5
    ? 'Detecting complex layout...'
    : status === 'processing' && normalizedProgress < 0.84
    ? 'Translating unstable regions...'
    : status === 'processing'
    ? 'Final rendering pass...'
    : null

  const handleCopy = async () => {
    if (!text) return
    try {
      await navigator.clipboard.writeText(text)
      onCopy('Copied to clipboard', 'success')
    } catch {
      onCopy('Copy failed — check browser permissions', 'error')
    }
  }

  const ts = new Date(stored.submittedAt).toLocaleTimeString([], {
    hour: '2-digit',
    minute: '2-digit',
  })

  return (
    <li className="pdf-job-row">
      <div className="pdf-job-row-header">
        <div className="pdf-job-info">
          <span className="pdf-job-filename" title={stored.filename}>
            {stored.filename.length > 32 ? stored.filename.slice(0, 30) + '…' : stored.filename}
          </span>
          <span className="pdf-job-meta">
            {stored.source} → {stored.target} · {ts}
          </span>
          {(stored.mode === 'extract' || stored.mode === 'ocr_only') && (
            <span className="pdf-mode-badge pdf-mode-extract" title="Submitted in Extract mode">
              Extract
            </span>
          )}
        </div>

        <div className="pdf-job-actions">
          <span className={`pdf-job-badge ${STATUS_CLASS[status]}`}>
            {statusLabel}
            {(status === 'pending' || status === 'processing') && (
              <span className="spinner spinner-sm pdf-spinner" aria-hidden="true" />
            )}
          </span>

          {status === 'completed' && live?.processing_method && (
            <span
              className={`pdf-method-badge pdf-method-${live.processing_method}`}
              title={live.processing_method === 'pdf_text' ? 'Extracted from embedded PDF text' : 'Extracted from scanned PDF pages'}
            >
              {live.processing_method === 'pdf_text' ? 'Embedded text' : 'Scanned pages'}
            </span>
          )}

          {status === 'completed' && text && (
            <button className="icon-btn" title="Show / hide translation" onClick={() => setExpanded(e => !e)}>
              {expanded ? '▲' : '▼'}
            </button>
          )}
          {status === 'completed' && text && (
            <button className="icon-btn" title="Copy translation" onClick={handleCopy}>
              ⎘
            </button>
          )}
          {isTerminal && (
            <button
              className="icon-btn"
              title="Remove from list"
              onClick={() => { removeStoredJob(stored.job_id); onRemove(stored.job_id) }}
            >
              ✕
            </button>
          )}
        </div>
      </div>

      {(status === 'processing' || status === 'completed' || status === 'failed') && (
        <>
          <div className="pdf-job-progress" aria-label={`Progress ${Math.round(normalizedProgress * 100)} percent`}>
            <div className="pdf-job-progress-bar">
              <div className="pdf-job-progress-fill" style={{ transform: `scaleX(${normalizedProgress})` }} />
            </div>
            <span className="pdf-job-progress-value">{Math.round(normalizedProgress * 100)}%</span>
          </div>
          <div className="pdf-job-progress-live-row">
            <span>{stageLeadText}</span>
            {reliabilityHint && <span className="pdf-job-reliability">{reliabilityHint}</span>}
          </div>
          <ol className="pdf-job-stage-track" aria-label="PDF translation pipeline">
            {PDF_STAGE_LABELS.map((stage, idx) => {
              const done = status === 'completed' ? true : idx < activeStageIdx
              const active = status === 'processing' && idx === activeStageIdx
              const failed = status === 'failed' && idx === Math.max(0, activeStageIdx)
              return (
                <li key={stage.key} className={`pdf-job-stage-chip${done ? ' is-done' : ''}${active ? ' is-active' : ''}${failed ? ' is-failed' : ''}`}>
                  <span className="pdf-job-stage-dot" aria-hidden="true" />
                  <span>{stage.label}</span>
                </li>
              )
            })}
          </ol>
        </>
      )}

      {expanded && text && (
        <div className="pdf-job-result">{text}</div>
      )}
      {status === 'failed' && live?.error && (
        <div className="pdf-job-error">{getFriendlyFailureMessage(live.error)}</div>
      )}
    </li>
  )
}

// ---------- panel ----------

interface PdfJobsPanelProps {
  onToast: (msg: string, type: 'success' | 'error') => void
}

export function PdfJobsPanel({ onToast }: PdfJobsPanelProps) {
  const [jobs, setJobs] = useState<StoredPdfJob[]>(loadStoredJobs)

  const handleRemove = (jobId: string) => {
    setJobs(prev => prev.filter(j => j.job_id !== jobId))
  }

  const clearAll = () => {
    clearStoredJobs()
    setJobs([])
  }

  return (
    <section className="history-panel">
      <div className="history-header">
        <span>PDF translation jobs</span>
        {jobs.length > 0 && (
          <button className="icon-btn" onClick={clearAll}>
            Clear all
          </button>
        )}
      </div>

      {jobs.length === 0 ? (
        <p className="history-empty">No PDF jobs yet. Upload a PDF using the PDF button.</p>
      ) : (
        <ul className="pdf-job-list">
          {jobs.map(j => (
            <JobRow key={j.job_id} stored={j} onCopy={onToast} onRemove={handleRemove} />
          ))}
        </ul>
      )}
    </section>
  )
}
