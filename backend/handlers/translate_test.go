package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"loklingo/backend/jobs"
	"loklingo/backend/services"
)

type mockTranslationService struct {
	result string
	err    error
	input  services.TranslationInput
}

func (m *mockTranslationService) Translate(input services.TranslationInput) (string, error) {
	m.input = input
	if m.err != nil {
		return "", m.err
	}
	return m.result, nil
}

// mockStore is a minimal no-op jobs.Store for handler tests.
type mockStore struct{}

func (s *mockStore) Enqueue(_ context.Context, _ *jobs.Job) error       { return nil }
func (s *mockStore) Get(_ context.Context, _ string) (*jobs.Job, error) { return nil, jobs.ErrNotFound }
func (s *mockStore) Update(_ context.Context, _ *jobs.Job) error        { return nil }
func (s *mockStore) Dequeue(_ context.Context) (*jobs.Job, error)       { return nil, nil }
func (s *mockStore) GetCached(_ context.Context, _, _, _ string) (string, error) {
	return "", jobs.ErrNotFound
}
func (s *mockStore) SetCached(_ context.Context, _, _, _, _ string) error { return nil }

func TestTranslateHandler_Success(t *testing.T) {
	mockSvc := &mockTranslationService{result: "Hallo Welt"}
	h := NewTranslateHandler(mockSvc, &mockStore{})

	app := fiber.New()
	app.Post("/translate", h.Translate)

	payload := map[string]string{
		"text":   "Hello world",
		"source": "en",
		"target": "de",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/translate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	if mockSvc.input.Text != "Hello world" || mockSvc.input.Source != "en" || mockSvc.input.Target != "de" {
		t.Fatalf("unexpected service input: %+v", mockSvc.input)
	}
}

func TestTranslateHandler_Validation(t *testing.T) {
	mockSvc := &mockTranslationService{result: "x"}
	h := NewTranslateHandler(mockSvc, &mockStore{})

	app := fiber.New()
	app.Post("/translate", h.Translate)

	tests := []struct {
		name string
		body map[string]string
	}{
		{name: "missing text", body: map[string]string{"target": "de"}},
		{name: "missing target", body: map[string]string{"text": "hello"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, _ := json.Marshal(tc.body)
			req := httptest.NewRequest(http.MethodPost, "/translate", bytes.NewReader(b))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d", resp.StatusCode)
			}
		})
	}
}

func TestTranslateHandler_UpstreamFailure(t *testing.T) {
	mockSvc := &mockTranslationService{err: errors.New("upstream failed")}
	h := NewTranslateHandler(mockSvc, &mockStore{})

	app := fiber.New()
	app.Post("/translate", h.Translate)

	payload := map[string]string{
		"text":   "Hello world",
		"source": "en",
		"target": "de",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/translate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", resp.StatusCode)
	}
}

func TestTranslateHandler_TextTooLong(t *testing.T) {
	mockSvc := &mockTranslationService{result: "x"}
	h := NewTranslateHandler(mockSvc, &mockStore{})

	app := fiber.New()
	app.Post("/translate", h.Translate)

	payload := map[string]string{
		"text":   strings.Repeat("a", maxTranslateTextChars+1),
		"source": "en",
		"target": "de",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/translate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", resp.StatusCode)
	}
}

func TestTranslateHandler_InvalidLanguageCodes(t *testing.T) {
	mockSvc := &mockTranslationService{result: "x"}
	h := NewTranslateHandler(mockSvc, &mockStore{})

	app := fiber.New()
	app.Post("/translate", h.Translate)

	tests := []map[string]string{
		{"text": "hello", "source": "en$", "target": "de"},
		{"text": "hello", "source": "en", "target": "auto"},
		{"text": "hello", "source": "en", "target": "@@"},
	}

	for _, payload := range tests {
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/translate", bytes.NewReader(body))
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
