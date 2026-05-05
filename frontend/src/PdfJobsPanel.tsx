import { useEffect, useRef, useState } from 'react'
import { fetchJob, type JobResponse } from './api/translate'

// ---------- persistence ----------

const JOBS_KEY = 'loklingo-pdf-jobs'
const MAX_STORED = 20

export interface StoredPdfJob {
  job_id: string
  filename: string
  source: string
  target: string
  submittedAt: number
}

function loadStoredJobs(): StoredPdfJob[] {
  try {
    return JSON.parse(localStorage.getItem(JOBS_KEY) ?? '[]')
  } catch {
    return []
  }
}

export function saveStoredJob(job: StoredPdfJob) {
  const existing = loadStoredJobs()
  const next = [job, ...existing.filter(j => j.job_id !== job.job_id)].slice(0, MAX_STORED)
  localStorage.setItem(JOBS_KEY, JSON.stringify(next))
}

function removeStoredJob(jobId: string) {
  const next = loadStoredJobs().filter(j => j.job_id !== jobId)
  localStorage.setItem(JOBS_KEY, JSON.stringify(next))
}

// ---------- live job row ----------

const STATUS_LABEL: Record<JobResponse['status'], string> = {
  pending: 'Queued',
  processing: 'Translating…',
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
        </div>

        <div className="pdf-job-actions">
          <span className={`pdf-job-badge ${STATUS_CLASS[status]}`}>
            {STATUS_LABEL[status]}
            {(status === 'pending' || status === 'processing') && (
              <span className="spinner spinner-sm pdf-spinner" aria-hidden="true" />
            )}
          </span>

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

  // Re-sync from localStorage when the panel is shown (e.g. new job added externally)
  useEffect(() => {
    setJobs(loadStoredJobs())
  }, [])

  const handleRemove = (jobId: string) => {
    setJobs(prev => prev.filter(j => j.job_id !== jobId))
  }

  const clearAll = () => {
    localStorage.removeItem(JOBS_KEY)
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
        <p className="history-empty">No PDF jobs yet. Upload a PDF using the OCR / PDF button.</p>
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
