const JOBS_KEY = 'loklingo-pdf-jobs'
const MAX_STORED = 20

export interface StoredPdfJob {
  job_id: string
  filename: string
  source: string
  target: string
  submittedAt: number
  mode?: string
}

export function loadStoredJobs(): StoredPdfJob[] {
  try {
    const raw = JSON.parse(localStorage.getItem(JOBS_KEY) ?? '[]') as Array<Partial<StoredPdfJob>>
    return raw
      .filter(j => typeof j.job_id === 'string')
      .map(j => ({
        job_id: j.job_id as string,
        filename: typeof j.filename === 'string' ? j.filename : 'document.pdf',
        source: typeof j.source === 'string' ? j.source : 'auto',
        target: typeof j.target === 'string' ? j.target : 'en',
        submittedAt: typeof j.submittedAt === 'number' ? j.submittedAt : Date.now(),
        mode: typeof j.mode === 'string' ? j.mode : undefined,
      }))
  } catch {
    return []
  }
}

export function saveStoredJob(job: StoredPdfJob) {
  const existing = loadStoredJobs()
  const next = [job, ...existing.filter(j => j.job_id !== job.job_id)].slice(0, MAX_STORED)
  localStorage.setItem(JOBS_KEY, JSON.stringify(next))
}

export function removeStoredJob(jobId: string) {
  const next = loadStoredJobs().filter(j => j.job_id !== jobId)
  localStorage.setItem(JOBS_KEY, JSON.stringify(next))
}

export function clearStoredJobs() {
  localStorage.removeItem(JOBS_KEY)
}
