package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestWriteAPIAuth_DevelopmentWithoutToken_Allows(t *testing.T) {
	app := fiber.New()
	app.Use(WriteAPIAuth("development", ""))
	app.Post("/", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestWriteAPIAuth_ProductionWithoutToken_Blocked(t *testing.T) {
	app := fiber.New()
	app.Use(WriteAPIAuth("production", ""))
	app.Post("/", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}
}

func TestWriteAPIAuth_WithXAPIToken_Allows(t *testing.T) {
	app := fiber.New()
	app.Use(WriteAPIAuth("production", "secret"))
	app.Post("/", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-API-Token", "secret")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestWriteAPIAuth_WithBearer_Allows(t *testing.T) {
	app := fiber.New()
	app.Use(WriteAPIAuth("production", "secret"))
	app.Post("/", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer secret")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestWriteAPIAuth_InvalidToken_Unauthorized(t *testing.T) {
	app := fiber.New()
	app.Use(WriteAPIAuth("production", "secret"))
	app.Post("/", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-API-Token", "wrong")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}
