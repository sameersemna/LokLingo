import { useCallback, useEffect, useRef, useState } from "react"
import { FileText, Copy, ChevronDown, ChevronUp, X, Plus, Trash2, ArrowUpRight, Search } from "lucide-react"
import { fetchJob, subscribeJobEvents, type JobResponse } from './api/translate'
import { ApiError } from "./utils/errors"
import {
  clearStoredJobs,
  loadStoredJobs,
  removeStoredJob,
  type StoredPdfJob,
} from './pdfJobsStorage'

type PdfStageKey = 'queued' | 'extract' | 'languages' | 'translate' | 'render' | 'complete'
type JobListStatus = JobResponse["status"] | "missing"

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
  if (!raw) return 'This job was interrupted.'
  if (raw.includes('timeout')) return 'Document processing timed out.'
  if (raw.includes('network') || raw.includes('fetch')) return 'Connection to the translation service was interrupted.'
  if (raw.includes('retry') || raw.includes('circuit')) return 'The translation provider could not complete this job.'
  return 'Translation failed for this document.'
}

function inferPdfStage(job: JobResponse | null): PdfStageKey {
  if (!job) return 'queued'
  if (job.status === 'completed') return 'complete'
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
  onStatusChange: (jobId: string, status: JobListStatus) => void
  onOpenJob: (jobId: string) => void
  hidden: boolean
}

function JobRow({ stored, onCopy, onRemove, onStatusChange, onOpenJob, hidden }: JobRowProps) {
  const [live, setLive] = useState<JobResponse | null>(null)
  const [expanded, setExpanded] = useState(false)
  const [pollIssue, setPollIssue] = useState(false)
  const [missing, setMissing] = useState(false)
  const stopRef = useRef(false)

  useEffect(() => {
    stopRef.current = false
    let timer: ReturnType<typeof setTimeout>
    let closeStream: (() => void) | null = null

    const poll = async () => {
      if (stopRef.current) return
      try {
        const job = await fetchJob(stored.job_id)
        setPollIssue(false)
        setLive(job)
        if (job.status === 'pending' || job.status === 'processing') {
          timer = setTimeout(poll, 1800)
        }
      } catch (error) {
        if (stopRef.current) return
        if (error instanceof ApiError && error.status === 404) {
          setMissing(true)
          closeStream?.()
          return
        }
        setPollIssue(true)
        timer = setTimeout(poll, 5000)
      }
    }

    poll()
    closeStream = subscribeJobEvents(
      stored.job_id,
      (event) => {
        if (stopRef.current) return
        setPollIssue(false)
        setLive(event)
        if (event.status === 'completed' || event.status === 'failed') {
          closeStream?.()
        }
      },
      () => {
        if (stopRef.current) return
        setPollIssue(true)
      },
    )

    return () => {
      stopRef.current = true
      clearTimeout(timer)
      closeStream?.()
    }
  }, [stored.job_id])

  const status = live?.status ?? 'pending'
  useEffect(() => { onStatusChange(stored.job_id, missing ? "missing" : status) }, [missing, onStatusChange, status, stored.job_id])
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
    : stageProgress
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
    : status === 'processing' && currentStage === 'queued'
    ? 'Warming OCR pipeline...'
    : status === 'processing' && currentStage === 'extract'
    ? 'Detecting complex layout...'
    : status === 'processing' && currentStage === 'translate'
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
    <li className="pdf-job-row" hidden={hidden}>
      <div className="pdf-job-row-header">
        <div className="pdf-job-info">
          <span className="pdf-job-filename" title={stored.filename}>
            {stored.filename.length > 32 ? stored.filename.slice(0, 30) + '…' : stored.filename}
          </span>
          <span className="pdf-job-meta">
            {stored.source} → {stored.target} · {ts}
          </span>
          <button type="button" className="text-action job-detail-link" onClick={() => onOpenJob(stored.job_id)}><span>{stored.job_id}</span><ArrowUpRight size={14} aria-hidden="true" /></button>
          {(stored.mode === 'extract' || stored.mode === 'ocr_only') && (
            <span className="pdf-mode-badge pdf-mode-extract" title="Submitted in Extract mode">
              Extract
            </span>
          )}
        </div>

        <div className="pdf-job-actions">
          <span className={`pdf-job-badge ${STATUS_CLASS[status]}`}>
            {missing ? "Expired" : statusLabel}
            {!missing && (status === 'pending' || status === 'processing') && (
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
              {expanded ? <ChevronUp size={16} aria-hidden="true" /> : <ChevronDown size={16} aria-hidden="true" />}
            </button>
          )}
          {status === 'completed' && text && (
            <button className="icon-btn" title="Copy translation" onClick={handleCopy}>
              <Copy size={16} aria-hidden="true" />
            </button>
          )}
          {(isTerminal || missing) && (
            <button
              className="icon-btn"
              title="Remove from list"
              onClick={() => { removeStoredJob(stored.job_id); onRemove(stored.job_id) }}
            >
              <X size={16} aria-hidden="true" />
            </button>
          )}
        </div>
      </div>

      {missing && <p className="pdf-job-error">This job is no longer available on the server.</p>}
      {pollIssue && !missing && <p className="pdf-job-warning" role="status">Connection interrupted. Reconnecting...</p>}

      {(status === 'processing' || status === 'completed' || status === 'failed') && (
        <>
          <div className="pdf-job-progress" aria-label={normalizedProgress === null ? 'Progress unavailable' : `Progress ${Math.round(normalizedProgress * 100)} percent`}>
            <div className="pdf-job-progress-bar">
              <div className={`pdf-job-progress-fill${normalizedProgress === null && status === 'processing' ? ' is-indeterminate' : ''}`} style={{ transform: `scaleX(${normalizedProgress ?? 0})` }} />
            </div>
            <span className="pdf-job-progress-value">{normalizedProgress === null ? status === 'failed' ? 'Interrupted' : 'Processing' : `${Math.round(normalizedProgress * 100)}%`}</span>
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
        <div className="pdf-job-result" dir="auto">{text}</div>
      )}
      {status === 'failed' && live?.error && (
        <div className="pdf-job-error">{getFriendlyFailureMessage(live.error)}</div>
      )}
      {Array.isArray(live?.warnings) && live.warnings.length > 0 && (
        <div className="pdf-job-warning" role="status" aria-live="polite">
          {live.warnings[0]}
          {typeof live.ocr_confidence === 'number' && live.ocr_confidence > 0 && (
            <span>{` (OCR confidence ${Math.round(live.ocr_confidence * 100)}%)`}</span>
          )}
        </div>
      )}
    </li>
  )
}

