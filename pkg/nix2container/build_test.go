package nix2container

import (
	"os"
	"testing"
	"github.com/google/go-cmp/cmp"
	"runtime"
	"bufio"
	"encoding/json"
	"path/filepath"
	"github.com/stretchr/testify/require"
	"github.com/pdtpartners/nix-snapshotter/types"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

func TestBuild(t *testing.T) {
	type testCase struct {
		name     string
		setup    func(outPath string) (types.Image, error) 
		expected int
	}

	for _, tc := range []testCase{{
		"placeholder",
		func(outPath string) (types.Image, error) {
			image := types.Image{
				Config: ocispec.ImageConfig{},
				Architecture: runtime.GOARCH,
				OS:           runtime.GOOS,
				StorePaths: []string{"store_test1","store_test2"},
				CopyToRoots: []string{"copy_test1","copy_test2"},
			}
			return image, nil
		},
		1,
	}}{
		t.Run("test", func(t *testing.T) {
			testDir, err := os.MkdirTemp(getTempDir(), "nix2container-test")
			require.NoError(t, err)
			// defer os.RemoveAll(testDir)

			image, err := tc.setup(testDir)
			require.NoError(t, err)

			//Write out imageConfig
			dt, err := json.MarshalIndent(&image.Config, "", "  ")
			require.NoError(t, err)
			configPath := filepath.Join(testDir, "imageConfig")
			os.WriteFile(configPath, dt, 0o644)
			
			//Write out dummy files representing nix stores
			storePaths := filepath.Join(testDir, "storePaths")
			f, err := os.Create(storePaths)
			require.NoError(t, err)
			writer := bufio.NewWriter(f)
			for _, path := range image.StorePaths {
				_, err = writer.WriteString(path + "\n")
				require.NoError(t, err)
			}
			writer.Flush()

			//Write out copyToRoots
			dt, err = json.MarshalIndent(&image.CopyToRoots, "", "  ")
			require.NoError(t, err)
			copyToRootsPath := filepath.Join(testDir,"copyToRoots")
			os.WriteFile(copyToRootsPath, dt, 0o644)

			//Use build
			buildImagePath := filepath.Join(testDir, "buildImage")
			err = Build(configPath,storePaths,copyToRootsPath,buildImagePath)
			require.NoError(t, err)

			var regeneratedImage types.Image 
			dt, err = os.ReadFile(buildImagePath)
			require.NoError(t, err)
			err = json.Unmarshal(dt, &regeneratedImage)
			require.NoError(t, err)

			diff := cmp.Diff(image,regeneratedImage)
			if diff != "" {
				t.Fatalf(diff)
			}
			require.Equal(t, 1, 0)
		})
	}
}