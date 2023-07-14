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

	"github.com/google/go-cmp/cmp"
	specs "github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pdtpartners/nix-snapshotter/types"
	"github.com/stretchr/testify/require"
)

func TestInitilizeManifest(t *testing.T) {
	type testCase struct {
		name         string
		img          types.Image
		expectedMfst ocispec.Manifest
		expectedCfg  ocispec.Image
	}

	for _, tc := range []testCase{
		{
			"placeholder",
			types.Image{},
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
	} {
		t.Run(tc.name, func(t *testing.T) {

			ctx := context.TODO()
			provider := NewInmemoryProvider()
			mfst, cfg, err := initializeManifest(ctx, tc.img, provider)
			require.NoError(t, err)

			diff := cmp.Diff(mfst, tc.expectedMfst)
			if diff != "" {
				t.Fatalf(diff)
			}

			diff = cmp.Diff(cfg, tc.expectedCfg)
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
		ExpectedFs  []string
	}

	for _, tc := range []testCase{
		{
			"empty",
			[]string{},
			[]string{},
			[]string{},
		},
		{
			"file",
			[]string{"<tdir>/test.file"},
			[]string{"<tdir>/"},
			[]string{"test.file"},
		},
		{
			"file_with_dir",
			[]string{"<tdir>/dir/test.file"},
			[]string{"<tdir>/"},
			[]string{"<tdir>/dir/", "dir/", "dir/test.file"},
		},
		{
			"file_with_long_dir",
			[]string{"<tdir>/dir/that/is/long/test.file"},
			[]string{"<tdir>/"},
			[]string{"<tdir>/dir/", "<tdir>/dir/that/", "<tdir>/dir/that/is/", "<tdir>/dir/that/is/long/", "dir/", "dir/that/", "dir/that/is/", "dir/that/is/long/", "dir/that/is/long/test.file"},
		},
		{
			"file_with_copy_to_root",
			[]string{"<tdir>/dir/test.file"},
			[]string{"<tdir>/dir/"},
			[]string{"<tdir>/dir/", "test.file"},
		},
		{
			"file_with_copy_to_root_and_no_trailing_slash",
			[]string{"<tdir>/dir/test.file"},
			[]string{"<tdir>/dir"},
			[]string{"<tdir>/dir/", "test.file"},
		},
		{
			"multiple_files",
			[]string{"<tdir>/dir/test_1.file", "<tdir>/dir/test_2.file"},
			[]string{"<tdir>/dir/"},
			[]string{"<tdir>/dir/", "test_1.file", "test_2.file"},
		},
		{
			"multiple_files_on_different_levels",
			[]string{"<tdir>/dir/test_1.file", "<tdir>/test_2.file"},
			[]string{"<tdir>/"},
			[]string{"<tdir>/dir/", "dir/", "dir/test_1.file", "test_2.file"},
		},
		{
			"multiple_copy_to_roots",
			[]string{"<tdir>/dir/test.file"},
			[]string{"<tdir>/", "<tdir>/dir/"},
			[]string{"<tdir>/dir/", "dir/", "dir/test.file", "test.file"},
		},
		{
			"ignore_file_below_copy_to_root",
			[]string{"<tdir>/dir/test_1.file", "<tdir>/test_2.file"},
			[]string{"<tdir>/dir/"},
			[]string{"<tdir>/dir/", "test_1.file"},
		},
		// Is this actually the expected behaviour?
		{
			"no_copy_to_roots",
			[]string{"<tdir>/dir/test.file"},
			[]string{},
			[]string{"<tdir>/dir/"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testDir, err := os.MkdirTemp(getTempDir(), "nix2container-test")
			require.NoError(t, err)
			defer os.RemoveAll(testDir)

			// Generate files for the store paths and append the test dir we are working in to the paths
			for idx, path := range tc.storePaths {
				path := "/" + addTestDirIfNeeded(path, testDir)
				_, err := os.Stat(filepath.Dir(path))
				if os.IsNotExist(err) {
					err = os.MkdirAll(filepath.Dir(path), 0o755)
				}
				require.NoError(t, err)
				f, err := os.Create(path)
				require.NoError(t, err)
				f.Close()
				tc.storePaths[idx] = path
			}

			for idx, path := range tc.copyToRoots {
				tc.copyToRoots[idx] = "/" + addTestDirIfNeeded(path, testDir)
			}

			ctx := context.TODO()
			buf := new(bytes.Buffer)
			_, err = writeNixClosureLayer(ctx, buf, tc.storePaths, tc.copyToRoots)
			require.NoError(t, err)

			// Convert Tar to file system
			tempFs, err := GetFileSystemFromTar(buf.Bytes())
			require.NoError(t, err)
			fsOut := []string{}

			//Verify epoch 0 and file perms
			for path, attrs := range tempFs {
				_, err := tempFs.Stat(path)
				// If nil then File else Dir
				if err == nil {
					require.Equal(t, attrs.Mode, fs.FileMode(0x1ff))
				} else {
					require.Equal(t, attrs.Mode, fs.FileMode(0x1ed))
				}
				fsOut = append(fsOut, path)
				require.Equal(t, attrs.ModTime, time.Unix(0, 0))
				require.Equal(t, attrs.Data, make([]byte, 0))
			}

			for idx, path := range tc.ExpectedFs {
				tc.ExpectedFs[idx] = addTestDirIfNeeded(path, testDir)
			}

			// Fills in test folder dirs if any output is expected
			if len(tc.ExpectedFs) > 0 {
				path := testDir
				for path != "/" {
					tc.ExpectedFs = append(tc.ExpectedFs, path[1:]+"/")
					path = filepath.Dir(path)
				}
			}
			SameStringSlice(fsOut, tc.ExpectedFs, t)
		})

	}
}

func addTestDirIfNeeded(path string, testDir string) string {
	if len(path) >= 6 && path[:6] == "<tdir>" {
		return testDir[1:] + (path[6:])
	}
	return path
}

func SameStringSlice(sliceA []string, sliceB []string, t *testing.T) {
	sort.Strings(sliceA)
	sort.Strings(sliceB)
	diff := cmp.Diff(sliceA, sliceB)
	if diff != "" {
		t.Fatalf(diff)
	}
}

func GetFileSystemFromTar(tarBytes []byte) (fstest.MapFS, error) {
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
		}
		data, err := io.ReadAll(tarRead)
		if err != nil {
			return nil, err
		}
		files[cur.Name] = &fstest.MapFile{Data: data, Mode: fs.FileMode(cur.Mode), ModTime: cur.ModTime}

	}
	return files, nil

}
