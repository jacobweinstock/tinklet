package errs

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestTimeoutError(t *testing.T) {
	expected := "timeout reached: 5s"
	err := &TimeoutError{
		TimeoutValue: time.Second * 5,
	}
	if diff := cmp.Diff(err.Error(), expected); diff != "" {
		t.Fatal(diff)
	}
}
