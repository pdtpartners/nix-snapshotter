package nix

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/containerd/containerd/mount"
	"github.com/containerd/containerd/pkg/testutil"
	"github.com/containerd/containerd/snapshots"
	"github.com/containerd/containerd/snapshots/overlay"
	"github.com/containerd/containerd/snapshots/storage"
	"github.com/containerd/containerd/snapshots/testsuite"
	"github.com/docker/docker/daemon/graphdriver/overlayutils"
	"github.com/pdtpartners/nix-snapshotter/pkg/nix2container"
	"github.com/stretchr/testify/require"
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

func TestNixWithSnaphotterSuite(t *testing.T) {
	testutil.RequiresRoot(t)
	optTestCases := map[string][]interface{}{
		"no opt": nil,
		// default in init()
		"AsynchronousRemove": {overlay.AsynchronousRemove},
		"FuseOverlayFs":      {WithFuseOverlayfs},
	}
	for optsName, opts := range optTestCases {
		t.Run(optsName, func(t *testing.T) {
			newSnapshotter := newSnapshotterWithOpts("/nix/store", opts...)
			// The Nix-Snapshotter pass the overlayfs profile of tests
			testsuite.SnapshotterSuite(t, "overlayfs", newSnapshotter)
		})
	}
}

func TestNix(t *testing.T) {
	optTestCases := map[string][]interface{}{
		"no opt": nil,
		// default in init()
		"AsynchronousRemove": {overlay.AsynchronousRemove},
		"FuseOverlayFs":      {WithFuseOverlayfs},
	}
	for optsName, opts := range optTestCases {
		t.Run(optsName, func(t *testing.T) {
			newSnapshotter := newSnapshotterWithOpts("", opts...)
			t.Run("TestNixMounts", func(t *testing.T) {
				testNixMounts(t, newSnapshotter)
			})
			t.Run("TestNixRemove", func(t *testing.T) {
				testNixRemove(t, newSnapshotter)
			})
			t.Run("TestNixView", func(t *testing.T) {
				testNixView(t, newSnapshotter)
			})
			// Legacy tests inherited from https://github.com/containerd/containerd/blob/main/snapshots/overlay/overlay_test.go
			t.Run("TestNonNixMounts", func(t *testing.T) {
				testNonNixMounts(t, newSnapshotter)
			})
			t.Run("TestNonNixCommit", func(t *testing.T) {
				testNonNixCommit(t, newSnapshotter)
			})
			t.Run("TestNonNixView", func(t *testing.T) {
				testNonNixView(t, newSnapshotterWithOpts("", append(opts, overlay.WithMountOptions([]string{"volatile"}))...))
			})
			t.Run("TestNonNixOverlayMount", func(t *testing.T) {
				testNonNixOverlayMount(t, newSnapshotter)
			})
			t.Run("TestNonNixOverlayRead", func(t *testing.T) {
				testNonNixOverlayRead(t, newSnapshotter)
			})

		})
	}
}

func testNixMounts(t *testing.T, newSnapshotter testsuite.SnapshotterFunc) {
	// 	ctx := context.TODO()
	// 	root := t.TempDir()
	// 	o, _, err := newSnapshotter(ctx, root)
	// 	if err != nil {
	// 		t.Fatal(err)
	// 	}
	// 	labels := make(map[string]string)
	// 	labels[nix2container.NixLayerAnnotation] = "some string"
	// 	labels[nix2container.NixStorePrefixAnnotation] = "/nix/store"
	// 	key := "/nix/store/base"
	// 	mounts, err := o.Prepare(ctx, key, "", snapshots.WithLabels(labels))
	// 	if err != nil {
	// 		t.Fatal(err)
	// 	}
	// m := mounts[0]
	// if err := os.WriteFile(filepath.Join(m.Source, "foo"), []byte("hi"), 0660); err != nil {
	// 	t.Fatal(err)
	// }
	// mounts2, err := o.Mounts(ctx, key)
	// if err != nil {
	// 	t.Fatal(err)
	// }

	// if len(mounts) != len(mounts2) {
	// 	t.Errorf("Different number of mounts returned from Prepare():%v and Mounts():%v", len(mounts), len(mounts2))
	// }
	// if m.Type != "bind" {
	// 	t.Errorf("mount type should be bind but received %q", m.Type)
	// }
	// m = mounts2[1]
	// if m.Type != "bind" {
	// 	t.Errorf("mount type should be bind but received %q", m.Type)
	// }
}

