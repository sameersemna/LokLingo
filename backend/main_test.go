package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"loklingo/backend/middleware"

	"github.com/gofiber/fiber/v2"
)

func TestRegisterWriteRoute_DoesNotGuardSiblingRoutes(t *testing.T) {
	app := fiber.New()
	api := app.Group("/api/v1")
	ok := func(ctx *fiber.Ctx) error { return ctx.SendStatus(http.StatusOK) }
	registerWriteRoute(api, "/jobs", []fiber.Handler{middleware.WriteAPIAuth("production", "write-token")}, ok)
	api.Get("/jobs/id", ok)
	api.Get("/metrics", middleware.InternalToken("internal-token"), ok)
	api.Post("/jobs/id/replay", middleware.InternalToken("internal-token"), ok)
	cases := []struct {
		name          string
		method        string
		path          string
		writeToken    string
		internalToken string
		wantStatus    int
	}{
		{name: "public job status", method: http.MethodGet, path: "/api/v1/jobs/id", wantStatus: 200},
		{name: "internal token alone", method: http.MethodGet, path: "/api/v1/metrics", internalToken: "internal-token", wantStatus: 200},
		{name: "internal route rejects missing token", method: http.MethodGet, path: "/api/v1/metrics", wantStatus: 401},
		{name: "write token cannot replace internal token", method: http.MethodGet, path: "/api/v1/metrics", writeToken: "write-token", wantStatus: 401},
		{name: "replay uses internal token", method: http.MethodPost, path: "/api/v1/jobs/id/replay", internalToken: "internal-token", wantStatus: 200},
		{name: "write rejects missing token", method: http.MethodPost, path: "/api/v1/jobs", wantStatus: 401},
		{name: "write accepts write token", method: http.MethodPost, path: "/api/v1/jobs", writeToken: "write-token", wantStatus: 200},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(testCase.method, testCase.path, nil)
			request.Header.Set("X-API-Token", testCase.writeToken)
			request.Header.Set("X-Internal-Token", testCase.internalToken)
			response, err := app.Test(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != testCase.wantStatus {
				t.Fatalf("got status %d, want %d", response.StatusCode, testCase.wantStatus)
			}
		})
	}
}
