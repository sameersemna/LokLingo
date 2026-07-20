import { apiFetch, buildInternalHeaders } from "../utils/http"

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

export interface JobResponse {
  job_id: string
  status: "pending" | "processing" | "completed" | "failed"
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
  processing_method?: "pdf_text" | "ocr"
  warnings?: string[]
  ocr_confidence?: number
}

export interface JobProgressUpdate {
  job_id: string
  status: JobResponse["status"]
  stage?: string
  stage_message?: string
  stage_progress?: number
  total_pages?: number
  processed_pages?: number
  source?: string
  target?: string
}

export interface JobEventResponse extends JobResponse {
  event_type?: string
  timestamp?: string
}

const POLL_INTERVAL_MS = 600
const MAX_POLLS = 100
const REQUEST_TIMEOUT_MS = 30_000
const IMAGE_REQUEST_TIMEOUT_MS = 120_000
const IMAGE_MAX_POLLS = Math.ceil(IMAGE_REQUEST_TIMEOUT_MS / POLL_INTERVAL_MS)

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

export async function fetchJob(jobId: string): Promise<JobResponse> {
  return apiFetch<JobResponse>(`/api/v1/jobs/${jobId}`)
}

export function subscribeJobEvents(
  jobId: string,
  onEvent: (event: JobEventResponse) => void,
  onError?: () => void,
): () => void {
  if (typeof window === "undefined" || typeof window.EventSource === "undefined") {
    onError?.()
    return () => {}
  }

  const es = new window.EventSource(`/api/v1/jobs/${jobId}/events`)
  const handler = (evt: MessageEvent<string>) => {
    try {
      const parsed = JSON.parse(evt.data) as JobEventResponse
      onEvent(parsed)
    } catch {
      // Ignore malformed event payloads.
    }
  }

  es.addEventListener("progress", handler as EventListener)
  es.onerror = () => {
    onError?.()
  }

  return () => {
    es.removeEventListener("progress", handler as EventListener)
    es.close()
  }
}

async function pollJob(
  jobId: string,
  fallbackSource: string,
  fallbackTarget: string,
  failMsg = "Translation job failed",
  onProgress?: (job: JobProgressUpdate) => void,
  maxPolls = MAX_POLLS,
): Promise<TranslateResponse> {
  for (let i = 0; i < maxPolls; i++) {
    await sleep(POLL_INTERVAL_MS)

    const job = await fetchJob(jobId)
    onProgress?.(job)

    if (job.status === "completed") {
      return {
        translated_text: job.translated_text ?? "",
        source: job.source ?? fallbackSource,
        target: job.target ?? fallbackTarget,
        image_url: job.image_url,
      }
    }

    if (job.status === "failed") {
      throw new Error(job.error ?? failMsg)
    }
  }

  throw new Error("Translation timed out. Please try again.")
}

export async function translate(req: TranslateRequest): Promise<TranslateResponse> {
  const enqueue = await apiFetch<{ job_id: string }>("/api/v1/jobs", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ ...req, mode: req.mode ?? "overlay" }),
  })
  const { job_id } = enqueue

  const unsubscribe = subscribeJobEvents(job_id, () => {
    // Text workflow currently doesn't expose per-stage UI, but subscribing keeps
    // the transport path consistent with image/pdf jobs and allows easy future wiring.
  })

  try {
    return await pollJob(job_id, req.source, req.target, "Translation job failed")
  } finally {
    unsubscribe()
  }
}

export interface UploadPDFResponse {
  job_id: string
}

export async function uploadPDF(
  file: File,
  source: string,
  target: string,
  mode = "overlay",
): Promise<UploadPDFResponse> {
  const form = new FormData()
  form.append("file", file)
  form.append("source", source || "auto")
  form.append("target", target)
  form.append("mode", mode)

  return apiFetch<UploadPDFResponse>("/api/v1/jobs/pdf", {
    method: "POST",
    body: form,
  })
}

export async function translatePDF(
  file: File,
  source: string,
  target: string,
  mode = "overlay",
): Promise<TranslateResponse> {
  const { job_id } = await uploadPDF(file, source, target, mode)
  return pollJob(job_id, source, target, "PDF translation job failed")
}

export async function translateImage(
  file: File,
  source: string,
  target: string,
  mode = "overlay",
  onProgress?: (job: JobProgressUpdate) => void,
  signal?: AbortSignal,
): Promise<TranslateResponse> {
  const form = new FormData()
  form.append("file", file)
  form.append("source", source || "auto")
  form.append("target", target)
  form.append("mode", mode)

  const enqueue = await apiFetch<{ job_id: string }>("/api/v1/jobs/image", {
    method: "POST",
    body: form,
    signal,
    timeoutMs: IMAGE_REQUEST_TIMEOUT_MS,
  })

  const unsubscribe = subscribeJobEvents(enqueue.job_id, (event) => {
    onProgress?.(event)
  })

  try {
    return await pollJob(enqueue.job_id, source, target, "Image translation job failed", onProgress, IMAGE_MAX_POLLS)
  } finally {
    unsubscribe()
  }
}
