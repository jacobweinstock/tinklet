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

type executionError struct {
	stdout   string
	exitCode int
	details  string
	msg      string
}

func (a *executionError) Error() string {
	return fmt.Sprintf("msg: %v; exit code: %v; details: %v; stdout: %v", a.msg, a.exitCode, a.details, a.stdout)
}
