package stabilizer

import (
	"sort"
	"sync"
	"time"
)

// LagEstimator learns, per gateway, how long a genuinely successful payment
// takes to appear on the verification API after the webhook arrives. Only feed
// it samples from transactions that confirmed; see docs/lag-estimator.md
type LagEstimator struct {
	mu        sync.RWMutex
	maxSample int
	minSample int
	prior     defaults
	samples   map[string][]time.Duration
}

type defaults struct {
	p50, p75, p90, p99 time.Duration
}

func NewLagEstimator() *LagEstimator {
	return &LagEstimator{
		maxSample: 500,
		minSample: 50,
		prior: defaults{
			p50: 10 * time.Second,
			p75: 30 * time.Second,
			p90: 60 * time.Second,
			p99: 180 * time.Second,
		},
		samples: make(map[string][]time.Duration),
	}
}

// Record adds a success-propagation lag sample (webhook arrival to first
// success poll). only call for confirmed transactions
func (e *LagEstimator) Record(gateway string, lag time.Duration) {
	if lag < 0 {
		lag = 0
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	s := append(e.samples[gateway], lag)
	if len(s) > e.maxSample {
		s = s[len(s)-e.maxSample:]
	}
	e.samples[gateway] = s
}

// Schedule is when to poll (to catch a late success) and when to give up and
// declare failure.
type Schedule struct {
	CatchPolls []time.Duration
	FailAfter  time.Duration
}

// ScheduleFor returns the poll schedule for a gateway, using the conservative
// prior until enough samples accumulate, then empirical quantiles
func (e *LagEstimator) ScheduleFor(gateway string) Schedule {
	e.mu.RLock()
	s := e.samples[gateway]
	e.mu.RUnlock()

	var p50, p75, p90, p99 time.Duration
	if len(s) < e.minSample {
		p50, p75, p90, p99 = e.prior.p50, e.prior.p75, e.prior.p90, e.prior.p99
	} else {
		sorted := make([]time.Duration, len(s))
		copy(sorted, s)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
		p50 = quantile(sorted, 0.50)
		p75 = quantile(sorted, 0.75)
		p90 = quantile(sorted, 0.90)
		p99 = quantile(sorted, 0.99)
	}

	return Schedule{
		CatchPolls: []time.Duration{p50, p75, p90},
		FailAfter:  p99,
	}
}

// quantile returns the nearest-rank value at percentile q (0..1). sorted must
// be ascending
func quantile(sorted []time.Duration, q float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	rank := int(q * float64(len(sorted)))
	if rank >= len(sorted) {
		rank = len(sorted) - 1
	}
	return sorted[rank]
}

func (e *LagEstimator) SampleCount(gateway string) int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.samples[gateway])
}
