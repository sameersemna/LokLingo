package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestSecurityHeaders_SetsDefensiveHeaders(t *testing.T) {
	app := fiber.New()
	app.Use(SecurityHeaders())
	app.Get("/api/v1/jobs/x", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/x", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	checks := map[string]string{
		"X-Content-Type-Options":        "nosniff",
		"X-Frame-Options":               "DENY",
		"Referrer-Policy":               "no-referrer",
		"Permissions-Policy":            "camera=(), microphone=(), geolocation=()",
		"Cross-Origin-Resource-Policy":  "same-site",
		"Cache-Control":                 "no-store",
	}
	for header, want := range checks {
		if got := resp.Header.Get(header); got != want {
			t.Fatalf("%s = %q, want %q", header, got, want)
		}
	}
}

func TestSecurityHeaders_HealthSkipsNoStore(t *testing.T) {
	app := fiber.New()
	app.Use(SecurityHeaders())
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if got := resp.Header.Get("Cache-Control"); got == "no-store" {
		t.Fatalf("health should not force no-store, got %q", got)
	}
	if got := resp.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("expected nosniff on health, got %q", got)
	}
}
