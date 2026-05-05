package jobs

import "time"

// Status represents the lifecycle state of an async translation job.
type Status string

const (
	StatusPending    Status = "pending"
	StatusProcessing Status = "processing"
	StatusCompleted  Status = "completed"
	StatusFailed     Status = "failed"
)

// JobType distinguishes the kind of work a job represents.
type JobType = string

const (
	TypeText JobType = "text"
	TypePDF  JobType = "translate_pdf"
)

// Job holds all state for one async translation request.
type Job struct {
	ID             string    `json:"id"`
	Status         Status    `json:"status"`
	Type           JobType   `json:"type,omitempty"`
	Text           string    `json:"text"`
	Source         string    `json:"source"`
	Target         string    `json:"target"`
	TranslatedText string    `json:"translated_text,omitempty"`
	ErrorMsg       string    `json:"error,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`

	// PDF-job-specific fields.
	FilePath         string `json:"file_path,omitempty"`         // path to the PDF on the worker's filesystem
	Lang             string `json:"lang,omitempty"`              // OCR language hint (ISO 639-1 or "auto")
	ProcessingMethod string `json:"processing_method,omitempty"` // "pdf_text" or "ocr"
}
