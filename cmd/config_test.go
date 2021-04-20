package cmd

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestFlagStringMethod(t *testing.T) {
	expectedOut := "[]"
	r := &registries{}
	out := r.String()
	if diff := cmp.Diff(out, expectedOut); diff != "" {
		t.Fatal(diff)
	}
}

func TestFlagSetMethod(t *testing.T) {
	r := &registries{}
	err := r.Set("blah")
	if diff := cmp.Diff(err.Error(), "invalid character 'b' looking for beginning of value"); diff != "" {
		t.Fatal(diff)
	}

	err = r.Set(`{"name":"","user":"","pass":""}`)
	if diff := cmp.Diff(err, nil); diff != "" {
		t.Fatal(diff)
	}
}

func TestInitConfig(t *testing.T) {

}