func testNixRemove(t *testing.T, newSnapshotter testsuite.SnapshotterFunc) {
	ctx := context.TODO()
	root := t.TempDir()
	o, _, err := newSnapshotter(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	labels := make(map[string]string)
	labels[nix2container.NixLayerAnnotation] = "some string"
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
	_, err = o.View(ctx, "view1", "base", snapshots.WithLabels(labels))
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
	labels[nix2container.NixLayerAnnotation] = "some string"
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
	if len(mounts) != 2 {
		t.Fatalf("should only have 1 mount but received %d", len(mounts))
	}
	m = mounts[0]
	if m.Type != "bind" {
		t.Errorf("mount type should be bind but received %q", m.Type)
	}
	m = mounts[1]
	if m.Type != "bind" {
		t.Errorf("mount type should be bind but received %q", m.Type)
	}
	expected := labels[nix2container.NixStorePrefixAnnotation]
	if m.Source != expected {
		t.Errorf("expected source %q but received %q", expected, m.Source)
	}
	if m.Options[0] != "ro" {
		t.Errorf("expected mount option ro but received %q", m.Options[0])
	}
	if m.Options[1] != "rbind" {
		t.Errorf("expected mount option rbind but received %q", m.Options[1])
	}

	mounts, err = o.View(ctx, "view2", "top", snapshots.WithLabels(labels))
	if err != nil {
		t.Fatal(err)
	}
	if len(mounts) != 2 {
		t.Fatalf("should only have 1 mount but received %d", len(mounts))
	}
	m = mounts[0]
	if m.Type != "overlay" {
		t.Errorf("mount type should be overlay but received %q", m.Type)
	}
	if m.Source != "overlay" {
		t.Errorf("mount source should be overlay but received %q", m.Source)
	}
	m = mounts[1]
	if m.Type != "bind" {
		t.Errorf("mount type should be bind but received %q", m.Type)
	}
	if m.Source != labels[nix2container.NixStorePrefixAnnotation] {
		t.Errorf("mount source should be %q but received %q", labels[nix2container.NixStorePrefixAnnotation], m.Source)
	}

	supportsIndex := supportsIndex()
	expectedOptions := 2
	if !supportsIndex {
		expectedOptions--
	}
	userxattr, err := overlayutils.NeedsUserXAttr(root)
	if err != nil {
		t.Fatal(err)
	}
	if userxattr {
		expectedOptions++
	}

	if len(m.Options) != expectedOptions {
		t.Errorf("expected %d additional mount option but got %d", expectedOptions, len(m.Options))
	}
	expected = "rbind"
	optIdx := 1
	if !supportsIndex {
		optIdx--
	}
	if userxattr {
		optIdx++
	}
	if m.Options[optIdx] != expected {
		t.Errorf("expected option %q but received %q", expected, m.Options[optIdx])
	}
}

func testNonNixMounts(t *testing.T, newSnapshotter testsuite.SnapshotterFunc) {
	ctx := context.Background()
	root := t.TempDir()
	snapshotter, _, err := newSnapshotter(ctx, root)
	require.NoError(t, err)

	mounts, err := snapshotter.Prepare(ctx, root, "")
	require.NoError(t, err)

	if len(mounts) != 1 {
		t.Errorf("should only have 1 mount but received %d", len(mounts))
	}
	m := mounts[0]
	if m.Type != "bind" {
		t.Errorf("mount type should be bind but received %q", m.Type)
	}
	expected := filepath.Join(root, "snapshots", "1", "fs")
	if m.Source != expected {
		t.Errorf("expected source %q but received %q", expected, m.Source)
	}
	if m.Options[0] != "rw" {
		t.Errorf("expected mount option rw but received %q", m.Options[0])
	}
	if m.Options[1] != "rbind" {
		t.Errorf("expected mount option rbind but received %q", m.Options[1])
	}
}

func testNonNixCommit(t *testing.T, newSnapshotter testsuite.SnapshotterFunc) {
	ctx := context.TODO()
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

func testNonNixView(t *testing.T, newSnapshotter testsuite.SnapshotterFunc) {
	ctx := context.TODO()
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
	if len(mounts) != 1 {
		t.Fatalf("should only have 1 mount but received %d", len(mounts))
	}
	m = mounts[0]
	if m.Type != "bind" {
		t.Errorf("mount type should be bind but received %q", m.Type)
	}
	expected := getParents(ctx, o, root, "/tmp/view1")[0]
	if m.Source != expected {
		t.Errorf("expected source %q but received %q", expected, m.Source)
	}
	if m.Options[0] != "ro" {
		t.Errorf("expected mount option ro but received %q", m.Options[0])
	}
	if m.Options[1] != "rbind" {
		t.Errorf("expected mount option rbind but received %q", m.Options[1])
	}

	mounts, err = o.View(ctx, "/tmp/view2", "top")
	if err != nil {
		t.Fatal(err)
	}
	if len(mounts) != 1 {
		t.Fatalf("should only have 1 mount but received %d", len(mounts))
	}
	m = mounts[0]
	if m.Type != "overlay" {
		t.Errorf("mount type should be overlay but received %q", m.Type)
	}
	if m.Source != "overlay" {
		t.Errorf("mount source should be overlay but received %q", m.Source)
	}

	supportsIndex := supportsIndex()
	expectedOptions := 3
	if !supportsIndex {
		expectedOptions--
	}
	userxattr, err := overlayutils.NeedsUserXAttr(root)
	if err != nil {
		t.Fatal(err)
	}
	if userxattr {
		expectedOptions++
	}

	if len(m.Options) != expectedOptions {
		t.Errorf("expected %d additional mount option but got %d", expectedOptions, len(m.Options))
	}
	lowers := getParents(ctx, o, root, "/tmp/view2")
	expected = fmt.Sprintf("lowerdir=%s:%s", lowers[0], lowers[1])
	optIdx := 2
	if !supportsIndex {
		optIdx--
	}
	if userxattr {
		optIdx++
	}
	if m.Options[0] != "volatile" {
		t.Error("expected option first option to be provided option \"volatile\"")
	}
	if m.Options[optIdx] != expected {
		t.Errorf("expected option %q but received %q", expected, m.Options[optIdx])
	}
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

func testNonNixOverlayMount(t *testing.T, newSnapshotter testsuite.SnapshotterFunc) {
	ctx := context.TODO()
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
	if len(mounts) != 1 {
		t.Errorf("should only have 1 mount but received %d", len(mounts))
	}
	m := mounts[0]
	if m.Type != "overlay" {
		t.Errorf("mount type should be overlay but received %q", m.Type)
	}
	if m.Source != "overlay" {
		t.Errorf("expected source %q but received %q", "overlay", m.Source)
	}
	// var (
	// 	bp    = getBasePath(ctx, o, root, "/tmp/layer2")
	// 	work  = "workdir=" + filepath.Join(bp, "work")
	// 	upper = "upperdir=" + filepath.Join(bp, "fs")
	// 	lower = "lowerdir=" + getParents(ctx, o, root, "/tmp/layer2")[0]
	// )

	// expected := []string{}
	// if !supportsIndex() {
	// 	expected = expected[1:]
	// }
	// if userxattr, err := overlayutils.NeedsUserXAttr(root); err != nil {
	// 	t.Fatal(err)
	// } else if userxattr {
	// 	expected = append(expected, "userxattr")
	// }

	// expected = append(expected, "index=off")
	// expected = append(expected, []string{
	// 	work,
	// 	upper,
	// 	lower,
	// }...)
	// for i, v := range expected {
	// 	if m.Options[i] != v {
	// 		t.Errorf("expected %q but received %q", v, m.Options[i])
	// 	}
	// }
}

// func getBasePath(ctx context.Context, sn snapshots.Snapshotter, root, key string) string {
// 	o := sn.(*nixSnapshotter)
// 	ctx, t, err := o.ms.TransactionContext(ctx, false)
// 	if err != nil {
// 		panic(err)
// 	}
// 	defer func() {
// 		err = t.Rollback()
// 		if err != nil {
// 			panic(err)
// 		}
// 	}()

// 	s, err := storage.GetSnapshot(ctx, key)
// 	if err != nil {
// 		panic(err)
// 	}

// 	return filepath.Join(root, "snapshots", s.ID)
// }

func testNonNixOverlayRead(t *testing.T, newSnapshotter testsuite.SnapshotterFunc) {
	testutil.RequiresRoot(t)
	ctx := context.TODO()
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
