import { useEffect, useState } from "react"
import { ArrowLeft, Copy, RefreshCw, Download, FileText } from "lucide-react"
import { fetchJob, type JobResponse } from "../api/translate"
import { ApiError, getErrorMessage } from "../utils/errors"

interface JobDetailPanelProps {
  jobId: string
  onBack: () => void
  onCopy: (text: string) => void
  onDownload: (url: string, name: string) => void
  onExportPDF: (text: string, imageUrl: string | null) => void
}

export function JobDetailPanel({ jobId, onBack, onCopy, onDownload, onExportPDF }: JobDetailPanelProps) {
  const [job, setJob] = useState<JobResponse | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [refresh, setRefresh] = useState(0)

  useEffect(() => {
    let cancelled = false
    let timer: ReturnType<typeof setTimeout> | undefined
    const load = async () => {
      try {
        const response = await fetchJob(jobId)
        if (cancelled) return
        setJob(response)
        setError(null)
        if (response.status === "pending" || response.status === "processing") timer = setTimeout(() => void load(), 1800)
      } catch (failure) {
        if (cancelled) return
        setError(failure instanceof ApiError && failure.status === 404 ? "Job not found or expired." : getErrorMessage(failure, "Unable to load this job."))
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    void load()
    return () => { cancelled = true; clearTimeout(timer) }
  }, [jobId, refresh])

  const statusLabels = { pending: "Queued", processing: "Processing", completed: "Completed", failed: "Failed" }
  return <section className="job-detail-panel" aria-label="Job details">
    <div className="history-header"><div><button type="button" className="text-action" onClick={onBack}><ArrowLeft size={15} aria-hidden="true" /> PDF Jobs</button><h2>Job details</h2></div><button type="button" className="icon-btn" onClick={() => setRefresh(value => value + 1)} disabled={loading}><RefreshCw size={16} aria-hidden="true" /> Refresh</button></div>
    <div className="job-detail-meta"><code>{jobId}</code><button type="button" className="icon-btn" onClick={() => onCopy(jobId)}><Copy size={15} aria-hidden="true" /> Copy job ID</button></div>
    {loading && <p className="panel-loading" role="status">Loading job...</p>}
    {error && <div className="inline-error" role="alert">{error}</div>}
    {job && <>
      <div className="job-detail-summary"><span className={`pdf-job-badge pdf-job-${job.status === "completed" ? "done" : job.status}`}>{statusLabels[job.status]}</span><span>{job.source} → {job.target}</span>{typeof job.processed_pages === "number" && typeof job.total_pages === "number" && <span>{job.processed_pages} / {job.total_pages} pages</span>}{job.stage_message && <span>{job.stage_message}</span>}</div>
      {job.error && <p className="inline-error" role="alert">{job.error}</p>}
      {job.warnings?.map((warning, index) => <p key={index} className="job-detail-warning" role="status">{warning}</p>)}
      {job.translated_text && <section className="job-detail-output"><div className="editor-heading"><h3>Translated text</h3><div className="settings-actions"><button type="button" className="icon-btn" onClick={() => onCopy(job.translated_text!)}><Copy size={15} aria-hidden="true" /> Copy text</button><button type="button" className="icon-btn" onClick={() => onExportPDF(job.translated_text!, job.image_url ?? null)}><FileText size={15} aria-hidden="true" /> PDF</button></div></div><div className="result-text" dir="auto">{job.translated_text}</div></section>}
      {job.image_url && <section className="job-detail-image"><img src={job.image_url} alt="Rendered job output" /><button type="button" className="icon-btn" onClick={() => onDownload(job.image_url!, "loklingo-job-output")}><Download size={15} aria-hidden="true" /> Download image</button></section>}
    </>}
  </section>
}