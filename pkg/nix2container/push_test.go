package nix2container

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/containerd/containerd/archive/compression"
	"github.com/google/go-cmp/cmp"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pdtpartners/nix-snapshotter/types"
	"github.com/stretchr/testify/require"
)

func TestPush(t *testing.T) {
	type testCase struct {
		name     string
		srcImg   types.Image
		expected ocispec.Descriptor
	}

	for _, tc := range []testCase{
		{
			"media_type",
			types.Image{},
			ocispec.Descriptor{
				MediaType: ocispec.MediaTypeImageManifest,
				Digest:    "sha256:8d1d8628f9398665a1efd215fb1684ca873a903119323a1af048e9eac009ca6c",
				Size:      558,
			},
		},
		// TODO ADD MORE
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.TODO()

			provider := NewInmemoryProvider()
			imgResult, err := generateImage(ctx, tc.srcImg, provider)
			require.NoError(t, err)
			diff := cmp.Diff(imgResult, tc.expected)
			if diff != "" {
				t.Fatalf(diff)
			}
		})

	}
}

func TestWriteNixClosureLayer(t *testing.T) {
	type testCase struct {
		name        string
		storePaths  []string
		copyToRoots []string
	}

	for _, tc := range []testCase{
		{
			"place_holder",
			[]string{"some/path/file1", "some/path/file2", "some/file3", "some/other/file4"},
			[]string{"some/path", "some/other"},
		},
		// TODO ADD MORE
	} {
		t.Run(tc.name, func(t *testing.T) {
			testDir, err := os.MkdirTemp(getTempDir(), "nix2container-test")
			require.NoError(t, err)
			defer os.RemoveAll(testDir)

			// Generate Store Paths and files and append test dir to paths
			var joinedStorePaths []string
			for _, path := range tc.storePaths {
				joinedPath := filepath.Join(testDir, path)
				joinedStorePaths = append(joinedStorePaths, joinedPath)
				_, err := os.Stat(filepath.Dir(joinedPath))
				if os.IsNotExist(err) {
					err = os.MkdirAll(filepath.Dir(joinedPath), 0o755)
				}
				require.NoError(t, err)
				_, err = os.Create(joinedPath)
				require.NoError(t, err)
			}

			var joinedCopyToRoots []string
			for _, path := range tc.copyToRoots {
				joinedCopyToRoots = append(joinedCopyToRoots, filepath.Join(testDir, path))
			}

			ctx := context.TODO()
			buf := new(bytes.Buffer)
			_, err = writeNixClosureLayer(ctx, buf, joinedStorePaths, joinedCopyToRoots)

			fmt.Printf("%v\n\n", &buf)
			tr, err := compression.DecompressStream(tar.NewReader(bytes.NewReader(buf.Bytes())))
			require.NoError(t, err)
			fmt.Printf("%v", tr)
		})

	}
}
