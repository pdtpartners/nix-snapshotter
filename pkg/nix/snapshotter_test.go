package nix

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/containerd/containerd/v2/core/mount"
	"github.com/containerd/containerd/v2/core/snapshots"
	"github.com/containerd/containerd/v2/core/snapshots/storage"
	"github.com/containerd/containerd/v2/core/snapshots/testsuite"
	"github.com/pdtpartners/nix-snapshotter/pkg/nix2container"
	"github.com/pdtpartners/nix-snapshotter/pkg/testutil"
	"github.com/stretchr/testify/require"
)

func newSnapshotterWithOpts(opts ...SnapshotterOpt) testsuite.SnapshotterFunc {
	return func(ctx context.Context, root string) (snapshots.Snapshotter, func() error, error) {
		snapshotter, err := NewSnapshotter(root, opts...)
		if err != nil {
			return nil, nil, err
		}

		return snapshotter, func() error { return snapshotter.Close() }, nil
	}
}

type testCase struct {
	name          string
	nixStorePaths []string
	extraLabels   map[string]string
}

func TestNixSnapshotter(t *testing.T) {
	for _, tc := range []testCase{
		{
			name: "empty",
		},
		{
			name: "basic",
			nixStorePaths: []string{
				"/nix/store/34xlpp3j3vy7ksn09zh44f1c04w77khf-libunistring-1.0",
				"/nix/store/4nlgxhb09sdr51nc9hdm8az5b08vzkgx-glibc-2.35-163",
				"/nix/store/5mh5019jigj0k14rdnjam1xwk5avn1id-libidn2-2.3.2",
				"/nix/store/g2m8kfw7kpgpph05v2fxcx4d5an09hl3-hello-2.12.1",
			},
			extraLabels: map[string]string{
				nix2container.NixLayerAnnotation: "true",
			},
		},
		{
			name: "custom nix store dir",
			nixStorePaths: []string{
				"/other/nix/store/34xlpp3j3vy7ksn09zh44f1c04w77khf-libunistring-1.0",
				"/other/nix/store/4nlgxhb09sdr51nc9hdm8az5b08vzkgx-glibc-2.35-163",
				"/other/nix/store/5mh5019jigj0k14rdnjam1xwk5avn1id-libidn2-2.3.2",
				"/other/nix/store/g2m8kfw7kpgpph05v2fxcx4d5an09hl3-hello-2.12.1",
			},
			extraLabels: map[string]string{
				nix2container.NixLayerAnnotation: "true",
			},
		},
		{
			name: "with no nix layer annotation",
			nixStorePaths: []string{
				"/nix/store/34xlpp3j3vy7ksn09zh44f1c04w77khf-libunistring-1.0",
				"/nix/store/4nlgxhb09sdr51nc9hdm8az5b08vzkgx-glibc-2.35-163",
				"/nix/store/5mh5019jigj0k14rdnjam1xwk5avn1id-libidn2-2.3.2",
				"/nix/store/g2m8kfw7kpgpph05v2fxcx4d5an09hl3-hello-2.12.1",
			},
		},
		{
			name: "with irrelevant labels",
			nixStorePaths: []string{
				"/nix/store/34xlpp3j3vy7ksn09zh44f1c04w77khf-libunistring-1.0",
				"/nix/store/4nlgxhb09sdr51nc9hdm8az5b08vzkgx-glibc-2.35-163",
				"/nix/store/5mh5019jigj0k14rdnjam1xwk5avn1id-libidn2-2.3.2",
				"/nix/store/g2m8kfw7kpgpph05v2fxcx4d5an09hl3-hello-2.12.1",
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
			for idx, value := range tc.nixStorePaths {
				labels[nix2container.NixStorePrefixAnnotation+strconv.Itoa(idx)] = value
			}
			for idx, value := range tc.extraLabels {
				labels[idx] = value
			}

			testBindMounts(ctx, t, tc, labels)
			testGCRoots(ctx, t, tc, labels)
		})
	}
}

func testBindMounts(ctx context.Context, t *testing.T, tc testCase, labels map[string]string) {
	key := "test"
	root := t.TempDir()
	snapshotterFunc := newSnapshotterWithOpts()
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
	for _, nixStorePath := range tc.nixStorePaths {
		expectedMounts = append(expectedMounts,
			mount.Mount{
				Type:    "bind",
				Source:  nixStorePath,
				Target:  nixStorePath,
				Options: []string{"ro", "rbind"},
			})
	}
	testutil.IsIdentical(t, mounts, expectedMounts)
}

func testGCRoots(ctx context.Context, t *testing.T, tc testCase, labels map[string]string) {
	key := "test"
	root := t.TempDir()

	var outLinks, nixStorePaths []string
	testBuilder := func(ctx context.Context, outLink, nixStorePath string) error {
		outLinks = append(outLinks, outLink)
		nixStorePaths = append(nixStorePaths, nixStorePath)
		return nil
	}

	snapshotterFunc := newSnapshotterWithOpts(WithNixBuilder(testBuilder))
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
		require.Equal(t, len(tc.nixStorePaths), len(outLinks))
		for idx := 0; idx < len(tc.nixStorePaths); idx += 1 {
			outLink := filepath.Join(root, "gcroots", id, filepath.Base(tc.nixStorePaths[idx]))
			testutil.IsIdentical(t, outLinks[idx], outLink)
			testutil.IsIdentical(t, nixStorePaths[idx], tc.nixStorePaths[idx])
		}
	} else {
		require.Equal(t, 0, len(outLinks))
	}
}
