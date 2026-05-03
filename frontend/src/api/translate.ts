const API_BASE = '/api/v1'

export interface TranslateRequest {
  source_text: string
  source_lang: string
  target_lang: string
}

export interface TranslateResponse {
  translated_text: string
  source_lang: string
  target_lang: string
}

export async function translate(req: TranslateRequest): Promise<TranslateResponse> {
  const res = await fetch(`${API_BASE}/translate`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })

  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: 'Unknown error' }))
    throw new Error(err.error ?? `HTTP ${res.status}`)
  }

  return res.json()
}
