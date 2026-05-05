export interface TranslateRequest {
  text: string
  source: string
  target: string
}

export interface TranslateResponse {
  translated_text: string
  source: string
  target: string
  cached?: boolean
}

// --- async jobs API types ---

export interface JobResponse {
  job_id: string
  status: 'pending' | 'processing' | 'completed' | 'failed'
  translated_text?: string
  source?: string
  target?: string
  error?: string
  processing_method?: 'pdf_text' | 'ocr'
}

const POLL_INTERVAL_MS = 600
const MAX_POLLS = 100 // 60 s timeout

// ---------- shared helpers ----------

function sleep(ms: number): Promise<void> {
  return new Promise(resolve => setTimeout(resolve, ms))
}

function buildTranslateError(status: number, err: { error?: string; request_id?: string }): Error {
  if (status === 502) {
    const requestId = err.request_id ? ` Request ID: ${err.request_id}.` : ''
    return new Error(`Translation upstream failed. Check LiteLLM model/config and backend logs.${requestId}`)
  }
  return new Error(err.error ?? `HTTP ${status}`)
}

/**
 * Fetches a single job snapshot from GET /api/v1/jobs/:id.
 */
export async function fetchJob(jobId: string): Promise<JobResponse> {
  const res = await fetch(`/api/v1/jobs/${jobId}`)
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: 'Unknown error' }))
    throw buildTranslateError(res.status, err)
  }
  return res.json()
}

/**
 * Polls GET /api/v1/jobs/:id until the job reaches a terminal state.
 * Resolves with the completed TranslateResponse or rejects on failure/timeout.
 *
 * @param fallbackSource  source lang to use when job doesn't echo it back
 * @param fallbackTarget  target lang to use when job doesn't echo it back
 * @param failMsg         error message prefix on job failure
 */
async function pollJob(
  jobId: string,
  fallbackSource: string,
  fallbackTarget: string,
  failMsg = 'Translation job failed',
): Promise<TranslateResponse> {
  for (let i = 0; i < MAX_POLLS; i++) {
    await sleep(POLL_INTERVAL_MS)

    const job = await fetchJob(jobId)

    if (job.status === 'completed') {
      return {
        translated_text: job.translated_text ?? '',
        source: job.source ?? fallbackSource,
        target: job.target ?? fallbackTarget,
      }
    }

    if (job.status === 'failed') {
      throw new Error(job.error ?? failMsg)
    }

    // 'pending' | 'processing' → keep polling
  }

  throw new Error('Translation timed out. Please try again.')
}

// ---------- public API ----------

/**
 * Translates text via the async jobs API (POST /api/v1/jobs → poll GET /api/v1/jobs/:id).
 */
export async function translate(req: TranslateRequest): Promise<TranslateResponse> {
  const enqueueRes = await fetch('/api/v1/jobs', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })

  if (!enqueueRes.ok) {
    const err = await enqueueRes.json().catch(() => ({ error: 'Unknown error' }))
    throw buildTranslateError(enqueueRes.status, err)
  }

  const { job_id }: { job_id: string } = await enqueueRes.json()
  return pollJob(job_id, req.source, req.target, 'Translation job failed')
}

export interface UploadPDFResponse {
  job_id: string
}

/**
 * Uploads a PDF file and enqueues a translate_pdf job
 * (POST /api/v1/jobs/pdf).
 */
export async function uploadPDF(
  file: File,
  source: string,
  target: string,
): Promise<UploadPDFResponse> {
  const form = new FormData()
  form.append('file', file)
  form.append('source', source || 'auto')
  form.append('target', target)

  const enqueueRes = await fetch('/api/v1/jobs/pdf', {
    method: 'POST',
    body: form,
  })

  if (!enqueueRes.ok) {
    const err = await enqueueRes.json().catch(() => ({ error: 'Unknown error' }))
    throw buildTranslateError(enqueueRes.status, err)
  }

  return enqueueRes.json()
}

/**
 * Uploads a PDF and polls until the translate_pdf job completes.
 */
export async function translatePDF(
  file: File,
  source: string,
  target: string,
): Promise<TranslateResponse> {
  const { job_id } = await uploadPDF(file, source, target)
  return pollJob(job_id, source, target, 'PDF translation job failed')
}
