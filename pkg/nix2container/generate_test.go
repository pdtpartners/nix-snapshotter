package nix2container

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"testing/fstest"
	"time"

	"github.com/containerd/containerd/v2/core/content"
	"github.com/containerd/containerd/v2/plugins/content/local"
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
		setup        func(t *testing.T, store content.Store) *types.Image
		expectedMfst ocispec.Manifest
		expectedCfg  ocispec.Image
	}

	for _, tc := range []testCase{
		{
			"empty",
			func(t *testing.T, store content.Store) *types.Image {
				return &types.Image{}
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
			func(t *testing.T, store content.Store) *types.Image {
				return &types.Image{
					BaseImage: writeEmptyImage(t, store),
				}
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			testDir := t.TempDir()
			store, err := local.NewStore(filepath.Join(testDir, "store"))
			require.NoError(t, err)

			ctx := context.Background()
			img := tc.setup(t, store)
			mfst, cfg, err := initializeManifest(ctx, img, store)
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
			[]string{"$TEST_DIR/test-file"},
			[]string{},
			[]string{"$TEST_DIR_EXPAND", "$TEST_DIR/test-file"},
		},
		{
			"file_with_dirs",
			[]string{"$TEST_DIR/test-dir/bin/test-file"},
			[]string{},
			[]string{
				"$TEST_DIR_EXPAND",
				"$TEST_DIR/test-dir/",
				"$TEST_DIR/test-dir/bin/",
				"$TEST_DIR/test-dir/bin/test-file",
			},
		},
		{
			"file_with_copy_to_root",
			[]string{"$TEST_DIR/test-dir1/bin/test-file1", "$TEST_DIR/test-dir2/bin/test-file2"},
			[]string{"$TEST_DIR/test-dir1/"},
			[]string{
				"$TEST_DIR_EXPAND",
				"$TEST_DIR/test-dir1/",
				"$TEST_DIR/test-dir1/bin/",
				"$TEST_DIR/test-dir1/bin/test-file1",
				"$TEST_DIR/test-dir2/",
				"$TEST_DIR/test-dir2/bin/",
				"$TEST_DIR/test-dir2/bin/test-file2",
				"/bin/",
				"/bin/test-file1",
			},
		},
		{
			"file_with_copy_to_root_and_no_trailing_slash",
			[]string{"$TEST_DIR/test-dir/bin/test-file"},
			[]string{"$TEST_DIR/test-dir"},
			[]string{
				"$TEST_DIR_EXPAND",
				"$TEST_DIR/test-dir/",
				"$TEST_DIR/test-dir/bin/",
				"$TEST_DIR/test-dir/bin/test-file",
				"/bin/",
				"/bin/test-file",
			},
		},
		{
			"multiple_files",
			[]string{"$TEST_DIR/test-dir/bin/test-file1", "$TEST_DIR/test-dir/bin/test-file2"},
			[]string{"$TEST_DIR/test-dir/"},
			[]string{
				"$TEST_DIR_EXPAND",
				"$TEST_DIR/test-dir/",
				"$TEST_DIR/test-dir/bin/",
				"$TEST_DIR/test-dir/bin/test-file1",
				"$TEST_DIR/test-dir/bin/test-file2",
				"/bin/",
				"/bin/test-file1",
				"/bin/test-file2"},
		},
		{
			"multiple_files_on_different_levels",
			[]string{"$TEST_DIR/test-dir/bin/test-file", "$TEST_DIR/test-file"},
			[]string{"$TEST_DIR/test-dir/"},
			[]string{
				"$TEST_DIR_EXPAND",
				"$TEST_DIR/test-dir/",
				"$TEST_DIR/test-dir/bin/",
				"$TEST_DIR/test-dir/bin/test-file",
				"$TEST_DIR/test-file",
				"/bin/",
				"/bin/test-file",
			},
		},
		{
			"multiple_copy_to_roots",
			[]string{"$TEST_DIR/test-dir1/bin/test-file", "$TEST_DIR/test-dir2/share/test-file"},
			[]string{"$TEST_DIR/test-dir1", "$TEST_DIR/test-dir2/"},
			[]string{
				"$TEST_DIR_EXPAND",
				"$TEST_DIR/test-dir1/",
				"$TEST_DIR/test-dir1/bin/",
				"$TEST_DIR/test-dir1/bin/test-file",
				"$TEST_DIR/test-dir2/",
				"$TEST_DIR/test-dir2/share/",
				"$TEST_DIR/test-dir2/share/test-file",
				"/bin/",
				"/bin/test-file",
				"/share/",
				"/share/test-file",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testDir := t.TempDir()
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
			_, err := writeNixClosureLayer(ctx, buf, tc.storePaths, tc.copyToRoots)
			require.NoError(t, err)

			// Convert Tar to file system
			tempFs, err := newMapFSFromTar(buf.Bytes())
			require.NoError(t, err)
			fsOut := []string{}

			// Verify epoch 0 and file perms
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
			ModTime: cur.ModTime,
		}
	}

	return files, nil
}
