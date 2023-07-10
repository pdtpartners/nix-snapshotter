package nix2container

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/google/go-cmp/cmp"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pdtpartners/nix-snapshotter/types"
	"github.com/stretchr/testify/require"
)

func TestBuild(t *testing.T) {
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
				Architecture: runtime.GOARCH,
				OS:           runtime.GOOS,
				StorePaths:   []string{"/some/file/location1", "/some/file/location2"},
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
			"full_image",
			types.Image{
				Config: ocispec.ImageConfig{
					Entrypoint: []string{
						"/some/file/location1",
					},
				},
				Architecture: runtime.GOARCH,
				OS:           runtime.GOOS,
				StorePaths:   []string{"/some/file/location2", "/some/file/location3"},
				CopyToRoots:  []string{"/some/file/location4", "/some/file/location5"},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testDir, err := os.MkdirTemp(getTempDir(), "nix2container-test")
			require.NoError(t, err)
			defer os.RemoveAll(testDir)

			writeOut := func(name string, data interface{}) string {
				dt, err := json.MarshalIndent(data, "", "  ")
				require.NoError(t, err)
				filePath := filepath.Join(testDir, name)
				os.WriteFile(filePath, dt, 0o644)
				return filePath
			}

			configPath := writeOut("imageConfig", &tc.sourceImage.Config)
			copyToRootsPath := writeOut("copyToRoots", &tc.sourceImage.CopyToRoots)

			//Is read in a different format to imageConfig and copyToRoots
			storePathsPath := filepath.Join(testDir, "storePaths")
			f, err := os.Create(storePathsPath)
			require.NoError(t, err)
			writer := bufio.NewWriter(f)
			for _, path := range tc.sourceImage.StorePaths {
				_, err = writer.WriteString(path + "\n")
				require.NoError(t, err)
			}
			writer.Flush()

			//Use build
			buildImagePath := filepath.Join(testDir, "buildImage")
			err = Build(configPath, storePathsPath, copyToRootsPath, buildImagePath)
			require.NoError(t, err)

			//Load back in sourceImage and compare to source
			var regeneratedImage types.Image
			dt, err := os.ReadFile(buildImagePath)
			require.NoError(t, err)
			err = json.Unmarshal(dt, &regeneratedImage)
			require.NoError(t, err)

			diff := cmp.Diff(tc.sourceImage, regeneratedImage)
			if diff != "" {
				t.Fatalf(diff)
			}
		})
	}
}
