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
	"github.com/stretchr/testify/require"
)

// func TestPush(t *testing.T) {
// 	type testCase struct {
// 		name     string
// 		srcImg   types.Image
// 		expected ocispec.Descriptor
// 	}

// 	for _, tc := range []testCase{
// 		{
// 			"media_type",
// 			types.Image{},
// 			ocispec.Descriptor{
// 				MediaType: ocispec.MediaTypeImageManifest,
// 				Digest:    "sha256:8d1d8628f9398665a1efd215fb1684ca873a903119323a1af048e9eac009ca6c",
// 				Size:      558,
// 			},
// 		},
// 		// TODO ADD MORE
// 	} {
// 		t.Run(tc.name, func(t *testing.T) {
// 			ctx := context.TODO()

// 			provider := NewInmemoryProvider()
// 			imgResult, err := generateImage(ctx, tc.srcImg, provider)
// 			require.NoError(t, err)
// 			diff := cmp.Diff(imgResult, tc.expected)
// 			if diff != "" {
// 				t.Fatalf(diff)
// 			}
// 		})

// 	}
// }

func TestWriteNixClosureLayer(t *testing.T) {
	type testCase struct {
		name          string
		storePaths    []string
		copyToRoots   []string
		ExpectedDirs  []string
		ExpectedFiles []string
	}

	for _, tc := range []testCase{
		{
			"place_holder",
			[]string{"someDir/file1", "someDir/file2"},
			[]string{"someDir"},
			[]string{"someDir"},
			[]string{"file2", "file1"},
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

			tempFs, err := GetFileSystemFromTar(buf.Bytes())
			require.NoError(t, err)
			var fsFiles []string
			var fsDirs []string
			for path, attrs := range tempFs {
				_, err := tempFs.Stat(path)
				// If nil then File else Dir
				if err == nil {
					require.Equal(t, attrs.Mode, fs.FileMode(0x1ff))
					fsFiles = append(fsFiles, path)
				} else {
					require.Equal(t, attrs.Mode, fs.FileMode(0x1ed))
					fsDirs = append(fsDirs, path)
				}
				require.Equal(t, attrs.ModTime, time.Unix(0, 0))
				require.Equal(t, attrs.Data, make([]byte, 0))
			}

			sort.Strings(fsFiles)
			sort.Strings(tc.ExpectedFiles)
			diff := cmp.Diff(fsFiles, tc.ExpectedFiles)
			if diff != "" {
				t.Fatalf(diff)
			}

			var expectedDirs []string
			path := testDir
			for path != "/" {
				expectedDirs = append(expectedDirs, path[1:]+"/")
				path = filepath.Dir(path)
			}
			for _, path := range tc.ExpectedDirs {
				expectedDirs = append(expectedDirs, filepath.Join(testDir, path)[1:]+"/")
			}
			sort.Strings(fsDirs)
			sort.Strings(expectedDirs)
			diff = cmp.Diff(fsDirs, expectedDirs)
			if diff != "" {
				t.Fatalf(diff)
			}

		})

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
		// fmt.Printf("%v\n", cur)
		files[cur.Name] = &fstest.MapFile{Data: data, Mode: fs.FileMode(cur.Mode), ModTime: cur.ModTime}

	}
	return files, nil

}
