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

interface JobResponse {
  job_id: string
  status: 'pending' | 'processing' | 'completed' | 'failed'
  translated_text?: string
  source?: string
  target?: string
  error?: string
}

const POLL_INTERVAL_MS = 600
const MAX_POLLS = 100 // 60 s timeout

/**
 * Translates text via the async jobs API (POST /api/v1/jobs → poll GET /api/v1/jobs/:id).
 * Falls back to an error if the job fails or times out.
 */
export async function translate(req: TranslateRequest): Promise<TranslateResponse> {
  // 1. Enqueue
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

  // 2. Poll until done
  for (let i = 0; i < MAX_POLLS; i++) {
    await sleep(POLL_INTERVAL_MS)

    const pollRes = await fetch(`/api/v1/jobs/${job_id}`)
    if (!pollRes.ok) {
      const err = await pollRes.json().catch(() => ({ error: 'Unknown error' }))
      throw buildTranslateError(pollRes.status, err)
    }

    const job: JobResponse = await pollRes.json()

    if (job.status === 'completed') {
      return {
        translated_text: job.translated_text ?? '',
        source: job.source ?? req.source,
        target: job.target ?? req.target,
      }
    }

    if (job.status === 'failed') {
      throw new Error(job.error ?? 'Translation job failed')
    }

    // 'pending' | 'processing' → keep polling
  }

  throw new Error('Translation timed out. Please try again.')
}

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
