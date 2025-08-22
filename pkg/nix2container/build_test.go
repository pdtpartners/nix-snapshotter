package nix2container

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/containerd/containerd/v2/core/content"
	"github.com/containerd/containerd/v2/core/images/archive"
	"github.com/containerd/containerd/v2/plugins/content/local"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pdtpartners/nix-snapshotter/types"
	"github.com/stretchr/testify/require"
)

func TestBuild(t *testing.T) {
	store, err := local.NewStore(filepath.Join(t.TempDir(), "store"))
	require.NoError(t, err)

	baseImagePath := writeEmptyImage(t, store)

	type testCase struct {
		name        string
		sourceImage types.Image
	}

	for _, tc := range []testCase{
		{
			"empty",
			types.Image{
				Architecture: runtime.GOARCH,
				OS:           runtime.GOOS,
			},
		},
		{
			"config",
			types.Image{
				Config: ocispec.ImageConfig{
					Entrypoint: []string{
						"/some/file/location",
					},
				},
				Architecture: runtime.GOARCH,
				OS:           runtime.GOOS,
			},
		},
		{
			"store_paths",
			types.Image{
				Architecture:  runtime.GOARCH,
				OS:            runtime.GOOS,
				NixStorePaths: []string{"/some/file/location1", "/some/file/location2"},
			},
		},
		{
			"copy_to_roots",
			types.Image{
				Architecture: runtime.GOARCH,
				OS:           runtime.GOOS,
				CopyToRoots:  []string{"/some/file/location1", "/some/file/location2"},
			},
		},
		{
			"base_image",
			types.Image{
				Architecture: runtime.GOARCH,
				OS:           runtime.GOOS,
				BaseImage:    baseImagePath,
			},
		},
		{
			"full_image",
			types.Image{
				Config: ocispec.ImageConfig{
					Entrypoint: []string{
						"/some/file/location1",
					},
				},
				Architecture:  runtime.GOARCH,
				OS:            runtime.GOOS,
				NixStorePaths: []string{"/some/file/location2", "/some/file/location3"},
				CopyToRoots:   []string{"/some/file/location4", "/some/file/location5"},
				BaseImage:     baseImagePath,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testDir := t.TempDir()
			configPath := writeOut(t, testDir, "imageConfig", tc.sourceImage.Config)
			copyToRootsPath := writeCopyToRoots(t, testDir, "copyToRoots", tc.sourceImage.CopyToRoots)
			closurePath := writeClosure(t, testDir, tc.sourceImage.NixStorePaths)

			ctx := context.Background()
			img, err := Build(ctx,
				configPath,
				closurePath,
				copyToRootsPath,
				WithFromImage(tc.sourceImage.BaseImage),
			)
			require.NoError(t, err)

			buf := new(bytes.Buffer)
			err = Export(ctx, store, img, tc.name, buf)
			require.NoError(t, err)

			_, err = archive.ImportIndex(ctx, store, buf)
			require.NoError(t, err)

			// TODO: Use archive.ImportIndex and verify everything
		})
	}
}

func writeOut(t *testing.T, testDir, name string, data interface{}) string {
	dt, err := json.MarshalIndent(data, "", "  ")
	require.NoError(t, err)

	filename := filepath.Join(testDir, name)
	err = os.WriteFile(filename, dt, 0o644)
	require.NoError(t, err)

	return filename
}

func writeCopyToRoots(t *testing.T, testDir, name string, nixStorePaths []string) string {
	var realPaths []string
	for _, nixStorePath := range nixStorePaths {
		realPath := filepath.Join(testDir, nixStorePath)
		realPaths = append(realPaths, realPath)

		err := os.MkdirAll(realPath, 0o755)
		require.NoError(t, err)
	}
	return writeOut(t, testDir, "copyToRoots", realPaths)
}

func writeClosure(t *testing.T, testDir string, nixStorePaths []string) string {
	closurePath := filepath.Join(testDir, "storePaths")
	f, err := os.Create(closurePath)
	require.NoError(t, err)
	defer f.Close()

	writer := bufio.NewWriter(f)
	for _, nixStorePath := range nixStorePaths {
		realPath := filepath.Join(testDir, nixStorePath)
		_, err = writer.WriteString(realPath + "\n")
		require.NoError(t, err)

		err = os.MkdirAll(realPath, 0o755)
		require.NoError(t, err)
	}

	err = writer.Flush()
	require.NoError(t, err)

	return closurePath
}

func writeEmptyImage(t *testing.T, store content.Store) string {
	testDir := t.TempDir()
	imagePath := filepath.Join(testDir, "base-image")

	f, err := os.Create(imagePath)
	require.NoError(t, err)
	defer f.Close()

	err = Export(context.Background(), store, &types.Image{}, "base/image", f)
	require.NoError(t, err)

	return imagePath
}
