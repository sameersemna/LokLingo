export interface TranslateRequest {
  text: string
  source: string
  target: string
  mode?: string
}

export interface TranslateResponse {
  translated_text: string
  source: string
  target: string
  image_url?: string
  cached?: boolean
}

// --- async jobs API types ---

export interface JobResponse {
  job_id: string
  status: 'pending' | 'processing' | 'completed' | 'failed'
  translated_text?: string
  image_url?: string
  source?: string
  target?: string
  error?: string
  stage?: string
  stage_message?: string
  stage_progress?: number
  total_pages?: number
  processed_pages?: number
  processing_method?: 'pdf_text' | 'ocr'
}

const POLL_INTERVAL_MS = 600
const MAX_POLLS = 100 // 60 s timeout
const REQUEST_TIMEOUT_MS = 30_000
const IMAGE_REQUEST_TIMEOUT_MS = 120_000

// ---------- shared helpers ----------

function sleep(ms: number): Promise<void> {
  return new Promise(resolve => setTimeout(resolve, ms))
}

function buildTranslateError(status: number, err: { error?: string; request_id?: string }): Error {
  if (status === 502) {
    const requestId = err.request_id ? ` Request ID: ${err.request_id}.` : ''
    return new Error(`Translation upstream failed. Check LiteLLM model/config and backend logs.${requestId}`)
  }
  if (status === 504) {
    return new Error('Image translation timed out. Please retry or use OCR only mode for faster results.')
  }
  return new Error(err.error ?? `HTTP ${status}`)
}

async function parseErrorBody(res: Response): Promise<{ error?: string; request_id?: string }> {
  const raw = await res.text().catch(() => '')
  let parsed: { error?: unknown; request_id?: unknown } = {}
  if (raw) {
    try {
      parsed = JSON.parse(raw) as { error?: unknown; request_id?: unknown }
    } catch {
      parsed = {}
    }
  }
  const fallback = raw && !raw.trim().startsWith('<') ? raw.trim() : undefined
  return {
    error: typeof parsed.error === 'string' ? parsed.error : fallback,
    request_id: typeof parsed.request_id === 'string' ? parsed.request_id : undefined,
  }
}

async function fetchWithTimeout(input: string, init?: RequestInit, timeoutMs = REQUEST_TIMEOUT_MS): Promise<Response> {
  const controller = new AbortController()
  const timeoutId = setTimeout(() => controller.abort(), timeoutMs)
  try {
    return await fetch(input, { ...init, signal: controller.signal })
  } catch (err) {
    if (err instanceof DOMException && err.name === 'AbortError') {
      throw new Error('Request timed out. Please try again.', { cause: err })
    }
    if (err instanceof Error) {
      throw new Error('Network request failed.', { cause: err })
    }
    throw new Error('Network request failed.', { cause: err })
  } finally {
    clearTimeout(timeoutId)
  }
}

async function fetchJsonOrThrow<T>(input: string, init?: RequestInit, timeoutMs = REQUEST_TIMEOUT_MS): Promise<T> {
  const res = await fetchWithTimeout(input, init, timeoutMs)
  if (!res.ok) {
    const err = await parseErrorBody(res)
    throw buildTranslateError(res.status, err)
  }
  return res.json() as Promise<T>
}

/**
 * Fetches a single job snapshot from GET /api/v1/jobs/:id.
 */
export async function fetchJob(jobId: string): Promise<JobResponse> {
  return fetchJsonOrThrow<JobResponse>(`/api/v1/jobs/${jobId}`)
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
        image_url: job.image_url,
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
  const enqueue = await fetchJsonOrThrow<{ job_id: string }>('/api/v1/jobs', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ ...req, mode: req.mode ?? 'overlay' }),
  })
  const { job_id } = enqueue
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
  mode = 'overlay',
): Promise<UploadPDFResponse> {
  const form = new FormData()
  form.append('file', file)
  form.append('source', source || 'auto')
  form.append('target', target)
  form.append('mode', mode)

  return fetchJsonOrThrow<UploadPDFResponse>('/api/v1/jobs/pdf', {
    method: 'POST',
    body: form,
  })
}


/**
 * Uploads a PDF and polls until the translate_pdf job completes.
 */
export async function translatePDF(
  file: File,
  source: string,
  target: string,
  mode = 'overlay',
): Promise<TranslateResponse> {
  const { job_id } = await uploadPDF(file, source, target, mode)
  return pollJob(job_id, source, target, 'PDF translation job failed')
}

/**
 * Uploads an image and returns rendered output metadata.
 * Endpoint: POST /api/v1/translate/image
 */
export async function translateImage(
  file: File,
  source: string,
  target: string,
  mode = 'overlay',
): Promise<TranslateResponse> {
  const form = new FormData()
  form.append('file', file)
  form.append('source', source || 'auto')
  form.append('target', target)
  form.append('mode', mode)

  const data = await fetchJsonOrThrow<{
    image_url?: string
    translated_text?: string
    source?: string
    target?: string
  }>('/api/v1/translate/image', {
    method: 'POST',
    body: form,
  }, IMAGE_REQUEST_TIMEOUT_MS)
  return {
    translated_text: data.translated_text ?? '',
    source: data.source ?? source,
    target: data.target ?? target,
    image_url: data.image_url,
  }
}
