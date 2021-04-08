package internal

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestExecutionError(t *testing.T) {
	expected := "msg: this was a failure; exit code: 127; details: so many details; stdout: i have failed"
	err := &executionError{
		stdout:   "i have failed",
		exitCode: 127,
		details:  "so many details",
		msg:      "this was a failure",
	}
	if diff := cmp.Diff(err.Error(), expected); diff != "" {
		t.Fatal(diff)
	}
}

func TestTimeoutError(t *testing.T) {
	expected := "timeout reached: 5s"
	err := &timeoutError{
		timeoutValue: time.Second * 5,
	}
	if diff := cmp.Diff(err.Error(), expected); diff != "" {
		t.Fatal(diff)
	}
}
