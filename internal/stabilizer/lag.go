package stabilizer

import (
	"sort"
	"sync"
	"time"
)

// webHooks  will arrive at paystable n one more repoll request will go to payMent gateway ryt!!this buffer time is lag estimator
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

//1) NewLagEstimator initializes a LagEstimator with default values(10s, 30s, 60s, 180s) and empty samples.
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

//2) Record adds a new lag sample for a gateway, ensuring it doesn't exceed maxSample. Negative lags are treated as zero.
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

//ScheduleFor returns the poll schedule for a gateway, using the conservative prior until enough samples accumulate
type Schedule struct {
	CatchPolls []time.Duration
	FailAfter  time.Duration
}

//3) ScheduleFor computes the poll schedule for a gateway based on recorded samples. If samples are below minSample, it uses the prior defaults
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


//4) quantile computes the q-th quantile from a sorted slice of durations. If the slice is empty, it returns zero. If q is out of bounds, it returns the closest valid quantile.
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
//5) SampleCount returns the number of samples recorded for a gateway.
func (e *LagEstimator) SampleCount(gateway string) int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.samples[gateway])
}
