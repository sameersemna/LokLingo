import { apiFetch } from "../utils/http"

export interface TextBlock {
  text: string
  confidence: number
  reading_order: number
  bbox: [number, number, number, number]
}

export interface OCRImageResponse {
  text: string
  confidence: number
  blocks: TextBlock[]
  debug_image_path?: string | null
  exported_json_path?: string | null
}

export interface PageResult {
  page_number: number
  text: string
  confidence: number
  blocks: TextBlock[]
  debug_image_path?: string | null
}

export interface OCRPdfResponse {
  text: string
  confidence: number
  pages: PageResult[]
  exported_json_path?: string | null
}

export async function extractTextFromImage(
  file: File,
  lang = "auto",
): Promise<OCRImageResponse> {
  const b64 = await fileToBase64(file)
  return apiFetch<OCRImageResponse>("/ocr/image", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ image_b64: b64, mime_type: file.type || "image/png", lang }),
    timeoutMs: 30_000,
  })
}

export async function extractTextFromPdf(
  file: File,
  lang = "auto",
  dpi = 200,
): Promise<OCRPdfResponse> {
  const b64 = await fileToBase64(file)
  return apiFetch<OCRPdfResponse>("/ocr/pdf", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ pdf_b64: b64, lang, dpi }),
    timeoutMs: 60_000,
  })
}

export async function extractText(file: File, lang = "auto"): Promise<{ text: string; confidence: number }> {
  if (file.type === "application/pdf") {
    const result = await extractTextFromPdf(file, lang)
    return { text: result.text, confidence: result.confidence }
  }
  return extractTextFromImage(file, lang)
}

function fileToBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => {
      const result = reader.result as string
      resolve(result.split(",")[1])
    }
    reader.onerror = () => reject(new Error("Failed to read file"))
    reader.readAsDataURL(file)
  })
}
