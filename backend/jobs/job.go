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

// Job holds all state for one async translation request.
type Job struct {
	ID             string    `json:"id"`
	Status         Status    `json:"status"`
	Text           string    `json:"text"`
	Source         string    `json:"source"`
	Target         string    `json:"target"`
	TranslatedText string    `json:"translated_text,omitempty"`
	ErrorMsg       string    `json:"error,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
