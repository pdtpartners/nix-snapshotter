package testutil

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func IsIdentical(x interface{}, y interface{}, t *testing.T) {
	diff := cmp.Diff(x, y)
	if diff != "" {
		t.Fatalf(diff)
	}
}
