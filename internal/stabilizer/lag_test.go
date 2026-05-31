package stabilizer

import (
	"testing"
	"time"
)

func TestScheduleFor_UsesPriorWhenColdStart(t *testing.T) {
	e := NewLagEstimator()

	//fewer than minSample records: should return the prior
	for i := 0; i < 10; i++ {
		e.Record("payu", 5*time.Second)
	}

	got := e.ScheduleFor("payu")
	if got.FailAfter != e.prior.p99 {
		t.Errorf("FailAfter = %v, want prior p99 %v", got.FailAfter, e.prior.p99)
	}
	if len(got.CatchPolls) != 3 {
		t.Fatalf("expected 3 catch polls, got %d", len(got.CatchPolls))
	}
	if got.CatchPolls[0] != e.prior.p50 {
		t.Errorf("first catch poll = %v, want prior p50 %v", got.CatchPolls[0], e.prior.p50)
	}
}

func TestScheduleFor_UsesEmpiricalWhenWarm(t *testing.T) {
	e := NewLagEstimator()

	// 100 samples evenly spread 1..100 seconds
	for i := 1; i <= 100; i++ {
		e.Record("payu", time.Duration(i)*time.Second)
	}

	got := e.ScheduleFor("payu")

	// p50 of 1..100 by nearest-rank is ~51s, p99 ~100s
	if got.CatchPolls[0] < 45*time.Second || got.CatchPolls[0] > 55*time.Second {
		t.Errorf("p50 = %v, want ~51s", got.CatchPolls[0])
	}
	if got.FailAfter < 95*time.Second || got.FailAfter > 100*time.Second {
		t.Errorf("p99 = %v, want ~100s", got.FailAfter)
	}
}

func TestRecord_RingBufferCapsAtMax(t *testing.T) {
	e := NewLagEstimator()

	for i := 0; i < e.maxSample+200; i++ {
		e.Record("payu", time.Second)
	}

	if c := e.SampleCount("payu"); c != e.maxSample {
		t.Errorf("sample count = %d, want %d", c, e.maxSample)
	}
}

func TestRecord_KeepsMostRecent(t *testing.T) {
	e := NewLagEstimator()
	e.minSample = 1

	//fill with old slow samples, then flood with recent fast ones
	for i := 0; i < e.maxSample; i++ {
		e.Record("payu", 500*time.Second)
	}
	for i := 0; i < e.maxSample; i++ {
		e.Record("payu", 2*time.Second)
	}

	got := e.ScheduleFor("payu")
	//all slow samples should have been evicted
	if got.FailAfter > 3*time.Second {
		t.Errorf("FailAfter = %v, want ~2s (slow samples should be evicted)", got.FailAfter)
	}
}

func TestRecord_NegativeLagClampedToZero(t *testing.T) {
	e := NewLagEstimator()
	e.minSample = 1
	e.Record("payu", -5*time.Second)

	got := e.ScheduleFor("payu")
	if got.CatchPolls[0] < 0 {
		t.Errorf("negative lag should be clamped, got %v", got.CatchPolls[0])
	}
}

func TestGatewaysAreIsolated(t *testing.T) {
	e := NewLagEstimator()
	e.minSample = 1

	e.Record("payu", 2*time.Second)
	e.Record("razorpay", 90*time.Second)

	payu := e.ScheduleFor("payu")
	razorpay := e.ScheduleFor("razorpay")

	if payu.CatchPolls[0] == razorpay.CatchPolls[0] {
		t.Error("expected per-gateway isolation, got identical schedules")
	}
}

func TestQuantile_EmptyReturnsZero(t *testing.T) {
	if q := quantile(nil, 0.5); q != 0 {
		t.Errorf("quantile of empty = %v, want 0", q)
	}
}

func TestSampleCount_UnknownGatewayIsZero(t *testing.T) {
	e := NewLagEstimator()
	if c := e.SampleCount("nonexistent"); c != 0 {
		t.Errorf("unknown gateway count = %d, want 0", c)
	}
}
