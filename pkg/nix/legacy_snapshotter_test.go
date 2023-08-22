package nix

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"testing"

	"github.com/containerd/containerd/mount"
	"github.com/containerd/containerd/snapshots"
	"github.com/containerd/containerd/snapshots/overlay"
	"github.com/containerd/containerd/snapshots/storage"
	"github.com/containerd/containerd/snapshots/testsuite"
	"github.com/docker/docker/daemon/graphdriver/overlayutils"
	"github.com/pdtpartners/nix-snapshotter/pkg/testutil"
	"github.com/stretchr/testify/require"
)

// Legacy tests adapted from https://github.com/containerd/containerd/blob/main/snapshots/overlay/overlay_test.go

func TestNixSnapshotterWithSnaphotterSuite(t *testing.T) {
	testutil.RequiresRoot(t)
	optTestCases := map[string][]interface{}{
		"no opt":             nil,
		"AsynchronousRemove": {overlay.AsynchronousRemove},
		"FuseOverlayFs":      {WithFuseOverlayfs},
	}
	for optsName, opts := range optTestCases {
		t.Run(optsName, func(t *testing.T) {
			newSnapshotter := newSnapshotterWithOpts("/nix/store", opts...)
			// The Nix-Snapshotter passes the overlayfs profile of tests
			testsuite.SnapshotterSuite(t, "overlayfs", newSnapshotter)
		})
	}
}

func TestSnapshotter(t *testing.T) {
	optTestCases := map[string][]interface{}{
		"no opt":             nil,
		"AsynchronousRemove": {overlay.AsynchronousRemove},
		"FuseOverlayFs":      {WithFuseOverlayfs},
	}
	for optsName, opts := range optTestCases {
		t.Run(optsName, func(t *testing.T) {
			newSnapshotter := newSnapshotterWithOpts("", opts...)
			t.Run("TestNonNixMounts", func(t *testing.T) {
				testSnapshotterMounts(t, newSnapshotter)
			})
			t.Run("TestNonNixCommit", func(t *testing.T) {
				testSnapshotterCommit(t, newSnapshotter)
			})
			t.Run("TestNonNixView", func(t *testing.T) {
				testSnapshotterView(t, newSnapshotterWithOpts("", append(opts, overlay.WithMountOptions([]string{"volatile"}))...))
			})
			t.Run("TestNonNixOverlayMount", func(t *testing.T) {
				testSnapshotterOverlayMount(t, newSnapshotter)
			})
			t.Run("TestNonNixOverlayRead", func(t *testing.T) {
				testSnapshotterOverlayRead(t, newSnapshotter)
			})

		})
	}
}
func testSnapshotterMounts(t *testing.T, newSnapshotter testsuite.SnapshotterFunc) {
	ctx := context.Background()
	root := t.TempDir()
	snapshotter, _, err := newSnapshotter(ctx, root)
	require.NoError(t, err)

	mounts, err := snapshotter.Prepare(ctx, root, "")
	require.NoError(t, err)

	sort.Strings(mounts[0].Options)

	expected := []mount.Mount{
		{
			Type:    "bind",
			Source:  filepath.Join(root, "snapshots", "1", "fs"),
			Options: []string{"rbind", "rw"},
		},
	}
	testutil.IsIdentical(t, mounts, expected)
}

