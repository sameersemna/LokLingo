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
	TypeText  JobType = "text"
	TypePDF   JobType = "translate_pdf"
	TypeImage JobType = "translate_image"
)

// Mode controls how translated content is rendered/applied.
type Mode = string

const (
	ModeOverlay Mode = "overlay"
	ModeLayout  Mode = "layout"
	ModeOCROnly Mode = "ocr_only"
)

// DefaultMode is used when no mode is specified in a job request.
const DefaultMode Mode = ModeOverlay

// Job holds all state for one async translation request.
type Job struct {
	ID             string    `json:"id"`
	Status         Status    `json:"status"`
	Type           JobType   `json:"type,omitempty"`
	Mode           Mode      `json:"mode"`
	Attempt        int       `json:"attempt,omitempty"`
	MaxAttempts    int       `json:"max_attempts,omitempty"`
	Text           string    `json:"text"`
	Source         string    `json:"source"`
	Target         string    `json:"target"`
	TranslatedText string    `json:"translated_text,omitempty"`
	OutputFilePath string    `json:"output_file_path,omitempty"`
	ErrorMsg       string    `json:"error,omitempty"`
	LastErrorMsg   string    `json:"last_error,omitempty"`
	NextRetryAt    time.Time `json:"next_retry_at,omitempty"`
	DeadLetteredAt time.Time `json:"dead_lettered_at,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`

	// File-job-specific fields (PDF and image jobs).
	FilePath         string `json:"file_path,omitempty"`         // path to the uploaded file on the worker's filesystem
	Lang             string `json:"lang,omitempty"`              // OCR language hint (ISO 639-1 or "auto")
	ProcessingMethod string `json:"processing_method,omitempty"` // "pdf_text" or "ocr"
	TotalPages       int    `json:"total_pages,omitempty"`       // total translatable pages discovered by the worker
	ProcessedPages   int    `json:"processed_pages,omitempty"`   // pages translated so far

	// Image rendering options (translate_image jobs only).
	JPEGQuality int `json:"jpeg_quality,omitempty"` // output JPEG quality [1,100]; 0 → default (90)
	BgAlpha     int `json:"bg_alpha"`               // background opacity [0,255]; -1 → default (220)
	TextPadding int `json:"text_padding"`           // padding between box edge and text (px); -1 → default (6), 0 → flush to edge
}
