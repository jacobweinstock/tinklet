package app

import (
	"fmt"
	"time"
)

// TimeoutError time out errors
type TimeoutError struct {
	TimeoutValue time.Duration
}

func (t *TimeoutError) Error() string {
	return fmt.Sprintf("timeout reached: %v", t.TimeoutValue)
}

/*
// ExecutionError execution errors
type ExecutionError struct {
	Stdout   string
	ExitCode int
	Details  string
	Msg      string
}

func (e *ExecutionError) Error() string {
	return fmt.Sprintf("msg: %v; exit code: %v; details: %v; stdout: %v", e.Msg, e.ExitCode, e.Details, e.Stdout)
}
*/
