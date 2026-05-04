const OCR_BASE = '/ocr'

export interface OCRResponse {
  text: string
  confidence: number
}

export async function extractText(file: File): Promise<OCRResponse> {
  const b64 = await fileToBase64(file)
  const res = await fetch(`${OCR_BASE}/ocr`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ image_b64: b64, mime_type: file.type || 'image/png' }),
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ detail: 'OCR failed' }))
    throw new Error(err.detail ?? `HTTP ${res.status}`)
  }
  return res.json()
}

function fileToBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => {
      const result = reader.result as string
      // strip the data URL prefix "data:...;base64,"
      resolve(result.split(',')[1])
    }
    reader.onerror = () => reject(new Error('Failed to read file'))
    reader.readAsDataURL(file)
  })
}
