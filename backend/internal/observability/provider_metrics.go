package observability

import (
	"sync"
	"sync/atomic"
)

// providerCounters holds per-provider atomic reliability counters.
type providerCounters struct {
	success        atomic.Int64
	failures       atomic.Int64
	retries        atomic.Int64
	exhausted      atomic.Int64
	failovers      atomic.Int64
	latencyTotalMS atomic.Int64
	latencyCount   atomic.Int64
}

// providerMetricsMap stores *providerCounters keyed by provider name.
var providerMetricsMap sync.Map

func getProviderCounters(name string) *providerCounters {
	v, _ := providerMetricsMap.LoadOrStore(name, &providerCounters{})
	return v.(*providerCounters)
}

// IncProviderSuccess records a successful translation call on provider.
func IncProviderSuccess(provider string) { getProviderCounters(provider).success.Add(1) }

// IncProviderFailure records a final failure on provider (all retries exhausted
// or a permanent error) before failing over to the next provider.
func IncProviderFailure(provider string) { getProviderCounters(provider).failures.Add(1) }

// RecordProviderRetry records one transient retry on provider.
func RecordProviderRetry(provider string) { getProviderCounters(provider).retries.Add(1) }

// IncProviderExhausted records that provider exhausted all retry attempts.
func IncProviderExhausted(provider string) { getProviderCounters(provider).exhausted.Add(1) }

// IncProviderFailover records that provider was chosen as a failover target
// (i.e. a previous provider failed and this one succeeded).
func IncProviderFailover(provider string) { getProviderCounters(provider).failovers.Add(1) }

// RecordProviderLatency records a successful call latency in milliseconds.
func RecordProviderLatency(provider string, ms int64) {
	c := getProviderCounters(provider)
	c.latencyTotalMS.Add(ms)
	c.latencyCount.Add(1)
}

// ProviderSnapshot captures per-provider counters at a point in time.
type ProviderSnapshot struct {
	Provider       string  `json:"provider"`
	SuccessTotal   int64   `json:"success_total"`
	FailureTotal   int64   `json:"failure_total"`
	RetryTotal     int64   `json:"retry_total"`
	ExhaustedTotal int64   `json:"exhausted_total"`
	FailoverTotal  int64   `json:"failover_total"`
	AvgLatencyMS   float64 `json:"avg_latency_ms"`
}

// SnapshotProviderMetrics returns a snapshot for every provider that has been
// observed since process start.
func SnapshotProviderMetrics() []ProviderSnapshot {
	var out []ProviderSnapshot
	providerMetricsMap.Range(func(k, v any) bool {
		name := k.(string)
		c := v.(*providerCounters)
		cnt := c.latencyCount.Load()
		avg := 0.0
		if cnt > 0 {
			avg = float64(c.latencyTotalMS.Load()) / float64(cnt)
		}
		out = append(out, ProviderSnapshot{
			Provider:       name,
			SuccessTotal:   c.success.Load(),
			FailureTotal:   c.failures.Load(),
			RetryTotal:     c.retries.Load(),
			ExhaustedTotal: c.exhausted.Load(),
			FailoverTotal:  c.failovers.Load(),
			AvgLatencyMS:   avg,
		})
		return true
	})
	return out
}
