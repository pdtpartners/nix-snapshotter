package nix2container

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPush(t *testing.T) {
	type testCase struct {
		name string
	}

	for _, tc := range []testCase{
		{
			"placeholder",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, 1, 1)
		})
	}
}
