package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func newCreateJobApp() *fiber.App {
	h := NewJobsHandler(&mockStore{}, 25*1024*1024)
	app := fiber.New()
	app.Post("/jobs", h.CreateJob)
	return app
}

func TestCreateJob_InvalidMode(t *testing.T) {
	app := newCreateJobApp()

	body, _ := json.Marshal(map[string]string{
		"text":   "hello",
		"source": "en",
		"target": "de",
		"mode":   "invalid",
	})

	req := httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestCreateJob_TrimmedInputsAccepted(t *testing.T) {
	app := newCreateJobApp()

	body, _ := json.Marshal(map[string]string{
		"text":   "  hello  ",
		"source": "  en  ",
		"target": "  de  ",
		"mode":   "  overlay  ",
	})

	req := httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}
}

func TestCreateJob_InvalidLanguageCodes(t *testing.T) {
	app := newCreateJobApp()

	tests := []map[string]string{
		{"text": "hello", "source": "en$", "target": "de", "mode": "overlay"},
		{"text": "hello", "source": "en", "target": "auto", "mode": "overlay"},
	}

	for _, payload := range tests {
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d for payload %#v", resp.StatusCode, payload)
		}
	}
}
