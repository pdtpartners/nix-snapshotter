package testutil

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func IsIdentical(t *testing.T, x interface{}, y interface{}) {
	diff := cmp.Diff(x, y)
	if diff != "" {
		t.Fatalf(diff)
	}
}
