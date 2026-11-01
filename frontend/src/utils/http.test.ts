import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { apiFetch, buildInternalHeaders, resolveInternalToken } from "./http"
import { ApiError } from "./errors"
import { saveSessionCredentials, STORAGE_KEYS } from "./storage"

describe("session credential HTTP boundaries", () => {
  beforeEach(() => {
    localStorage.clear()
    sessionStorage.clear()
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ ok: true }))))
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.unstubAllEnvs()
  })

  it("stores credentials only in the session and removes legacy persistent credentials", () => {
    localStorage.setItem(STORAGE_KEYS.INTERNAL_TOKEN, "legacy-test-token")
    saveSessionCredentials(" write-test-token ", " operator-test-token ")
    expect(sessionStorage.getItem(STORAGE_KEYS.WRITE_TOKEN)).toBe("write-test-token")
    expect(sessionStorage.getItem(STORAGE_KEYS.INTERNAL_TOKEN)).toBe("operator-test-token")
    expect(localStorage.getItem(STORAGE_KEYS.INTERNAL_TOKEN)).toBeNull()
    saveSessionCredentials("", "")
    expect(sessionStorage.getItem(STORAGE_KEYS.WRITE_TOKEN)).toBeNull()
    expect(resolveInternalToken()).toBeNull()
  })

  it("never uses build-time or persistent operator credentials", () => {
    vi.stubEnv("VITE_INTERNAL_TOKEN", "environment-test-token")
    localStorage.setItem(STORAGE_KEYS.INTERNAL_TOKEN, "persistent-test-token")
    expect(resolveInternalToken()).toBeNull()
    saveSessionCredentials("", "session-test-token")
    expect(new Headers(buildInternalHeaders()).get("X-Internal-Token")).toBe("session-test-token")
  })

  it("adds a session write token only to same-origin API writes", async () => {
    saveSessionCredentials("write-test-token", "")
    await apiFetch("/api/v1/jobs", { method: "POST" })
    const options = vi.mocked(fetch).mock.calls[0][1]!
    expect(new Headers(options.headers).get("X-API-Token")).toBe("write-test-token")
  })

  it("does not forward session tokens to foreign origins or read requests", async () => {
    saveSessionCredentials("write-test-token", "operator-test-token")
    await apiFetch("/api/v1/jobs", { method: "POST", baseUrl: "https://example.test" })
    await apiFetch("/api/v1/jobs/test")
    for (const [, options] of vi.mocked(fetch).mock.calls) {
      expect(new Headers(options!.headers).has("X-API-Token")).toBe(false)
      expect(new Headers(options!.headers).has("X-Internal-Token")).toBe(false)
    }
  })

  it("preserves an explicitly supplied write token", async () => {
    saveSessionCredentials("session-test-token", "")
    await apiFetch("/api/v1/jobs", { method: "POST", headers: { "X-API-Token": "override-test-token" } })
    expect(new Headers(vi.mocked(fetch).mock.calls[0][1]!.headers).get("X-API-Token")).toBe("override-test-token")
  })

  it("retains response status and request correlation for access errors", async () => {
    vi.mocked(fetch).mockResolvedValue(new Response(JSON.stringify({ error: "Unauthorized", request_id: "request-test" }), { status: 401 }))
    await expect(apiFetch("/api/v1/jobs/dead")).rejects.toMatchObject({ name: "ApiError", status: 401, requestId: "request-test" })
    try { await apiFetch("/api/v1/jobs/dead") } catch (error) { expect(error).toBeInstanceOf(ApiError) }
  })

  it("allows a degraded readiness payload only for explicitly accepted statuses", async () => {
    vi.mocked(fetch).mockResolvedValue(new Response(JSON.stringify({ status: "degraded" }), { status: 503 }))
    await expect(apiFetch("/ready", { acceptedStatuses: [503] })).resolves.toEqual({ status: "degraded" })
    await expect(apiFetch("/ready")).rejects.toMatchObject({ status: 503 })
  })
})