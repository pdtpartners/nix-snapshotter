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

	"github.com/google/go-cmp/cmp"
	specs "github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pdtpartners/nix-snapshotter/types"
	"github.com/stretchr/testify/require"
)

func TestInitilizeManifest(t *testing.T) {
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
			wrote_img, err := tc.setup(outPath)
			require.NoError(t, err)
			img := types.Image{}
			if wrote_img {
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

	// $tdir is the directory the test is run in and every parent directory
	// <tdir> inserts the test directory
	// e.g
	// ["$tdir"] -> ["user/1001/test4205/", "user/1001/","user/"]
	// ["<tdir>/dir/"] -> ["run/user/1001/nix2container-test4205847812/dir/"]
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
			[]string{"$tdir", "test.file"},
		},
		{
			"file_with_dir",
			[]string{"<tdir>/dir/test.file"},
			[]string{"<tdir>/"},
			[]string{"$tdir", "<tdir>/dir/", "dir/", "dir/test.file"},
		},
		{
			"file_with_long_dir",
			[]string{"<tdir>/dir/that/is/long/test.file"},
			[]string{"<tdir>/"},
			[]string{
				"$tdir",
				"<tdir>/dir/",
				"<tdir>/dir/that/",
				"<tdir>/dir/that/is/",
				"<tdir>/dir/that/is/long/",
				"dir/",
				"dir/that/",
				"dir/that/is/",
				"dir/that/is/long/",
				"dir/that/is/long/test.file"},
		},
		{
			"file_with_copy_to_root",
			[]string{"<tdir>/dir/test.file"},
			[]string{"<tdir>/dir/"},
			[]string{"$tdir", "<tdir>/dir/", "test.file"},
		},
		{
			"file_with_copy_to_root_and_no_trailing_slash",
			[]string{"<tdir>/dir/test.file"},
			[]string{"<tdir>/dir"},
			[]string{"$tdir", "<tdir>/dir/", "test.file"},
		},
		{
			"multiple_files",
			[]string{"<tdir>/dir/test_1.file", "<tdir>/dir/test_2.file"},
			[]string{"<tdir>/dir/"},
			[]string{"$tdir", "<tdir>/dir/", "test_1.file", "test_2.file"},
		},
		{
			"multiple_files_on_different_levels",
			[]string{"<tdir>/dir/test_1.file", "<tdir>/test_2.file"},
			[]string{"<tdir>/"},
			[]string{
				"$tdir",
				"<tdir>/dir/",
				"dir/",
				"dir/test_1.file",
				"test_2.file"},
		},
		{
			"multiple_copy_to_roots",
			[]string{"<tdir>/dir/test.file"},
			[]string{"<tdir>/", "<tdir>/dir/"},
			[]string{
				"$tdir",
				"<tdir>/dir/",
				"dir/",
				"dir/test.file",
				"test.file"},
		},
		{
			"ignore_file_below_copy_to_root",
			[]string{"<tdir>/dir/test_1.file", "<tdir>/test_2.file"},
			[]string{"<tdir>/dir/"},
			[]string{"$tdir", "<tdir>/dir/", "test_1.file"},
		},
		{
			"no_copy_to_roots",
			[]string{"<tdir>/dir/test.file"},
			[]string{},
			[]string{"$tdir", "<tdir>/dir/"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testDir, err := os.MkdirTemp(getTempDir(), "nix2container-test")
			require.NoError(t, err)
			defer os.RemoveAll(testDir)

			// Generate files for the store paths and append the test dir we
			// are working in to the paths
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
				fsOut = append(fsOut, path)
				require.Equal(t, attrs.ModTime, time.Unix(0, 0))
				require.Equal(t, attrs.Data, make([]byte, 0))
			}

			for idx, path := range tc.ExpectedFs {
				if path == "$tdir" {
					tc.ExpectedFs[idx] = testDir[1:] + "/"
					subTestPath := filepath.Dir(testDir)
					for subTestPath != "/" {
						tc.ExpectedFs = append(tc.ExpectedFs, subTestPath[1:]+"/")
						subTestPath = filepath.Dir(subTestPath)
					}
				} else {
					tc.ExpectedFs[idx] = addTestDirIfNeeded(path, testDir)
				}
			}

			sort.Strings(fsOut)
			sort.Strings(tc.ExpectedFs)
			diff := cmp.Diff(fsOut, tc.ExpectedFs)
			if diff != "" {
				t.Fatalf(diff)
			}
		})

	}
}

func addTestDirIfNeeded(path string, testDir string) string {
	if len(path) >= 6 && path[:6] == "<tdir>" {
		return testDir[1:] + (path[6:])
	}
	return path
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
		files[cur.Name] = &fstest.MapFile{
			Data:    data,
			Mode:    fs.FileMode(cur.Mode),
			ModTime: cur.ModTime}

	}
	return files, nil

}
