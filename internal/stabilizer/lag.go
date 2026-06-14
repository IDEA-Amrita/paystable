package stabilizer

import (
	"sort"
	"sync"
	"time"
)

// lagEstimator:webHook arrival->gateway API status reflection duration
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

//1) NewLagEstimator is used to determine the timing for p50,p75,p90 and p99(eg:p50 means p50 = 12s it means 50% of past confirmed successes showed up within 12s of the webhook)
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

//2) used to keep latest records with respect to a given bounded size
func (e *LagEstimator) Record(gateway string, lag time.Duration) {
	if lag < 0 {
		lag = 0
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	s := append(e.samples[gateway], lag)
	if len(s) > e.maxSample {
		s = s[len(s)-e.maxSample:]//to maintain once recent ones,if ir 502nd value,its updates in current 2nd value(bound 500)
	}
	e.samples[gateway] = s
}

//ScheduleFor returns the poll schedule for a gateway, using asymetric bounded polling
type Schedule struct {
	CatchPolls []time.Duration
	FailAfter  time.Duration
}

//3) ScheduleFor returns the poll schedule for a gateway, using asymetric bounded polling
func (e *LagEstimator) ScheduleFor(gateway string) Schedule {
	// 1.1 fix: copy inside RLock so concurrent Record() cannot race on the underlying array.
	e.mu.RLock()
	s := make([]time.Duration, len(e.samples[gateway]))
	copy(s, e.samples[gateway])
	e.mu.RUnlock()

	var p50, p75, p90, p99 time.Duration
	if len(s) < e.minSample {
		p50, p75, p90, p99 = e.prior.p50, e.prior.p75, e.prior.p90, e.prior.p99 //e.prior refers to default values
	} else {
		// s is already a safe copy — sort it directly.
		sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
		p50 = quantile(s, 0.50)
		p75 = quantile(s, 0.75)
		p90 = quantile(s, 0.90)
		p99 = quantile(s, 0.99)
	}

	return Schedule{
		CatchPolls: []time.Duration{p50, p75, p90},
		FailAfter:  p99,
	}
}


//4) A quantile pX is the value such that X% of samples are ≤ that value(similar to ascending order n percentile/median concept).If p50 = 12s it means 50% of past confirmed successes showed up within 12s of the webhook.
func quantile(sorted []time.Duration, q float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	rank := int(q * float64(len(sorted)))
	if rank >= len(sorted) {
		rank = len(sorted) - 1
	}
	return sorted[rank] //returns duration for given quantile 
}

//5) SampleCount returns the number of samples recorded for a gateway.
func (e *LagEstimator) SampleCount(gateway string) int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.samples[gateway])
}