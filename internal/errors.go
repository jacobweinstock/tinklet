package internal

import (
	"fmt"
	"time"
)

type timeoutError struct {
	timeoutValue time.Duration
}

func (t *timeoutError) Error() string {
	return fmt.Sprintf("timeout reached: %v", t.timeoutValue)
}

type actionFailedError struct {
	stdout   string
	exitCode int
	details  string
}

func (a *actionFailedError) Error() string {
	return fmt.Sprintf("action failed; exit code: %v; details: %v; stdout: %v", a.exitCode, a.details, a.stdout)
}
