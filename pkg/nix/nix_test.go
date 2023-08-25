package nix

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/containerd/containerd/mount"
	"github.com/containerd/containerd/snapshots"
	"github.com/containerd/containerd/snapshots/storage"
	"github.com/containerd/containerd/snapshots/testsuite"
	"github.com/pdtpartners/nix-snapshotter/pkg/nix2container"
	"github.com/pdtpartners/nix-snapshotter/pkg/testutil"
	"github.com/stretchr/testify/require"
)

func newSnapshotterWithOpts(nixStorePrefix string, opts ...interface{}) testsuite.SnapshotterFunc {
	return func(ctx context.Context, root string) (snapshots.Snapshotter, func() error, error) {
		snapshotter, err := NewSnapshotter(root, nixStorePrefix, opts...)
		if err != nil {
			return nil, nil, err
		}

		return snapshotter, func() error { return snapshotter.Close() }, nil
	}
}

func TestNixSnapshotter(t *testing.T) {
	type testCase struct {
		name        string
		nixHashes   []string
		extraLabels map[string]string
	}

	for _, tc := range []testCase{
		{
			name: "empty",
		},
		{
			name: "basic",
			nixHashes: []string{
				"34xlpp3j3vy7ksn09zh44f1c04w77khf-libunistring-1.0",
				"4nlgxhb09sdr51nc9hdm8az5b08vzkgx-glibc-2.35-163",
				"5mh5019jigj0k14rdnjam1xwk5avn1id-libidn2-2.3.2",
				"g2m8kfw7kpgpph05v2fxcx4d5an09hl3-hello-2.12.1",
			},
			extraLabels: map[string]string{
				nix2container.NixLayerAnnotation: "true",
			},
		},
		{
			name: "with no nix layer annotation",
			nixHashes: []string{
				"34xlpp3j3vy7ksn09zh44f1c04w77khf-libunistring-1.0",
				"4nlgxhb09sdr51nc9hdm8az5b08vzkgx-glibc-2.35-163",
				"5mh5019jigj0k14rdnjam1xwk5avn1id-libidn2-2.3.2",
				"g2m8kfw7kpgpph05v2fxcx4d5an09hl3-hello-2.12.1",
			},
		},
		{
			name: "with irrelevant labels",
			nixHashes: []string{
				"34xlpp3j3vy7ksn09zh44f1c04w77khf-libunistring-1.0",
				"4nlgxhb09sdr51nc9hdm8az5b08vzkgx-glibc-2.35-163",
				"5mh5019jigj0k14rdnjam1xwk5avn1id-libidn2-2.3.2",
				"g2m8kfw7kpgpph05v2fxcx4d5an09hl3-hello-2.12.1",
			},
			extraLabels: map[string]string{
				"labelToBeIgnored":               "ValueToBeIgnored",
				nix2container.NixLayerAnnotation: "true",
				"labelToBeIgnored2":              "ValueToBeIgnored2",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			labels := map[string]string{}
			for idx, value := range tc.nixHashes {
				labels[nix2container.NixStorePrefixAnnotation+strconv.Itoa(idx)] = value
			}
			for idx, value := range tc.extraLabels {
				labels[idx] = value
			}

			testBindMounts(t, ctx, tc.nixHashes, labels)
			testGCRoots(t, ctx, tc.nixHashes, labels)
		})
	}
}

func testBindMounts(t *testing.T, ctx context.Context, nixHashes []string, labels map[string]string) {
	key := "test"
	nixStorePrefix := "/nix/store"
	root := t.TempDir()
	snapshotterFunc := newSnapshotterWithOpts(nixStorePrefix)
	snapshotter, _, err := snapshotterFunc(ctx, root)
	require.NoError(t, err)
	s := snapshotter.(*nixSnapshotter)
	require.NoError(t, err)

	// Test that Prepare doesn't interact badly with Nix labels.
	_, err = s.Prepare(ctx, key, "", snapshots.WithLabels(labels))
	require.NoError(t, err)

	// Since we only care about the nix bind mounts, ignore the overlay mounts.
	mounts, err := s.withNixBindMounts(ctx, key, []mount.Mount{})
	require.NoError(t, err)

	expectedMounts := []mount.Mount{}
	for _, nixStore := range nixHashes {
		expectedMounts = append(expectedMounts,
			mount.Mount{
				Type:    "bind",
				Source:  filepath.Join(nixStorePrefix, nixStore),
				Target:  filepath.Join(nixStorePrefix, nixStore),
				Options: []string{"ro", "rbind"},
			})
	}
	testutil.IsIdentical(t, mounts, expectedMounts)
}

func testGCRoots(t *testing.T, ctx context.Context, nixHashes []string, labels map[string]string) {
	key := "test"
	nixStorePrefix := "/nix/store"
	root := t.TempDir()

	var gcRootPaths, nixStorePaths []string
	testBuilder := func(ctx context.Context, gcRootPath, nixStorePath string) error {
		gcRootPaths = append(gcRootPaths, gcRootPath)
		nixStorePaths = append(nixStorePaths, nixStorePath)
		return nil
	}

	snapshotterFunc := newSnapshotterWithOpts(nixStorePrefix, WithNixBuilder(testBuilder))
	snapshotter, _, err := snapshotterFunc(ctx, root)
	require.NoError(t, err)
	s := snapshotter.(*nixSnapshotter)
	require.NoError(t, err)

	_, err = s.Prepare(ctx, key, "", snapshots.WithLabels(labels))
	require.NoError(t, err)

	var id string
	err = s.ms.WithTransaction(ctx, false, func(ctx context.Context) (err error) {
		id, _, _, err = storage.GetInfo(ctx, key)
		return err
	})
	require.NoError(t, err)

	if labels[nix2container.NixLayerAnnotation] == "true" {
		require.Equal(t, len(nixHashes), len(gcRootPaths))
		for idx := 0; idx < len(nixHashes); idx += 1 {
			testutil.IsIdentical(t, gcRootPaths[idx], filepath.Join(root, "gcroots", id, nixHashes[idx]))
			testutil.IsIdentical(t, nixStorePaths[idx], filepath.Join(nixStorePrefix, nixHashes[idx]))
		}
	} else {
		require.Equal(t, 0, len(gcRootPaths))
	}

}
