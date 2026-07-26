import { useCallback } from "react"
import { getErrorMessage } from "../utils/errors"
import type { Toast } from "./useToasts"

export interface ExportActionsApi {
  handleCopy: (text: string) => Promise<void>
  handleDownload: (url: string, basename: string) => Promise<void>
  handleDownloadPDF: (result: string, resultImageUrl: string | null) => Promise<void>
}

/**
 * Copy/download/PDF-export actions for the translate output panel. Takes
 * the current text/image values as call-time arguments rather than
 * closing over App state, so the hook itself stays stateless.
 */
export function useExportActions(pushToast: (msg: string, type: Toast["type"]) => void): ExportActionsApi {
  const handleCopy = useCallback(async (text: string) => {
    if (!text) return
    try {
      await navigator.clipboard.writeText(text)
      pushToast("Copied to clipboard", "success")
    } catch {
      pushToast("Copy failed — check browser permissions", "error")
    }
  }, [pushToast])

  const handleDownload = useCallback(async (url: string, basename: string) => {
    try {
      const res = await fetch(url)
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      const blob = await res.blob()
      const ext = blob.type === "image/jpeg" ? "jpg"
        : blob.type === "image/webp" ? "webp"
        : blob.type === "image/gif"  ? "gif"
        : "png"
      const blobUrl = URL.createObjectURL(blob)
      const a = document.createElement("a")
      a.href = blobUrl
      a.download = `${basename}.${ext}`
      document.body.appendChild(a)
      a.click()
      document.body.removeChild(a)
      URL.revokeObjectURL(blobUrl)
    } catch (err) {
      pushToast(getErrorMessage(err, "Download failed"), "error")
    }
  }, [pushToast])

  const handleDownloadPDF = useCallback(async (result: string, resultImageUrl: string | null) => {
    if (!resultImageUrl && !result.trim()) {
      pushToast("Nothing to export", "error")
      return
    }

    try {
      const { jsPDF } = await import("jspdf")
      const pdf = new jsPDF({ orientation: "portrait", unit: "pt", format: "a4" })

      if (resultImageUrl) {
        const image = await new Promise<HTMLImageElement>((resolve, reject) => {
          const img = new Image()
          img.crossOrigin = "anonymous"
          img.onload = () => resolve(img)
          img.onerror = () => reject(new Error("Failed to prepare image for PDF export"))
          img.src = resultImageUrl
        })

        const pageWidth = pdf.internal.pageSize.getWidth()
        const pageHeight = pdf.internal.pageSize.getHeight()
        const scale = Math.min((pageWidth - 48) / image.width, (pageHeight - 70) / image.height)
        const drawWidth = image.width * scale
        const drawHeight = image.height * scale
        const x = (pageWidth - drawWidth) / 2
        const y = (pageHeight - drawHeight) / 2
        pdf.addImage(image, "PNG", x, y, drawWidth, drawHeight)
      } else {
        const pageWidth = pdf.internal.pageSize.getWidth()
        const lines = pdf.splitTextToSize(result, pageWidth - 60)
        pdf.setFont("helvetica", "normal")
        pdf.setFontSize(11)
        pdf.text(lines, 30, 40)
      }

      pdf.save("loklingo-export.pdf")
      pushToast("PDF exported", "success")
    } catch (err) {
      pushToast(getErrorMessage(err, "PDF export failed"), "error")
    }
  }, [pushToast])

  return { handleCopy, handleDownload, handleDownloadPDF }
}
