package platform

import (
	"fmt"
	"time"
)

type TimeoutError struct {
	TimeoutValue time.Duration
}

func (t *TimeoutError) Error() string {
	return fmt.Sprintf("timeout reached: %v", t.TimeoutValue)
}

type ExecutionError struct {
	Stdout   string
	ExitCode int
	Details  string
	Msg      string
}

func (e *ExecutionError) Error() string {
	return fmt.Sprintf("msg: %v; exit code: %v; details: %v; stdout: %v", e.Msg, e.ExitCode, e.Details, e.Stdout)
}