// ---------- panel ----------

interface PdfJobsPanelProps {
  onToast: (msg: string, type: 'success' | 'error') => void
  onNewJob: () => void
  onOpenJob: (jobId: string) => void
}

export function PdfJobsPanel({ onToast, onNewJob, onOpenJob }: PdfJobsPanelProps) {
  const [jobs, setJobs] = useState<StoredPdfJob[]>(loadStoredJobs)
  const [query, setQuery] = useState("")
  const [filter, setFilter] = useState<"all" | JobListStatus>("all")
  const [statuses, setStatuses] = useState<Record<string, JobListStatus>>({})
  const updateStatus = useCallback((jobId: string, status: JobListStatus) => {
    setStatuses(previous => previous[jobId] === status ? previous : { ...previous, [jobId]: status })
  }, [])
  const matches = (job: StoredPdfJob) => (filter === "all" || (statuses[job.job_id] ?? "pending") === filter) && `${job.filename} ${job.job_id} ${job.source} ${job.target}`.toLowerCase().includes(query.trim().toLowerCase())
  const visibleCount = jobs.filter(matches).length
  const filters = [
    { value: "all", label: "All" }, { value: "pending", label: "Queued" },
    { value: "processing", label: "Processing" }, { value: "completed", label: "Completed" },
    { value: "failed", label: "Failed" }, { value: "missing", label: "Expired" },
  ] as const

  const handleRemove = (jobId: string) => {
    setJobs(prev => prev.filter(j => j.job_id !== jobId))
  }

  const clearAll = () => {
    if (!window.confirm('Remove the PDF job list from this browser? Background jobs will continue running.')) return
    clearStoredJobs()
    setJobs([])
  }

  return (
    <section className="history-panel pdf-jobs-panel" aria-label="PDF translation jobs">
      <div className="history-header">
        <div><span className="workspace-eyebrow">DOCUMENTS</span><h2>PDF translation jobs</h2></div>
        <div className="jobs-header-actions">
        <button type="button" className="upload-primary-btn" onClick={onNewJob}><Plus size={17} aria-hidden="true" /> New PDF job</button>
        {jobs.length > 0 && (
          <button className="icon-btn" onClick={clearAll}>
            <Trash2 size={16} aria-hidden="true" /> Clear all
          </button>
        )}
        </div>
      </div>

      {jobs.length > 0 && <div className="job-list-toolbar">
        <div className="job-filter-tabs" role="group" aria-label="Job status filter">{filters.map(option => <button key={option.value} type="button" className={`icon-btn${filter === option.value ? " is-active" : ""}`} aria-pressed={filter === option.value} onClick={() => setFilter(option.value)}>{option.label}<span className="filter-count">{option.value === "all" ? jobs.length : jobs.filter(job => (statuses[job.job_id] ?? "pending") === option.value).length}</span></button>)}</div>
        <label className="job-search"><Search size={16} aria-hidden="true" /><input type="search" aria-label="Search PDF jobs" value={query} onChange={event => setQuery(event.target.value)} placeholder="Search documents or job IDs" /></label>
      </div>}
      {jobs.length > 0 && visibleCount === 0 && <p className="history-empty">No jobs match the current filters.</p>}
      {jobs.length === 0 ? (
        <div className="history-empty"><FileText size={36} aria-hidden="true" /><span>No PDF jobs yet.</span><button type="button" className="text-action" onClick={onNewJob}>Upload a PDF</button></div>
      ) : (
        <ul className="pdf-job-list">
          {jobs.map(j => (
            <JobRow key={j.job_id} stored={j} onCopy={onToast} onRemove={handleRemove} onStatusChange={updateStatus} onOpenJob={onOpenJob} hidden={!matches(j)} />
          ))}
        </ul>
      )}
    </section>
  )
}
