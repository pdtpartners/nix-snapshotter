package nix2container

import (
	_ "embed"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/pdtpartners/nix-snapshotter/types"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
)

//go:embed fixtures/hello-world.tar
var helloWorldTarball []byte

func TestDetectImageType(t *testing.T) {
	type testCase struct {
		name     string
		setup    func(outPath string) error
		expected ImageType
	}

	for _, tc := range []testCase{{
		"oci tarball",
		func(outPath string) error {
			return os.WriteFile(outPath, helloWorldTarball, 0o444)
		},
		ImageTypeOCITarball,
	}, {
		"nix2container image",
		func(outPath string) error {
			image := types.Image{
				Config: ocispec.ImageConfig{
					Entrypoint: []string{
						"/nix/store/g2m8kfw7kpgpph05v2fxcx4d5an09hl3-hello-2.12.1/bin/hello",
					},
				},
				Architecture: "amd64",
				OS:           "linux",
				StorePaths: []string{
					"/nix/store/34xlpp3j3vy7ksn09zh44f1c04w77khf-libunistring-1.0",
					"/nix/store/4nlgxhb09sdr51nc9hdm8az5b08vzkgx-glibc-2.35-163",
					"/nix/store/5mh5019jigj0k14rdnjam1xwk5avn1id-libidn2-2.3.2",
					"/nix/store/g2m8kfw7kpgpph05v2fxcx4d5an09hl3-hello-2.12.1",
				},
			}

			dt, err := json.MarshalIndent(&image, "", "  ")
			if err != nil {
				return err
			}

			return os.WriteFile(outPath, dt, 0o444)
		},
		ImageTypeNix,
	}} {
		t.Run(tc.name, func(t *testing.T) {
			testDir, err := os.MkdirTemp(getTempDir(), "nix2container-test")
			require.NoError(t, err)
			defer os.RemoveAll(testDir)

			outPath := filepath.Join(testDir, "out")
			err = tc.setup(outPath)
			require.NoError(t, err)

			actual, err := DetectImageType(outPath)
			require.NoError(t, err)
			require.Equal(t, tc.expected, actual)
		})
	}
}