func testSnapshotterCommit(t *testing.T, newSnapshotter testsuite.SnapshotterFunc) {
	ctx := context.Background()
	root := t.TempDir()
	o, _, err := newSnapshotter(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	key := "/tmp/test"
	mounts, err := o.Prepare(ctx, key, "")
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
}

func testSnapshotterView(t *testing.T, newSnapshotter testsuite.SnapshotterFunc) {
	ctx := context.Background()
	root := t.TempDir()
	o, _, err := newSnapshotter(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	key := "/tmp/base"
	mounts, err := o.Prepare(ctx, key, "")
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

	key = "/tmp/top"
	_, err = o.Prepare(ctx, key, "base")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(getParents(ctx, o, root, "/tmp/top")[0], "foo"), []byte("hi, again"), 0660); err != nil {
		t.Fatal(err)
	}
	if err := o.Commit(ctx, "top", key); err != nil {
		t.Fatal(err)
	}

	mounts, err = o.View(ctx, "/tmp/view1", "base")
	if err != nil {
		t.Fatal(err)
	}

	expected := []mount.Mount{
		{
			Type:    "bind",
			Source:  getParents(ctx, o, root, "/tmp/view1")[0],
			Options: []string{"ro", "rbind"},
		},
	}
	testutil.IsIdentical(t, mounts, expected)

	mounts, err = o.View(ctx, "/tmp/view2", "top")
	if err != nil {
		t.Fatal(err)
	}
	lowers := getParents(ctx, o, root, "/tmp/view2")

	mountOptions := mounts[0].Options

	expectedOptions := []string{"volatile", fmt.Sprintf("lowerdir=%s:%s", lowers[0], lowers[1])}
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

	sort.Strings(expectedOptions)
	sort.Strings(mountOptions)
	testutil.IsIdentical(t, mountOptions, expectedOptions)

	mounts[0].Options = nil
	expected = []mount.Mount{
		{
			Type:   "overlay",
			Source: "overlay",
		},
	}

	testutil.IsIdentical(t, mounts, expected)
}

func getParents(ctx context.Context, sn snapshots.Snapshotter, root, key string) []string {
	o := sn.(*nixSnapshotter)
	ctx, t, err := o.ms.TransactionContext(ctx, false)
	if err != nil {
		panic(err)
	}
	defer func() {
		err = t.Rollback()
		if err != nil {
			panic(err)
		}
	}()

	s, err := storage.GetSnapshot(ctx, key)
	if err != nil {
		panic(err)
	}
	parents := make([]string, len(s.ParentIDs))
	for i := range s.ParentIDs {
		parents[i] = filepath.Join(root, "snapshots", s.ParentIDs[i], "fs")
	}
	return parents
}

// supportsIndex checks whether the "index=off" option is supported by the kernel.
func supportsIndex() bool {
	if _, err := os.Stat("/sys/module/overlay/parameters/index"); err == nil {
		return true
	}
	return false
}

func testSnapshotterOverlayMount(t *testing.T, newSnapshotter testsuite.SnapshotterFunc) {
	ctx := context.Background()
	root := t.TempDir()
	o, _, err := newSnapshotter(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	key := "/tmp/test"
	if _, err = o.Prepare(ctx, key, ""); err != nil {
		t.Fatal(err)
	}
	if err := o.Commit(ctx, "base", key); err != nil {
		t.Fatal(err)
	}
	var mounts []mount.Mount
	if mounts, err = o.Prepare(ctx, "/tmp/layer2", "base"); err != nil {
		t.Fatal(err)
	}

	mountOptions := mounts[0].Options

	var bp = getBasePath(ctx, o, root, "/tmp/layer2")

	expectedOptions := []string{
		"workdir=" + filepath.Join(bp, "work"),
		"upperdir=" + filepath.Join(bp, "fs"),
		"lowerdir=" + getParents(ctx, o, root, "/tmp/layer2")[0]}
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

	sort.Strings(expectedOptions)
	sort.Strings(mountOptions)
	testutil.IsIdentical(t, mountOptions, expectedOptions)

	mounts[0].Options = nil
	expected := []mount.Mount{
		{
			Type:   "overlay",
			Source: "overlay",
		},
	}

	testutil.IsIdentical(t, mounts, expected)
}

func getBasePath(ctx context.Context, sn snapshots.Snapshotter, root, key string) string {
	o := sn.(*nixSnapshotter)
	ctx, t, err := o.ms.TransactionContext(ctx, false)
	if err != nil {
		panic(err)
	}
	defer func() {
		err = t.Rollback()
		if err != nil {
			panic(err)
		}
	}()

	s, err := storage.GetSnapshot(ctx, key)
	if err != nil {
		panic(err)
	}

	return filepath.Join(root, "snapshots", s.ID)
}

func testSnapshotterOverlayRead(t *testing.T, newSnapshotter testsuite.SnapshotterFunc) {
	testutil.RequiresRoot(t)
	ctx := context.Background()
	root := t.TempDir()
	o, _, err := newSnapshotter(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	key := "/tmp/test"
	mounts, err := o.Prepare(ctx, key, "")
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
	if mounts, err = o.Prepare(ctx, "/tmp/layer2", "base"); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, "dest")
	if err := os.Mkdir(dest, 0700); err != nil {
		t.Fatal(err)
	}
	if err := mount.All(mounts, dest); err != nil {
		t.Fatal(err)
	}
	defer func() {
		err = syscall.Unmount(dest, 0)
		if err != nil {
			panic(err)
		}
	}()
	data, err := os.ReadFile(filepath.Join(dest, "foo"))
	if err != nil {
		t.Fatal(err)
	}
	if e := string(data); e != "hi" {
		t.Fatalf("expected file contents hi but got %q", e)
	}
}