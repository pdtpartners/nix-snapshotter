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
		nixStores   []string
		extraLabels map[string]string
	}

	for _, tc := range []testCase{
		{
			name: "empty",
		},
		{
			name: "basic",
			nixStores: []string{
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
			name: "noNixLayerAnnotation",
			nixStores: []string{
				"34xlpp3j3vy7ksn09zh44f1c04w77khf-libunistring-1.0",
				"4nlgxhb09sdr51nc9hdm8az5b08vzkgx-glibc-2.35-163",
				"5mh5019jigj0k14rdnjam1xwk5avn1id-libidn2-2.3.2",
				"g2m8kfw7kpgpph05v2fxcx4d5an09hl3-hello-2.12.1",
			},
		},
		{
			name: "irrelevantLabels",
			nixStores: []string{
				"34xlpp3j3vy7ksn09zh44f1c04w77khf-libunistring-1.0",
				"4nlgxhb09sdr51nc9hdm8az5b08vzkgx-glibc-2.35-163",
				"5mh5019jigj0k14rdnjam1xwk5avn1id-libidn2-2.3.2",
				"g2m8kfw7kpgpph05v2fxcx4d5an09hl3-hello-2.12.1",
			},
			extraLabels: map[string]string{
				nix2container.NixLayerAnnotation: "true",
				"labelToBeIgnored":               "ValueToBeIgnored",
				"labelToBeIgnored2":              "ValueToBeIgnored2",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			key := "test"
			root := t.TempDir()
			nixStorePrefix := "/nix/store"
			snapshotterFunc := newSnapshotterWithOpts(nixStorePrefix)
			snapshotter, _, err := snapshotterFunc(ctx, root)
			if err != nil {
				t.Fatal(err)
			}

			labels := map[string]string{}
			for idx, value := range tc.nixStores {
				labels[nix2container.NixStorePrefixAnnotation+strconv.Itoa(idx)] = value
			}
			for idx, value := range tc.extraLabels {
				labels[idx] = value
			}

			_, err = snapshotter.Prepare(ctx, key, "", snapshots.WithLabels(labels))
			if err != nil {
				t.Fatal(err)
			}

			testBindMounts(t, ctx, snapshotter, key, nixStorePrefix, tc.nixStores)
			testGCRoots(t, ctx, snapshotter, key, nixStorePrefix, tc.nixStores, root, labels)
		})
	}
}

func testBindMounts(t *testing.T, ctx context.Context, snapshotter snapshots.Snapshotter, key string, nixStorePrefix string, nixStores []string) {
	mounts, err := snapshotter.(*nixSnapshotter).withNixBindMounts(ctx, key, []mount.Mount{})
	if err != nil {
		t.Fatal(err)
	}

	expectedMounts := []mount.Mount{}
	for _, nixStore := range nixStores {
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

func getStoreID(t *testing.T, ctx context.Context, snapshotter snapshots.Snapshotter, key string) string {
	ctx, transactor, err := snapshotter.(*nixSnapshotter).ms.TransactionContext(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err = transactor.Rollback()
		if err != nil {
			t.Fatal(err)
		}
	}()
	id, _, _, err := storage.GetInfo(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func testGCRoots(t *testing.T, ctx context.Context, snapshotter snapshots.Snapshotter, key string, nixStorePrefix string, nixStores []string, root string, labels map[string]string) {
	// Mock builder to test inputs
	var nixToolInputs, filepathInputs, nixPathInputs []string
	testBuilder := func(config *nixGCOptConfig) error {
		config.builder = func(nixTool string, filepath string, nixPath string) ([]byte, error) {
			nixToolInputs = append(nixToolInputs, nixTool)
			filepathInputs = append(filepathInputs, filepath)
			nixPathInputs = append(nixPathInputs, nixPath)
			return []byte{}, nil

		}
		return nil
	}

	err := snapshotter.(*nixSnapshotter).prepareNixGCRoots(ctx, key, labels, testBuilder)
	if err != nil {
		t.Fatal(err)
	}

	id := getStoreID(t, ctx, snapshotter, key)

	for idx := range nixToolInputs {
		testutil.IsIdentical(t, nixToolInputs[idx], "nix")
		testutil.IsIdentical(t, filepathInputs[idx], filepath.Join(root, "gcroots", id, nixStores[idx]))
		testutil.IsIdentical(t, nixPathInputs[idx], filepath.Join(nixStorePrefix, nixStores[idx]))
	}
}
