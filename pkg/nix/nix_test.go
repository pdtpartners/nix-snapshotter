package nix

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/containerd/containerd/mount"
	"github.com/containerd/containerd/snapshots"
	"github.com/containerd/containerd/snapshots/overlay"
	"github.com/containerd/containerd/snapshots/testsuite"
	"github.com/docker/docker/daemon/graphdriver/overlayutils"
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

func TestNixSnapshotter(t *testing.T) {
	optTestCases := map[string][]interface{}{
		"no opt":             nil,
		"AsynchronousRemove": {overlay.AsynchronousRemove},
		"FuseOverlayFs":      {WithFuseOverlayfs},
	}
	for optsName, opts := range optTestCases {
		t.Run(optsName, func(t *testing.T) {
			newSnapshotter := newSnapshotterWithOpts("", opts...)
			t.Run("TestNixRemove", func(t *testing.T) {
				testNixRemove(t, newSnapshotter)
			})
			t.Run("TestNixView", func(t *testing.T) {
				testNixView(t, newSnapshotter)
			})
		})
	}
}

func testNixRemove(t *testing.T, newSnapshotter testsuite.SnapshotterFunc) {
	ctx := context.TODO()
	root := t.TempDir()
	o, _, err := newSnapshotter(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	labels := make(map[string]string)
	labels[nix2container.NixLayerAnnotation] = "true"
	labels[nix2container.NixStorePrefixAnnotation] = "/nix/store"
	key := "/nix/store/base"
	mounts, err := o.Prepare(ctx, key, "", snapshots.WithLabels(labels))
	if err != nil {
		t.Fatal(err)
	}
	m := mounts[0]
	if err := os.WriteFile(filepath.Join(m.Source, "foo"), []byte("hi"), 0660); err != nil {
		t.Fatal(err)
	}
	err = o.Remove(ctx, key)
	if err != nil {
		panic(err)
	}
	_, err = o.View(ctx, "view1", "base")
	if err == nil {
		t.Fatal(fmt.Errorf("viewed snapshot that has been removed"))
	}
	if _, err := os.ReadFile(filepath.Join(m.Source, "foo")); err == nil {
		t.Fatal(fmt.Errorf("written file was not removed"))
	}
	err = o.(*nixSnapshotter).Cleanup(ctx)
	if err != nil {
		t.Fatal(err)
	}
}

func testNixView(t *testing.T, newSnapshotter testsuite.SnapshotterFunc) {
	ctx := context.TODO()
	root := t.TempDir()
	o, _, err := newSnapshotter(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	labels := make(map[string]string)
	labels[nix2container.NixLayerAnnotation] = "true"
	labels[nix2container.NixStorePrefixAnnotation] = "/nix/store"
	key := "/nix/store/base"
	mounts, err := o.Prepare(ctx, key, "", snapshots.WithLabels(labels))
	if err != nil {
		t.Fatal(err)
	}
	m := mounts[0]
	if err := os.WriteFile(filepath.Join(m.Source, "foo"), []byte("hi"), 0660); err != nil {
		t.Fatal(err)
	}
	if err := o.Commit(ctx, "base", key); err != nil {
		t.Fatal(err)
	}
	labels = make(map[string]string)
	labels[nix2container.NixStorePrefixAnnotation] = "/nix/store"
	key = "/nix/store/top"
	_, err = o.Prepare(ctx, key, "base", snapshots.WithLabels(labels))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(getParents(ctx, o, root, "/nix/store/top")[0], "foo"), []byte("hi, again"), 0660); err != nil {
		t.Fatal(err)
	}
	if err := o.Commit(ctx, "top", key); err != nil {
		t.Fatal(err)
	}

	mounts, err = o.View(ctx, "view1", "base", snapshots.WithLabels(labels))
	if err != nil {
		t.Fatal(err)
	}

	sort.Strings(mounts[0].Options)
	sort.Strings(mounts[1].Options)

	expected := []mount.Mount{
		{
			Type:    "bind",
			Source:  filepath.Join(root, "snapshots", "1", "fs"),
			Options: []string{"rbind", "ro"},
		},
		{
			Type:    "bind",
			Source:  labels[nix2container.NixStorePrefixAnnotation],
			Target:  labels[nix2container.NixStorePrefixAnnotation],
			Options: []string{"rbind", "ro"},
		},
	}
	testutil.IsIdentical(t, mounts, expected)

	mounts, err = o.View(ctx, "view2", "top", snapshots.WithLabels(labels))
	if err != nil {
		t.Fatal(err)
	}

	mountOptions := mounts[0].Options
	mounts[0].Options = nil

	lowers := getParents(ctx, o, root, "view2")
	expectedOptions := []string{fmt.Sprintf("lowerdir=%s:%s", lowers[0], lowers[1])}
	userxattr, err := overlayutils.NeedsUserXAttr(root)
	if err != nil {
		t.Fatal(err)
	}
	if userxattr {
		expectedOptions = append(expectedOptions, "userxattr")
	}
	if supportsIndex() {
		expectedOptions = append(expectedOptions, "index=off")
	}
	sort.Strings(mountOptions)
	sort.Strings(expectedOptions)
	sort.Strings(mounts[1].Options)

	testutil.IsIdentical(t, mountOptions, expectedOptions)

	expected = []mount.Mount{
		{
			Type:   "overlay",
			Source: "overlay",
		},
		{
			Type:    "bind",
			Source:  labels[nix2container.NixStorePrefixAnnotation],
			Target:  labels[nix2container.NixStorePrefixAnnotation],
			Options: []string{"rbind", "ro"},
		},
	}
	testutil.IsIdentical(t, mounts, expected)
}
