package handlers

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"loklingo/backend/jobs"
)

// JobsHandler holds dependencies for the async job endpoints.
type JobsHandler struct {
	store jobs.Store
}

// NewJobsHandler constructs a JobsHandler.
func NewJobsHandler(store jobs.Store) *JobsHandler {
	return &JobsHandler{store: store}
}

// CreateJob handles POST /api/v1/jobs.
// It accepts the same body as /api/v1/translate, enqueues the work, and
// immediately returns 202 Accepted with a job_id for polling.
func (h *JobsHandler) CreateJob(c *fiber.Ctx) error {
	var req struct {
		Text   string `json:"text"`
		Source string `json:"source"`
		Target string `json:"target"`
	}
	if err := c.BodyParser(&req); err != nil {
		return errResponse(c, fiber.StatusBadRequest, "invalid request body")
	}
	if req.Text == "" {
		return errResponse(c, fiber.StatusBadRequest, "text is required")
	}
	if req.Target == "" {
		return errResponse(c, fiber.StatusBadRequest, "target is required")
	}
	if req.Source == "" {
		req.Source = "auto"
	}

	now := time.Now()
	job := &jobs.Job{
		ID:        uuid.NewString(),
		Status:    jobs.StatusPending,
		Text:      req.Text,
		Source:    req.Source,
		Target:    req.Target,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := h.store.Enqueue(c.Context(), job); err != nil {
		return errResponse(c, fiber.StatusInternalServerError, "failed to enqueue job")
	}

	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
		"job_id": job.ID,
		"status": job.Status,
	})
}

// GetJob handles GET /api/v1/jobs/:id.
// Returns the current state of the job, including the translated text when complete.
func (h *JobsHandler) GetJob(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return errResponse(c, fiber.StatusBadRequest, "job id is required")
	}

	job, err := h.store.Get(c.Context(), id)
	if errors.Is(err, jobs.ErrNotFound) {
		return errResponse(c, fiber.StatusNotFound, "job not found")
	}
	if err != nil {
		return errResponse(c, fiber.StatusInternalServerError, "failed to retrieve job")
	}

	resp := fiber.Map{
		"job_id":     job.ID,
		"status":     job.Status,
		"source":     job.Source,
		"target":     job.Target,
		"created_at": job.CreatedAt,
		"updated_at": job.UpdatedAt,
	}
	if job.Status == jobs.StatusCompleted {
		resp["translated_text"] = job.TranslatedText
	}
	if job.Status == jobs.StatusFailed {
		resp["error"] = job.ErrorMsg
	}

	return c.JSON(resp)
}
