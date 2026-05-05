package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestInternalToken_EmptyToken_AllowsAll(t *testing.T) {
	app := fiber.New()
	app.Use(InternalToken(""))
	app.Get("/", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestInternalToken_CorrectToken_Allows(t *testing.T) {
	app := fiber.New()
	app.Use(InternalToken("secret"))
	app.Get("/", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Internal-Token", "secret")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestInternalToken_WrongToken_Returns401(t *testing.T) {
	app := fiber.New()
	app.Use(InternalToken("secret"))
	app.Get("/", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Internal-Token", "wrong")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestInternalToken_MissingToken_Returns401(t *testing.T) {
	app := fiber.New()
	app.Use(InternalToken("secret"))
	app.Get("/", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	// No X-Internal-Token header
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}
