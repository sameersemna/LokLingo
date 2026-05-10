import { useEffect, useRef, useState } from 'react'
import { fetchJob, type JobResponse } from './api/translate'
import {
  clearStoredJobs,
  loadStoredJobs,
  removeStoredJob,
  type StoredPdfJob,
} from './pdfJobsStorage'

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
  const stopRef = useRef(false)

  useEffect(() => {
    stopRef.current = false
    let timer: ReturnType<typeof setTimeout>

    async function poll() {
      if (stopRef.current) return
      try {
        const job = await fetchJob(stored.job_id)
        setLive(job)
        if (job.status === 'pending' || job.status === 'processing') {
          timer = setTimeout(poll, 1500)
        }
      } catch {
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

      {stageProgress !== null && stageProgress > 0 && (
    <div className="pdf-job-progress" aria-label={`Progress ${Math.round(stageProgress * 100)} percent`}>
      <div className="pdf-job-progress-bar">
      <div className="pdf-job-progress-fill" style={{ width: `${stageProgress * 100}%` }} />
      </div>
      <span className="pdf-job-progress-value">{Math.round(stageProgress * 100)}%</span>
    </div>
    )}

      {expanded && text && (
        <div className="pdf-job-result">{text}</div>
      )}
      {status === 'failed' && live?.error && (
        <div className="pdf-job-error">{live.error}</div>
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
