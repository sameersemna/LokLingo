package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"loklingo/backend/handlers"
)

func TestReadinessHandler_OK(t *testing.T) {
	app := fiber.New()
	app.Get("/ready", handlers.NewReadinessHandlerWithChecks(map[string]func(context.Context) error{
		"config": func(context.Context) error { return nil },
		"redis":  func(context.Context) error { return nil },
	}).Ready)

	resp, err := app.Test(httptest.NewRequest("GET", "/ready", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		Status       string                       `json:"status"`
		Service      string                       `json:"service"`
		Dependencies map[string]map[string]string `json:"dependencies"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "ok" {
		t.Fatalf("expected ok status, got %q", body.Status)
	}
	if body.Service != "loklingo-backend" {
		t.Fatalf("expected service=loklingo-backend, got %q", body.Service)
	}
	if body.Dependencies["config"]["status"] != "ok" {
		t.Fatalf("expected config dependency ok, got %#v", body.Dependencies["config"])
	}
	if body.Dependencies["redis"]["status"] != "ok" {
		t.Fatalf("expected redis dependency ok, got %#v", body.Dependencies["redis"])
	}
}

func TestReadinessHandlerStatusCodes(t *testing.T) {
	app := fiber.New()
	app.Get("/ready", handlers.NewReadinessHandlerWithChecks(map[string]func(context.Context) error{
		"config": func(context.Context) error { return nil },
		"redis":  func(context.Context) error { return errors.New("redis unavailable") },
	}).Ready)

	resp, err := app.Test(httptest.NewRequest("GET", "/ready", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}

	var body struct {
		Status       string                       `json:"status"`
		Service      string                       `json:"service"`
		Dependencies map[string]map[string]string `json:"dependencies"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "degraded" {
		t.Fatalf("expected degraded status, got %q", body.Status)
	}
	if body.Service != "loklingo-backend" {
		t.Fatalf("expected service=loklingo-backend, got %q", body.Service)
	}
	if body.Dependencies["redis"]["status"] != "error" {
		t.Fatalf("expected redis dependency error, got %#v", body.Dependencies["redis"])
	}
	if body.Dependencies["redis"]["error"] == "" {
		t.Fatal("expected non-empty error message for failed redis dependency")
	}
	if body.Dependencies["config"]["status"] != "ok" {
		t.Fatalf("expected config ok in degraded response, got %#v", body.Dependencies["config"])
	}
}
