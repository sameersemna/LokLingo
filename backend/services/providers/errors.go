package providers

import "fmt"

// ProviderError represents an HTTP-level error response from a translation
// provider. It carries the HTTP status code so the orchestrator can classify
// whether the error is transient and worth retrying.
type ProviderError struct {
	Provider   string
	StatusCode int
	Body       string
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("provider %s status %d: %s", e.Provider, e.StatusCode, e.Body)
}

// IsTransient returns true when the status code indicates a condition that may
// resolve on retry: rate-limit (429), bad gateway (502), service unavailable
// (503), or gateway timeout (504).
func (e *ProviderError) IsTransient() bool {
	switch e.StatusCode {
	case 429, 502, 503, 504:
		return true
	}
	return false
}
