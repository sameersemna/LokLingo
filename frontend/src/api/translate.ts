export interface TranslateRequest {
  text: string
  source: string
  target: string
}

export interface TranslateResponse {
  translated_text: string
  source: string
  target: string
}

export async function translate(req: TranslateRequest): Promise<TranslateResponse> {
  const payload = {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  }

  // Prefer the root translate endpoint and gracefully fall back to versioned route.
  let res = await fetch('/translate', payload)
  if (res.status === 404) {
    res = await fetch('/api/v1/translate', payload)
  }

  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: 'Unknown error' }))
    throw new Error(err.error ?? `HTTP ${res.status}`)
  }

  return res.json()
}
