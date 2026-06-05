package stabilizer

import (
	"math/rand"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// NextDelay computes a delay with full jitter for the given attempt.
// attempt is 1-indexed. catch contains early intervals (p50, p75, p90).
func NextDelay(attempt int, catch []time.Duration, maxBackoff time.Duration) time.Duration {
	if attempt <= 0 {
		attempt = 1
	}
	if attempt <= len(catch) {
		base := catch[attempt-1]
		if base <= 0 {
			base = time.Second
		}
		return time.Duration(rand.Int63n(int64(base)))
	}
	expoStep := attempt - len(catch)
	// exponential base: 1s,2s,4s... (expoStep 1 => 1s)
	base := time.Second << (expoStep - 1)
	if base <= 0 {
		base = time.Second
	}
	if base > maxBackoff {
		base = maxBackoff
	}
	return time.Duration(rand.Int63n(int64(base)))
}
