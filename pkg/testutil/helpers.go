package testutil

import (
	"testing"

	"github.com/containerd/containerd/pkg/testutil"
	"github.com/google/go-cmp/cmp"
)

func IsIdentical(t *testing.T, x interface{}, y interface{}) {
	diff := cmp.Diff(x, y)
	if diff != "" {
		t.Fatalf(diff)
	}
}

func RequiresRoot(t testing.TB) {
	testutil.RequiresRoot(t)
}
