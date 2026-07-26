import type { TextBlock } from "../api/ocr"
import type { JobProgressUpdate } from "../api/translate"
import type { ProgressStage } from "../constants"

export function isRenderableOCRBlock(block: TextBlock): boolean {
  const [x1, y1, x2, y2] = block.bbox
  return Number.isFinite(x1) && Number.isFinite(y1) && Number.isFinite(x2) && Number.isFinite(y2) && Math.abs(x2 - x1) >= 4 && Math.abs(y2 - y1) >= 4
}

export function mapTranslatedLinesToBlocks(blocks: TextBlock[], translatedText: string): string[] {
  const translatedLines = translatedText
    .split("\n")
    .map(line => line.trim())
    .filter(Boolean)

  const mapped = new Array<string>(blocks.length).fill("")
  let lineIdx = 0

  for (let i = 0; i < blocks.length; i++) {
    const source = blocks[i].text.trim()
    if (!source) {
      mapped[i] = ""
      continue
    }
    mapped[i] = translatedLines[lineIdx] ?? source
    lineIdx += 1
  }

  return mapped
}

export function formatFileSize(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B"
  if (bytes < 1024) return `${bytes} B`
  const kb = bytes / 1024
  if (kb < 1024) return `${kb.toFixed(kb >= 100 ? 0 : 1)} KB`
  const mb = kb / 1024
  return `${mb.toFixed(mb >= 100 ? 0 : 1)} MB`
}

export function hasRTLText(text: string): boolean {
  return /[\u0590-\u08FF]/.test(text)
}

export function hasVerticalTypography(blocks: TextBlock[]): boolean {
  return blocks.some(block => {
    const [x1, y1, x2, y2] = block.bbox
    const width = Math.abs(x2 - x1)
    const height = Math.abs(y2 - y1)
    return height > width * 1.35
  })
}

export function getPipelineHelpMessage(err: unknown): string | null {
  const raw = err instanceof Error ? err.message : String(err ?? "")
  const message = raw.toLowerCase()

  if (message.includes("unsupported api version")) {
    return "Backend API version mismatch. Update the frontend/backend pair or restart services with matching versions."
  }
  if (message.includes("failed to fetch") || message.includes("networkerror") || message.includes("network error")) {
    return "Cannot reach translation services right now. Check backend containers and network connectivity."
  }
  if (message.includes("http 5")) {
    return "Translation backend returned a server error. Retry in a moment or inspect backend logs."
  }

  return null
}

export function mapBackendImageStage(stage?: string): ProgressStage | undefined {
  switch ((stage ?? "").toLowerCase()) {
    case "detecting_text":
      return "detecting"
    case "understanding_layout":
      return "layout"
    case "detecting_languages":
      return "languages"
    case "translating":
      return "translating"
    case "retrying":
      return "translating"
    case "fallback_provider":
      return "translating"
    case "rebuilding_layout":
      return "typography"
    case "rendering":
      return "rendering"
    case "completed":
      return "rendering"
    default:
      return undefined
  }
}

export function mapBackendStageNarrative(job: JobProgressUpdate): string | null {
  const stage = (job.stage ?? "").toLowerCase()
  const message = (job.stage_message ?? "").toLowerCase()

  if (stage === "detecting_text") return "Detecting text regions"
  if (stage === "understanding_layout") return "Preserving layout geometry"
  if (stage === "detecting_languages") return "Detecting languages and script direction"
  if (stage === "translating") return "Translating content"
  if (stage === "rebuilding_layout") return "Rebuilding typography"
  if (stage === "rendering") return "Rendering final image"

  if (message.includes("layout")) return "Preserving layout geometry"
  if (message.includes("language")) return "Detecting languages and script direction"
  if (message.includes("translat")) return "Translating content"
  if (message.includes("render")) return "Rendering final image"
  if (message.includes("typography") || message.includes("rebuild")) return "Rebuilding typography"

  return null
}

export function mapBackendReliabilityHint(job: JobProgressUpdate): string | null {
  const stage = (job.stage ?? "").toLowerCase()
  const message = (job.stage_message ?? "").toLowerCase()
  if (stage === "retrying" || message.includes("retry")) {
    return "Retrying unstable region..."
  }
  if (stage === "fallback_provider" || message.includes("backup") || message.includes("switching")) {
    return "Switching rendering strategy..."
  }
  return null
}
