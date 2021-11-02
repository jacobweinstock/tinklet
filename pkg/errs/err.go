package errs

import (
	"fmt"
	"time"
)

// TimeoutError time out errors.
type TimeoutError struct {
	TimeoutValue time.Duration
}

func (t *TimeoutError) Error() string {
	return fmt.Sprintf("timeout reached: %v", t.TimeoutValue)
}
