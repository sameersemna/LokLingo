export function getErrorMessage(err: unknown, fallback: string): string {
  if (err instanceof Error) {
    return err.message || fallback
  }
  if (typeof err === "string" && err.trim()) {
    return err
  }
  return fallback
}

export interface ApiErrorBody {
  error?: unknown
  detail?: unknown
  request_id?: unknown
}

export async function parseApiErrorBody(res: Response): Promise<ApiErrorBody> {
  const raw = await res.text().catch(() => "")
  let parsed: Record<string, unknown> = {}
  if (raw) {
    try {
      parsed = JSON.parse(raw) as Record<string, unknown>
    } catch {
      parsed = {}
    }
  }
  const fallback = raw && !raw.trim().startsWith("<") ? raw.trim() : undefined
  return {
    error: typeof parsed.error === "string" ? parsed.error : fallback,
    detail: typeof parsed.detail === "string" ? parsed.detail : undefined,
    request_id: typeof parsed.request_id === "string" ? parsed.request_id : undefined,
  }
}

export function buildApiError(_status: number, body: ApiErrorBody, fallback: string): Error {
  const msg = typeof body.error === "string" && body.error.trim()
    ? body.error
    : typeof body.detail === "string" && body.detail.trim()
      ? body.detail
      : fallback
  return new Error(msg)
}
