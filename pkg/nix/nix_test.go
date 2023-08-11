package nix

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/containerd/containerd/snapshots"
	"github.com/containerd/containerd/snapshots/storage"
	"github.com/containerd/containerd/snapshots/testsuite"
	"github.com/stretchr/testify/require"
)

func newSnapshotterWithOpts(nixStore string, opts ...Opt) testsuite.SnapshotterFunc {
	return func(ctx context.Context, root string) (snapshots.Snapshotter, func() error, error) {
		snapshotter, err := NewSnapshotter(root, nixStore, opts...)
		if err != nil {
			return nil, nil, err
		}

		return snapshotter, func() error { return snapshotter.Close() }, nil
	}
}

// Require root is currently broken on my machine

// func TestNixWithSnaphotterSuite(t *testing.T) {
// 	testutil.RequiresRoot(t)
// 	optTestCases := map[string][]Opt{
// 		"no opt": nil,
// 		// default in init()
// 		"AsynchronousRemove": {AsynchronousRemove},
// 	}
// 	for optsName, opts := range optTestCases {
// 		t.Run(optsName, func(t *testing.T) {
// 			newSnapshotter := newSnapshotterWithOpts("", opts...)
// 			//Changing to nix here breaks tests
// 			testsuite.SnapshotterSuite(t, "overlayfs", newSnapshotter)
// 		})
// 	}
// }

func TestNix(t *testing.T) {
	optTestCases := map[string][]Opt{
		"no opt": nil,
		// default in init()
		"AsynchronousRemove": {AsynchronousRemove},
	}
	for optsName, opts := range optTestCases {
		t.Run(optsName, func(t *testing.T) {
			newSnapshotter := newSnapshotterWithOpts("", opts...)
			t.Run("TestOverlayMounts", func(t *testing.T) {
				testNixMounts(t, newSnapshotter)
			})
			t.Run("TestOverlayCommit", func(t *testing.T) {
				testNixCommit(t, newSnapshotter)
			})
			// t.Run("TestOverlayOverlayMount", func(t *testing.T) {
			// 	testOverlayOverlayMount(t, newSnapshotter)
			// })
			// t.Run("TestOverlayOverlayRead", func(t *testing.T) {
			// 	testOverlayOverlayRead(t, newSnapshotter)
			// })
			t.Run("TestOverlayView", func(t *testing.T) {
				// Fix when rebased with new version
				// testNixView(t, newSnapshotterWithOpts(append(opts, overlay.WithMountOptions([]string{"volatile"}))...))
				testNixView(t, newSnapshotter)
			})
		})
	}
}

func testNixMounts(t *testing.T, newSnapshotter testsuite.SnapshotterFunc) {
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

func testNixCommit(t *testing.T, newSnapshotter testsuite.SnapshotterFunc) {
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

func testNixView(t *testing.T, newSnapshotter testsuite.SnapshotterFunc) {
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

	//### UNCOMMENT when can support
	// supportsIndex := supportsIndex()
	// expectedOptions := 3
	// if !supportsIndex {
	// 	expectedOptions--
	// }
	// userxattr, err := overlayutils.NeedsUserXAttr(root)
	// if err != nil {
	// 	t.Fatal(err)
	// }
	// if userxattr {
	// 	expectedOptions++
	// }

	// if len(m.Options) != expectedOptions {
	// 	t.Errorf("expected %d additional mount option but got %d", expectedOptions, len(m.Options))
	// }
	// lowers := getParents(ctx, o, root, "/tmp/view2")
	// expected = fmt.Sprintf("lowerdir=%s:%s", lowers[0], lowers[1])
	// optIdx := 2
	// if !supportsIndex {
	// 	optIdx--
	// }
	// if userxattr {
	// 	optIdx++
	// }
	// if m.Options[0] != "volatile" {
	// 	t.Error("expected option first option to be provided option \"volatile\"")
	// }
	// if m.Options[optIdx] != expected {
	// 	t.Errorf("expected option %q but received %q", expected, m.Options[optIdx])
	// }
}

func getBasePath(ctx context.Context, sn snapshots.Snapshotter, root, key string) string {
	o := sn.(*snapshotter)
	ctx, t, err := o.ms.TransactionContext(ctx, false)
	if err != nil {
		panic(err)
	}
	defer t.Rollback()

	s, err := storage.GetSnapshot(ctx, key)
	if err != nil {
		panic(err)
	}

	return filepath.Join(root, "snapshots", s.ID)
}

func getParents(ctx context.Context, sn snapshots.Snapshotter, root, key string) []string {
	o := sn.(*snapshotter)
	ctx, t, err := o.ms.TransactionContext(ctx, false)
	if err != nil {
		panic(err)
	}
	defer t.Rollback()
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

// func testNixView(t *testing.T) {}
