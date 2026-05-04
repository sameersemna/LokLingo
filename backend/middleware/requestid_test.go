package middleware
package middleware_test

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"loklingo/backend/middleware"
)

func TestRequestID_Generated(t *testing.T) {
	app := fiber.New()
	app.Use(middleware.RequestID())
	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendString(c.Locals("requestID").(string))
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Header.Get(middleware.RequestIDHeader) == "" {
		t.Fatal("expected X-Request-ID header to be set")
	}
}

func TestRequestID_Propagated(t *testing.T) {
	app := fiber.New()
	app.Use(middleware.RequestID())
	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendString(c.Locals("requestID").(string))
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set(middleware.RequestIDHeader, "test-id-123")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if got := resp.Header.Get(middleware.RequestIDHeader); got != "test-id-123" {
		t.Fatalf("expected propagated ID, got %q", got)
	}
}
