package providers

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MockProvider is a controllable TranslationProvider for unit tests. It
// replays pre-configured results or errors in sequence; the last element in
// each slice is reused for all subsequent calls.
type MockProvider struct {
	mu        sync.Mutex
	name      string
	Results   []string        // translated text sequence; last repeated
	Errors    []error         // error sequence; last repeated (nil = success)
	Latencies []time.Duration // per-call artificial latency
	Calls     []TranslateRequest
}

// NewMockProvider returns a mock that always succeeds with result.
func NewMockProvider(name, result string) *MockProvider {
	return &MockProvider{name: name, Results: []string{result}}
}

// NewFailingMockProvider returns a mock that always returns err.
func NewFailingMockProvider(name string, err error) *MockProvider {
	return &MockProvider{name: name, Errors: []error{err}}
}

// NewTransientThenSuccessMock returns a mock that fails with transientErr for
// the first failCount calls, then succeeds with successResult.
func NewTransientThenSuccessMock(name string, failCount int, transientErr error, successResult string) *MockProvider {
	errs := make([]error, failCount+1)
	for i := range errs {
		if i < failCount {
			errs[i] = transientErr
		}
	}
	return &MockProvider{
		name:    name,
		Errors:  errs,
		Results: []string{successResult},
	}
}

func (m *MockProvider) Name() string { return m.name }

// Translate records the call and returns the configured result or error.
func (m *MockProvider) Translate(ctx context.Context, req TranslateRequest) (TranslateResponse, error) {
	m.mu.Lock()
	idx := len(m.Calls)
	m.Calls = append(m.Calls, req)
	m.mu.Unlock()

	if ctx.Err() != nil {
		return TranslateResponse{}, ctx.Err()
	}

	// Simulate per-call latency.
	if idx < len(m.Latencies) && m.Latencies[idx] > 0 {
		select {
		case <-time.After(m.Latencies[idx]):
		case <-ctx.Done():
			return TranslateResponse{}, ctx.Err()
		}
	}

	// Resolve error for this call index.
	if len(m.Errors) > 0 {
		errIdx := idx
		if errIdx >= len(m.Errors) {
			errIdx = len(m.Errors) - 1
		}
		if m.Errors[errIdx] != nil {
			return TranslateResponse{}, m.Errors[errIdx]
		}
	}

	// Resolve result for this call index.
	if len(m.Results) == 0 {
		return TranslateResponse{}, fmt.Errorf("mock provider %s: no results configured", m.name)
	}
	resultIdx := idx
	if resultIdx >= len(m.Results) {
		resultIdx = len(m.Results) - 1
	}
	return TranslateResponse{Text: m.Results[resultIdx], LatencyMS: 1}, nil
}

// CallCount returns the number of Translate calls recorded so far.
func (m *MockProvider) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.Calls)
}
