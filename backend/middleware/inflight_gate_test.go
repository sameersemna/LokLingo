package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

func TestInflightGate_RejectsWhenLimitExceeded(t *testing.T) {
	app := fiber.New()
	app.Use(InflightGate(1, "/limited"))
	app.Post("/limited", func(c *fiber.Ctx) error {
		time.Sleep(60 * time.Millisecond)
		return c.SendStatus(fiber.StatusOK)
	})

	var wg sync.WaitGroup
	statuses := make(chan int, 2)

	call := func() {
		defer wg.Done()
		req := httptest.NewRequest(http.MethodPost, "/limited", nil)
		resp, err := app.Test(req, 5_000)
		if err != nil {
			statuses <- 0
			return
		}
		statuses <- resp.StatusCode
	}

	wg.Add(2)
	go call()
	go call()
	wg.Wait()
	close(statuses)

	ok := 0
	rejected := 0
	for status := range statuses {
		if status == fiber.StatusOK {
			ok++
		}
		if status == fiber.StatusTooManyRequests {
			rejected++
		}
	}
	if ok == 0 || rejected == 0 {
		t.Fatalf("expected one OK and one 429, got ok=%d rejected=%d", ok, rejected)
	}
}
