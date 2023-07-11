package nix2container

import (
	"context"
	"testing"

	"github.com/containerd/containerd/content"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
)

type MockPusher struct{}

func (mock *MockPusher) Push(ctx context.Context, d ocispec.Descriptor) (content.Writer, error) {

	return mock.Push(ctx, d)
}

func TestPush(t *testing.T) {
	type testCase struct {
		name string
	}

	for _, tc := range []testCase{
		{
			"reference",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, 0, 0)
		})
	}
}
