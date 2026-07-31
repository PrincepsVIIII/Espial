package adapters

import "time"

const (
	RestartBaseDelay = time.Second
	RestartMaxDelay  = time.Minute
	HealthyReset     = 5 * time.Minute
)

func RestartDelay(failures int, jitter float64) time.Duration {
	if failures < 1 {
		failures = 1
	}
	if jitter < 0.8 {
		jitter = 0.8
	}
	if jitter > 1.2 {
		jitter = 1.2
	}
	delay := RestartBaseDelay
	for count := 1; count < failures && delay < RestartMaxDelay; count++ {
		delay *= 2
		if delay > RestartMaxDelay {
			delay = RestartMaxDelay
		}
	}
	return time.Duration(float64(delay) * jitter)
}
