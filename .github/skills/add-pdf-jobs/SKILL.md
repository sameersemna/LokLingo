---
name: add-pdf-jobs
description: >
  Extend LokLingo backend to support async translate_pdf jobs using file uploads
  and Go-native PDF text extraction (no OCR). Use when: adding /api/v1/jobs/pdf,
  extending Job for translate_pdf metadata, dispatching translate_pdf in worker,
  and integrating frontend upload API. DO NOT use to rewrite worker/store, modify
  TranslationService internals, or change existing endpoints behavior.
argument-hint: "Describe what PDF job behaviour you want to add or fix"
---

# Add PDF Translation Jobs to LokLingo

This skill extends LokLingo async jobs to support `translate_pdf` while reusing
the existing queue + worker + TranslationService pipeline.

## Required Behavior

1. Add a new job type `translate_pdf`.
2. Add `POST /api/v1/jobs/pdf` endpoint that accepts `multipart/form-data`.
3. Save uploaded PDFs to `/tmp/loklingo` and enqueue only (no processing in handler).
4. In worker, for `translate_pdf` jobs:
   - read `file_path`
   - extract text via Go PDF service (no OCR)
   - call existing `TranslationService`
   - store translated text and status
5. Keep existing text-job flow unchanged.

## Constraints

- Do not rewrite `jobs.Store`, queue semantics, or worker run loop.
- Do not duplicate translation logic.
- Do not modify existing endpoint contracts except by adding `/api/v1/jobs/pdf`.
- Do not introduce OCR in this flow.

## Implementation Steps

1. Extend [backend/jobs/job.go](../../../backend/jobs/job.go)
   - Add `JobType` and `TypePDF = "translate_pdf"`
   - Add PDF metadata fields: `Type`, `FilePath` (and optional language hint)

2. Add reusable PDF extractor service in [backend/internal/services/pdf_service.go](../../../backend/internal/services/pdf_service.go)
   - Use Go PDF library (`github.com/ledongthuc/pdf`)
   - Expose `PDFService` interface + `ExtractText(filePath string) (string, error)`
   - Return clean errors for invalid PDFs / empty extracted text

3. Extend worker in [backend/jobs/worker.go](../../../backend/jobs/worker.go)
   - Inject `PDFService` dependency
   - For `TypePDF`, extract text then pass it into existing `TranslationService`
   - Mark job `failed` on extraction errors, `completed` on success

4. Add enqueue endpoint in [backend/handlers/jobs.go](../../../backend/handlers/jobs.go)
   - `POST /api/v1/jobs/pdf`
   - Accept `file` upload + `source` + `target`
   - Save file to `/tmp/loklingo/<uuid>.pdf`
   - Enqueue job with `TypePDF` and `FilePath`
   - Return `job_id`

5. Wire dependencies in [backend/main.go](../../../backend/main.go)
   - Construct PDF service and pass to worker
   - Register `/api/v1/jobs/pdf`

6. Extend frontend API only (no UI)
   - Add `uploadPDF(file, source, target)` in [frontend/src/api/translate.ts](../../../frontend/src/api/translate.ts)
   - Call `POST /api/v1/jobs/pdf` and return `job_id`

## Validation Checklist

- `go build ./...` and `go test ./...` pass in `backend/`
- `npm run build` passes in `frontend/`
- `POST /api/v1/jobs/pdf` returns `job_id`
- Polling `GET /api/v1/jobs/:id` shows `pending -> processing -> completed|failed`
- Existing `POST /api/v1/jobs` text flow still works unchanged
