package nix

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"testing"

	"github.com/containerd/containerd/mount"
	"github.com/containerd/containerd/snapshots"
	"github.com/containerd/containerd/snapshots/storage"
	"github.com/containerd/containerd/snapshots/testsuite"
	"github.com/pdtpartners/nix-snapshotter/pkg/nix2container"
	"github.com/pdtpartners/nix-snapshotter/pkg/testutil"
)

func newSnapshotterWithOpts(nixStore string, opts ...interface{}) testsuite.SnapshotterFunc {
	return func(ctx context.Context, root string) (snapshots.Snapshotter, func() error, error) {
		snapshotter, err := NewSnapshotter(root, nixStore, opts...)
		if err != nil {
			return nil, nil, err
		}

		return snapshotter, func() error { return snapshotter.Close() }, nil
	}
}

func TestWithNixBindMounts(t *testing.T) {
	type testCase struct {
		name   string
		labels map[string]string
		// expectedMounts []mount.Mount
	}

	for _, tc := range []testCase{
		{
			name: "placeholder",
			labels: map[string]string{
				nix2container.NixLayerAnnotation:             "true",
				nix2container.NixStorePrefixAnnotation + "1": "34xlpp3j3vy7ksn09zh44f1c04w77khf-libunistring-1.0",
				nix2container.NixStorePrefixAnnotation + "2": "4nlgxhb09sdr51nc9hdm8az5b08vzkgx-glibc-2.35-163",
				nix2container.NixStorePrefixAnnotation + "3": "5mh5019jigj0k14rdnjam1xwk5avn1id-libidn2-2.3.2",
				nix2container.NixStorePrefixAnnotation + "4": "g2m8kfw7kpgpph05v2fxcx4d5an09hl3-hello-2.12.1",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			key := "test"
			root := t.TempDir()
			nixStore := "/nix/store"
			snapshotterFunc := newSnapshotterWithOpts(nixStore)
			snapshotter, _, err := snapshotterFunc(ctx, root)
			if err != nil {
				t.Fatal(err)
			}
			_, err = snapshotter.Prepare(ctx, key, "", snapshots.WithLabels(tc.labels))
			if err != nil {
				t.Fatal(err)
			}
			mounts, err := snapshotter.(*nixSnapshotter).withNixBindMounts(ctx, key, []mount.Mount{})
			if err != nil {
				t.Fatal(err)
			}
			keys := []string{}
			expectedMounts := []mount.Mount{}
			for key := range tc.labels {
				if key != nix2container.NixLayerAnnotation {
					keys = append(keys, key)
				}
			}
			sort.Strings(keys)
			for _, key := range keys {
				expectedMounts = append(expectedMounts,
					mount.Mount{
						Type:    "bind",
						Source:  filepath.Join(nixStore, tc.labels[key]),
						Target:  filepath.Join(nixStore, tc.labels[key]),
						Options: []string{"ro", "rbind"},
					})
			}
			testutil.IsIdentical(t, mounts, expectedMounts)
		})
	}

}

func TestPrepareNixGCRoots(t *testing.T) {
	type testCase struct {
		name   string
		labels []string
	}

	for _, tc := range []testCase{
		{
			name: "placeholder",
			labels: []string{
				"34xlpp3j3vy7ksn09zh44f1c04w77khf-libunistring-1.0",
				"4nlgxhb09sdr51nc9hdm8az5b08vzkgx-glibc-2.35-163",
				"5mh5019jigj0k14rdnjam1xwk5avn1id-libidn2-2.3.2",
				"g2m8kfw7kpgpph05v2fxcx4d5an09hl3-hello-2.12.1",
			},
			//ADD EDGE CASE WITH STUFF THAT SHOULD BE IGNORED
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			key := "test"
			root := t.TempDir()
			nixStore := "/nix/store"
			snapshotterFunc := newSnapshotterWithOpts(nixStore)
			snapshotter, _, err := snapshotterFunc(ctx, root)

			labels := map[string]string{}

			for idx, value := range tc.labels {
				labels[nix2container.NixStorePrefixAnnotation+strconv.Itoa(idx)] = value
			}

			if err != nil {
				t.Fatal(err)
			}
			_, err = snapshotter.Prepare(ctx, key, "", snapshots.WithLabels(labels))
			if err != nil {
				t.Fatal(err)
			}

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

			err = snapshotter.(*nixSnapshotter).prepareNixGCRoots(ctx, key, labels, testBuilder)
			if err != nil {
				t.Fatal(err)
			}

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

			for idx := range nixToolInputs {
				testutil.IsIdentical(t, nixToolInputs[idx], "nix")
				testutil.IsIdentical(t, filepathInputs[idx], filepath.Join(root, "gcroots", id, tc.labels[idx]))
				testutil.IsIdentical(t, nixPathInputs[idx], filepath.Join(nixStore, tc.labels[idx]))

			}

			fmt.Printf("TEST_DATA: \n%v\n %v\n %v\n", nixToolInputs, filepathInputs, nixPathInputs)
		})
	}

}
