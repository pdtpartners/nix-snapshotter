package nix2container

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"testing/fstest"
	"time"

	specs "github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pdtpartners/nix-snapshotter/pkg/testutil"
	"github.com/pdtpartners/nix-snapshotter/types"
	"github.com/stretchr/testify/require"
)

func TestInitializeManifest(t *testing.T) {
	type testCase struct {
		name         string
		setup        func(outPath string) (bool, error)
		expectedMfst ocispec.Manifest
		expectedCfg  ocispec.Image
	}

	for _, tc := range []testCase{
		{
			"empty",
			func(outPath string) (bool, error) {
				return false, nil
			},
			ocispec.Manifest{
				MediaType: ocispec.MediaTypeImageManifest,
				Versioned: specs.Versioned{
					SchemaVersion: 2,
				},
				Annotations: make(map[string]string),
			},
			ocispec.Image{
				RootFS: ocispec.RootFS{
					Type: "layers",
				},
			},
		},
		{
			"oci_tarball",
			func(outPath string) (bool, error) {
				return true, os.WriteFile(outPath, helloWorldTarball, 0o444)
			},
			ocispec.Manifest{
				MediaType: ocispec.MediaTypeImageManifest,
				Versioned: specs.Versioned{
					SchemaVersion: 2,
				},
				Layers: []v1.Descriptor{
					{
						MediaType: "application/vnd.oci.image.layer.v1.tar",
					},
				},
				Annotations: make(map[string]string),
			},
			ocispec.Image{
				RootFS: ocispec.RootFS{
					Type: "layers",
				},
			},
		},
		{
			"nix2container_image",
			func(outPath string) (bool, error) {
				image := types.Image{}
				dt, err := json.MarshalIndent(&image, "", "  ")
				if err != nil {
					return false, err
				}
				return true, os.WriteFile(outPath, dt, 0o444)
			},
			ocispec.Manifest{
				MediaType: ocispec.MediaTypeImageManifest,
				Versioned: specs.Versioned{
					SchemaVersion: 2,
				},
				Layers: []v1.Descriptor{
					{
						MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
						Annotations: map[string]string{
							"containerd.io/snapshot/nix-layer": "true"},
					},
				},
				Annotations: make(map[string]string),
			},
			ocispec.Image{
				RootFS: ocispec.RootFS{
					Type: "layers",
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testDir, err := os.MkdirTemp(getTempDir(), "nix2container-test")
			require.NoError(t, err)
			defer os.RemoveAll(testDir)

			outPath := filepath.Join(testDir, "out")
			wroteImg, err := tc.setup(outPath)
			require.NoError(t, err)
			img := types.Image{}
			if wroteImg {
				img.BaseImage = outPath
			}

			ctx := context.TODO()
			provider := NewInmemoryProvider()
			mfst, cfg, err := initializeManifest(ctx, img, provider)
			require.NoError(t, err)

			//Reset for ease of testing
			for idx := range mfst.Layers {
				mfst.Layers[idx].Digest = ""
				mfst.Layers[idx].Size = 0
			}
			cfg.RootFS.DiffIDs = nil

			testutil.IsIdentical(t, mfst, tc.expectedMfst)
			testutil.IsIdentical(t, cfg, tc.expectedCfg)
		})
	}
}

func TestWriteNixClosureLayer(t *testing.T) {
	type testCase struct {
		name                 string
		storePaths           []string
		copyToRoots          []string
		expectedTarballPaths []string
	}

	ctx := context.Background()

	// $TEST_DIR_EXPAND is the dir the test is run in and every parent directory
	// $TEST_DIR inserts the test directory
	// e.g
	// ["$TEST_DIR_EXPAND"] -> ["user/1001/test4205/", "user/1001/","user/"]
	// ["$TEST_DIR/dir/"] -> ["run/user/1001/nix2container-test4205847812/dir/"]
	for _, tc := range []testCase{
		{
			"empty",
			[]string{},
			[]string{},
			[]string{},
		},
		{
			"file",
			[]string{"$TEST_DIR/test.file"},
			[]string{"$TEST_DIR/"},
			[]string{"$TEST_DIR_EXPAND", "/test.file"},
		},
		{
			"file_with_dir",
			[]string{"$TEST_DIR/dir/test.file"},
			[]string{"$TEST_DIR/"},
			[]string{
				"$TEST_DIR_EXPAND",
				"$TEST_DIR/dir/",
				"/dir/",
				"/dir/test.file"},
		},
		{
			"file_with_long_dir",
			[]string{"$TEST_DIR/dir/that/is/long/test.file"},
			[]string{"$TEST_DIR/"},
			[]string{
				"$TEST_DIR_EXPAND",
				"$TEST_DIR/dir/",
				"$TEST_DIR/dir/that/",
				"$TEST_DIR/dir/that/is/",
				"$TEST_DIR/dir/that/is/long/",
				"/dir/",
				"/dir/that/",
				"/dir/that/is/",
				"/dir/that/is/long/",
				"/dir/that/is/long/test.file"},
		},
		{
			"file_with_copy_to_root",
			[]string{"$TEST_DIR/dir/test.file"},
			[]string{"$TEST_DIR/dir/"},
			[]string{"$TEST_DIR_EXPAND", "$TEST_DIR/dir/", "/test.file"},
		},
		{
			"file_with_copy_to_root_and_no_trailing_slash",
			[]string{"$TEST_DIR/dir/test.file"},
			[]string{"$TEST_DIR/dir"},
			[]string{"$TEST_DIR_EXPAND", "$TEST_DIR/dir/", "/test.file"},
		},
		{
			"multiple_files",
			[]string{"$TEST_DIR/dir/test_1.file", "$TEST_DIR/dir/test_2.file"},
			[]string{"$TEST_DIR/dir/"},
			[]string{
				"$TEST_DIR_EXPAND",
				"$TEST_DIR/dir/",
				"/test_1.file",
				"/test_2.file"},
		},
		{
			"multiple_files_on_different_levels",
			[]string{"$TEST_DIR/dir/test_1.file", "$TEST_DIR/test_2.file"},
			[]string{"$TEST_DIR/"},
			[]string{
				"$TEST_DIR_EXPAND",
				"$TEST_DIR/dir/",
				"/dir/",
				"/dir/test_1.file",
				"/test_2.file"},
		},
		{
			"multiple_copy_to_roots",
			[]string{"$TEST_DIR/dir/test.file"},
			[]string{"$TEST_DIR/", "$TEST_DIR/dir/"},
			[]string{
				"$TEST_DIR_EXPAND",
				"$TEST_DIR/dir/",
				"/dir/",
				"/dir/test.file",
				"/test.file"},
		},
		{
			"ignore_file_below_copy_to_root",
			[]string{"$TEST_DIR/dir/test_1.file", "$TEST_DIR/test_2.file"},
			[]string{"$TEST_DIR/dir/"},
			[]string{"$TEST_DIR_EXPAND", "$TEST_DIR/dir/", "/test_1.file"},
		},
		{
			"no_copy_to_roots",
			[]string{"$TEST_DIR/dir/test.file"},
			[]string{},
			[]string{"$TEST_DIR_EXPAND", "$TEST_DIR/dir/"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testDir, err := os.MkdirTemp(getTempDir(), "nix2container-test")
			require.NoError(t, err)
			defer os.RemoveAll(testDir)

			dirMapper := func(placeholderName string) string {
				if placeholderName == "TEST_DIR" {
					return string(testDir)
				} else {
					return ""
				}
			}

			// Generate files for the store paths and append the test dir we
			// are working in to the paths
			for idx, path := range tc.storePaths {
				path := os.Expand(path, dirMapper)
				_, err := os.Stat(filepath.Dir(path))
				if os.IsNotExist(err) {
					err = os.MkdirAll(filepath.Dir(path), 0o755)
				}
				require.NoError(t, err)
				f, err := os.Create(path)
				require.NoError(t, err)
				err = f.Close()
				require.NoError(t, err)
				tc.storePaths[idx] = path
			}

			for idx, path := range tc.copyToRoots {
				tc.copyToRoots[idx] = os.Expand(path, dirMapper)
			}

			buf := new(bytes.Buffer)
			_, err = writeNixClosureLayer(
				ctx, buf, tc.storePaths, tc.copyToRoots)
			require.NoError(t, err)

			// Convert Tar to file system
			tempFs, err := newMapFSFromTar(buf.Bytes())
			require.NoError(t, err)
			fsOut := []string{}

			//Verify epoch 0 and file perms
			for path, attrs := range tempFs {
				fsOut = append(fsOut, "/"+path)
				require.Equal(t, attrs.ModTime, time.Unix(0, 0))
				require.Equal(t, attrs.Data, make([]byte, 0))
			}

			for idx, path := range tc.expectedTarballPaths {
				if path == "$TEST_DIR_EXPAND" {
					tc.expectedTarballPaths[idx] = testDir + "/"
					subTestPath := filepath.Dir(testDir)
					for subTestPath != "/" {
						tc.expectedTarballPaths = append(
							tc.expectedTarballPaths, subTestPath+"/")
						subTestPath = filepath.Dir(subTestPath)
					}
				} else {
					tc.expectedTarballPaths[idx] = os.Expand(path, dirMapper)
				}
			}

			sort.Strings(fsOut)
			sort.Strings(tc.expectedTarballPaths)
			testutil.IsIdentical(t, fsOut, tc.expectedTarballPaths)
		})

	}
}

func newMapFSFromTar(tarBytes []byte) (fstest.MapFS, error) {
	gzRead, err := gzip.NewReader(bytes.NewReader(tarBytes))
	if err != nil {
		return nil, err
	}
	tarRead := tar.NewReader(gzRead)
	files := make(fstest.MapFS)
	for {
		cur, err := tarRead.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(tarRead)
		if err != nil {
			return nil, err
		}
		files[cur.Name] = &fstest.MapFile{
			Data:    data,
			Mode:    fs.FileMode(cur.Mode),
			ModTime: cur.ModTime}

	}
	return files, nil

}
