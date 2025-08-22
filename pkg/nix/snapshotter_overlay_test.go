// Forked from https://github.com/containerd/containerd/v2/blob/v1.7.2/snapshots/overlay/overlay_test.go
// Copyright The containerd Authors.
// Licensed under the Apache License, Version 2.0
// NOTICE: https://github.com/containerd/containerd/v2/blob/v1.7.2/NOTICE

package nix

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"testing"

	"github.com/containerd/containerd/v2/core/mount"
	"github.com/containerd/containerd/v2/core/snapshots"
	"github.com/containerd/containerd/v2/core/snapshots/storage"
	"github.com/containerd/containerd/v2/core/snapshots/testsuite"
	"github.com/containerd/containerd/v2/pkg/testutil"
	"github.com/containerd/containerd/v2/plugins/snapshots/overlay"
	"github.com/containerd/containerd/v2/plugins/snapshots/overlay/overlayutils"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
)

func TestNixSnapshotterWithSnaphotterSuite(t *testing.T) {
	testutil.RequiresRoot(t)
	optTestCases := map[string][]overlay.Opt{
		"no opt":             nil,
		"AsynchronousRemove": {overlay.AsynchronousRemove},
	}
	for optsName, overlayOpts := range optTestCases {
		opts := []SnapshotterOpt{WithOverlayOpts(overlayOpts...)}
		t.Run(optsName, func(t *testing.T) {
			newSnapshotter := newSnapshotterWithOpts(opts...)
			// The Nix-Snapshotter passes the overlayfs profile of tests
			testsuite.SnapshotterSuite(t, "overlayfs", newSnapshotter)
		})
	}
}

func TestSnapshotter(t *testing.T) {
	optTestCases := map[string][]overlay.Opt{
		"no opt":             nil,
		"AsynchronousRemove": {overlay.AsynchronousRemove},
	}
	for optsName, overlayOpts := range optTestCases {
		opts := []SnapshotterOpt{WithOverlayOpts(overlayOpts...)}
		t.Run(optsName, func(t *testing.T) {
			newSnapshotter := newSnapshotterWithOpts(opts...)
			t.Run("TestSnapshotterRemove", func(t *testing.T) {
				testSnapshotterRemove(t, newSnapshotter)
			})
			t.Run("TestSnapshotterMounts", func(t *testing.T) {
				testSnapshotterMounts(t, newSnapshotter)
			})
			t.Run("TestSnapshotterCommit", func(t *testing.T) {
				testSnapshotterCommit(t, newSnapshotter)
			})
			t.Run("TestSnapshotterView", func(t *testing.T) {
				testSnapshotterView(t, newSnapshotterWithOpts(
					append(opts, WithOverlayOpts(overlay.WithMountOptions([]string{"volatile"})))...),
				)
			})
			t.Run("TestSnapshotterOverlayMount", func(t *testing.T) {
				testSnapshotterOverlayMount(t, newSnapshotter)
			})
			t.Run("TestSnapshotterOverlayRead", func(t *testing.T) {
				testSnapshotterOverlayRead(t, newSnapshotter)
			})

		})
	}
}

func testSnapshotterRemove(t *testing.T, newSnapshotter testsuite.SnapshotterFunc) {
	ctx := context.TODO()
	root := t.TempDir()
	o, _, err := newSnapshotter(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	labels := make(map[string]string)
	key := "/tmp/base"
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
		t.Fatal(err)
	}
	_, err = o.View(ctx, "view1", "base")
	if err == nil {
		t.Fatal(fmt.Errorf("viewed snapshot that has been removed"))
	}
	if _, err := os.ReadFile(filepath.Join(m.Source, "foo")); err == nil {
		t.Fatal(fmt.Errorf("written file was not removed"))
	}
	err = o.(snapshots.Cleaner).Cleanup(ctx)
	if err != nil {
		t.Fatal(err)
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
	IsIdentical(t, mounts, expected)
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
	if err := os.WriteFile(filepath.Join(getParents(t, ctx, o, root, "/tmp/top")[0], "foo"), []byte("hi, again"), 0660); err != nil {
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
			Source:  getParents(t, ctx, o, root, "/tmp/view1")[0],
			Options: []string{"ro", "rbind"},
		},
	}
	IsIdentical(t, mounts, expected)

	mounts, err = o.View(ctx, "/tmp/view2", "top")
	if err != nil {
		t.Fatal(err)
	}
	lowers := getParents(t, ctx, o, root, "/tmp/view2")

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
	IsIdentical(t, mountOptions, expectedOptions)

	mounts[0].Options = nil
	expected = []mount.Mount{
		{
			Type:   "overlay",
			Source: "overlay",
		},
	}

	IsIdentical(t, mounts, expected)
}

func getParents(t *testing.T, ctx context.Context, sn snapshots.Snapshotter, root, key string) []string {
	o := sn.(*nixSnapshotter)
	ctx, transactor, err := o.ms.TransactionContext(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err = transactor.Rollback()
		if err != nil {
			t.Fatal(err)
		}
	}()

	s, err := storage.GetSnapshot(ctx, key)
	if err != nil {
		t.Fatal(err)
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

	var bp = getBasePath(t, ctx, o, root, "/tmp/layer2")

	expectedOptions := []string{
		"workdir=" + filepath.Join(bp, "work"),
		"upperdir=" + filepath.Join(bp, "fs"),
		"lowerdir=" + getParents(t, ctx, o, root, "/tmp/layer2")[0]}
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
	IsIdentical(t, mountOptions, expectedOptions)

	mounts[0].Options = nil
	expected := []mount.Mount{
		{
			Type:   "overlay",
			Source: "overlay",
		},
	}

	IsIdentical(t, mounts, expected)
}

func getBasePath(t *testing.T, ctx context.Context, sn snapshots.Snapshotter, root, key string) string {
	o := sn.(*nixSnapshotter)
	ctx, transactor, err := o.ms.TransactionContext(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err = transactor.Rollback()
		if err != nil {
			t.Fatal(err)
		}
	}()

	s, err := storage.GetSnapshot(ctx, key)
	if err != nil {
		t.Fatal(err)
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
			t.Fatal(err)
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

func IsIdentical(t *testing.T, x interface{}, y interface{}) {
	diff := cmp.Diff(x, y)
	if diff != "" {
		t.Fatalf(diff)
	}
}
