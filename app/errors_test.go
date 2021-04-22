package app

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

/*
func TestExecutionError(t *testing.T) {
	expected := "msg: this was a failure; exit code: 127; details: so many details; stdout: i have failed"
	err := &ExecutionError{
		Stdout:   "i have failed",
		ExitCode: 127,
		Details:  "so many details",
		Msg:      "this was a failure",
	}
	if diff := cmp.Diff(err.Error(), expected); diff != "" {
		t.Fatal(diff)
	}
}
*/

func TestTimeoutError(t *testing.T) {
	expected := "timeout reached: 5s"
	err := &TimeoutError{
		TimeoutValue: time.Second * 5,
	}
	if diff := cmp.Diff(err.Error(), expected); diff != "" {
		t.Fatal(diff)
	}
}
