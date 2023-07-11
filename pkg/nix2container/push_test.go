package nix2container

import (
	"context"
	"os"
	"testing"

	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/platforms"
	"github.com/containerd/containerd/remotes"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pdtpartners/nix-snapshotter/types"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/semaphore"
)

type MockPusher struct{}

func (mock *MockPusher) Push(ctx context.Context, d ocispec.Descriptor) (content.Writer, error) {

	return mock.Push(ctx, d)
}

func TestPush(t *testing.T) {
	type testCase struct {
		name           string
		setUpImg       func(dirPath string) types.Image
		getPusher      func(o *PushOpts)
		getPushContent func(o *PushOpts)
		ref            string
	}

	getMockPushWithExpected := func(expectedRef string) func(o *PushOpts) {
		return func(o *PushOpts) {
			o.GetPusher = func(ctx context.Context, ref string) (remotes.Pusher, error) {
				require.Equal(t, expectedRef, ref)
				return &MockPusher{}, nil
			}
		}
	}

	getMockPushContentWithExpected := func() func(o *PushOpts) {
		return func(o *PushOpts) {
			o.GetPushContent = func(ctx context.Context, pusher remotes.Pusher, desc ocispec.Descriptor, store content.Provider, limiter *semaphore.Weighted, platform platforms.MatchComparer, wrapper func(h images.Handler) images.Handler) error {
				//  some require equal TODO
				return nil
			}
		}
	}

	for _, tc := range []testCase{
		{
			"reference",
			func(dirPath string) types.Image {
				return types.Image{}
			},
			getMockPushWithExpected("ref"),
			getMockPushContentWithExpected(),
			"ref",
		},
		{
			"placeholder",
			func(dirPath string) types.Image {
				return types.Image{
					Config:      ocispec.ImageConfig{},
					StorePaths:  []string{dirPath},
					CopyToRoots: []string{dirPath},
				}
			},
			getMockPushWithExpected(""),
			getMockPushContentWithExpected(),
			"",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.TODO()

			testDir, err := os.MkdirTemp(getTempDir(), "nix2container-test")
			require.NoError(t, err)
			defer os.RemoveAll(testDir)

			image := tc.setUpImg(testDir)

			err = Push(ctx, image, tc.ref, tc.getPusher, tc.getPushContent)
			require.Equal(t, err, nil)
		})
	}
}
