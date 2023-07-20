/*
   Copyright The containerd Authors.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package nix

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/containerd/containerd/log"
	"github.com/containerd/containerd/mount"
	"github.com/containerd/containerd/snapshots"
	"github.com/containerd/containerd/snapshots/storage"
	"github.com/pdtpartners/nix-snapshotter/pkg/nix2container"
	"github.com/pdtpartners/nix-snapshotter/pkg/overlayfork"
)

const (
	// upperdirKey is a key of an optional label to each snapshot.
	// This optional label of a snapshot contains the location of "upperdir" where
	// the change set between this snapshot and its parent is stored.
	upperdirKey = "containerd.io/snapshot/overlay.upperdir"

	labelSnapshotRef = "containerd.io/snapshot.ref"

	defaultNixTool = "nix"
)

// SnapshotterConfig is used to configure the overlay snapshotter instance
type SnapshotterConfig struct {
	asyncRemove   bool
	upperdirLabel bool
	fuse          bool
}

// Opt is an option to configure the overlay snapshotter
type Opt func(config *SnapshotterConfig) error

// AsynchronousRemove defers removal of filesystem content until
// the Cleanup method is called. Removals will make the snapshot
// referred to by the key unavailable and make the key immediately
// available for re-use.
func AsynchronousRemove(config *SnapshotterConfig) error {
	config.asyncRemove = true
	return nil
}

// WithUpperdirLabel adds as an optional label
// "containerd.io/snapshot/overlay.upperdir". This stores the location
// of the upperdir that contains the changeset between the labelled
// snapshot and its parent.
func WithUpperdirLabel(config *SnapshotterConfig) error {
	config.upperdirLabel = true
	return nil
}

// WithFuseOverlayfs changes the overlay mount type used to fuse-overlayfs, an
// FUSE implementation for overlayfs.
//
// See: https://github.com/containers/fuse-overlayfs
func WithFuseOverlayfs(config *SnapshotterConfig) error {
	config.fuse = true
	return nil
}

type nixSnapshotter struct {
	overlayfork.Snapshotter
	fuse        bool
	nixStoreDir string
}

// NewSnapshotter returns a Snapshotter which uses overlayfs. The overlayfs
// diffs are stored under the provided root. A metadata file is stored under
// the root.
func NewSnapshotter(root, nixStoreDir string, opts ...Opt) (snapshots.Snapshotter, error) {
	var config SnapshotterConfig
	for _, opt := range opts {
		if err := opt(&config); err != nil {
			return nil, err
		}
	}

	generalSnapshotter, _ := overlayfork.NewSnapshotter(root)
	overlaySnapshotter := generalSnapshotter.(*overlayfork.Snapshotter)
	return &nixSnapshotter{
		Snapshotter: overlayfork.Snapshotter(*overlaySnapshotter),
		nixStoreDir: nixStoreDir,
		fuse:        config.fuse,
	}, nil
}

func (o *nixSnapshotter) Prepare(ctx context.Context, key, parent string, opts ...snapshots.Opt) ([]mount.Mount, error) {
	var base snapshots.Info
	for _, opt := range opts {
		if err := opt(&base); err != nil {
			return nil, err
		}
	}

	mounts, err := o.createSnapshot(ctx, snapshots.KindActive, key, parent, opts)
	if err != nil {
		return nil, err
	}

	// Annotations with prefix `containerd.io/snapshot/` will be passed down by
	// the unpacker during CRI pull time. If this is a nix layer, then we need to
	// prepare gc roots to ensure nix doesn't GC the underlying paths while this
	// snapshot is alive.
	//
	// We also don't return any nix bind mounts because the unpacker just needs
	// to retrieve and unpack the layer tarball containing the nix store
	// mountpoints and copyToRoot symlinks. Returning nix bind mounts will error
	// due to the paths being read only.
	if _, ok := base.Labels[nix2container.NixLayerAnnotation]; ok {
		err = o.prepareNixGCRoots(ctx, key, base.Labels)
		return mounts, err
	}

	return o.withNixBindMounts(ctx, key, mounts)
}

func (o *nixSnapshotter) prepareNixGCRoots(ctx context.Context, key string, labels map[string]string) error {
	ctx, t, err := o.GetMs().TransactionContext(ctx, false)
	if err != nil {
		return err
	}
	defer t.Rollback()
	id, _, _, err := storage.GetInfo(ctx, key)
	if err != nil {
		return err
	}

	// Allow users to specify which nix to use. Perhaps this should be coming from
	// a label.
	nixTool := os.Getenv("NIX_TOOL")
	if nixTool == "" {
		nixTool = defaultNixTool
	}

	gcRootsDir := filepath.Join(o.GetRoot(), "gcroots", id)
	log.G(ctx).Infof("Preparing nix gc roots at %s", gcRootsDir)
	for label, nixHash := range labels {
		if !strings.HasPrefix(label, nix2container.NixStorePrefixAnnotation) {
			continue
		}

		// nix build with a store path fetches a store path from the configured
		// substituters, if it doesn't already exist.
		nixPath := filepath.Join(o.nixStoreDir, nixHash)
		_, err = exec.Command(
			nixTool,
			"build",
			"--out-link",
			filepath.Join(gcRootsDir, nixHash),
			nixPath,
		).Output()
		if err != nil {
			return err
		}
	}

	return nil
}

func (o *nixSnapshotter) View(ctx context.Context, key, parent string, opts ...snapshots.Opt) ([]mount.Mount, error) {
	mounts, err := o.createSnapshot(ctx, snapshots.KindView, key, parent, opts)
	if err != nil {
		return nil, err
	}
	return o.withNixBindMounts(ctx, key, mounts)
}

// Mounts returns the mounts for the transaction identified by key. Can be
// called on an read-write or readonly transaction.
//
// This can be used to recover mounts after calling View or Prepare.
func (o *nixSnapshotter) Mounts(ctx context.Context, key string) ([]mount.Mount, error) {
	ctx, t, err := o.GetMs().TransactionContext(ctx, false)
	if err != nil {
		return nil, err
	}
	s, err := storage.GetSnapshot(ctx, key)
	t.Rollback()
	if err != nil {
		return nil, fmt.Errorf("failed to get active mount: %w", err)
	}

	return o.withNixBindMounts(ctx, key, o.mounts(s))
}

// Remove abandons the snapshot identified by key. The snapshot will
// immediately become unavailable and unrecoverable. Disk space will
// be freed up on the next call to `Cleanup`.
func (o *nixSnapshotter) Remove(ctx context.Context, key string) (err error) {
	ctx, t, err := o.GetMs().TransactionContext(ctx, true)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			if rerr := t.Rollback(); rerr != nil {
				log.G(ctx).WithError(rerr).Warn("failed to rollback transaction")
			}
		}
	}()

	_, _, err = storage.Remove(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to remove: %w", err)
	}

	if !o.GetAsyncRemove() {
		var removals []string
		removals, err = o.getCleanupDirectories(ctx)
		if err != nil {
			return fmt.Errorf("unable to get directories for removal: %w", err)
		}

		// Remove directories after the transaction is closed, failures must not
		// return error since the transaction is committed with the removal
		// key no longer available.
		defer func() {
			if err == nil {
				for _, dir := range removals {
					if err := os.RemoveAll(dir); err != nil {
						log.G(ctx).WithError(err).WithField("path", dir).Warn("failed to remove directory")
					}
				}
			}
		}()

	}

	return t.Commit()
}

func (o *nixSnapshotter) getCleanupDirectories(ctx context.Context) ([]string, error) {
	ids, err := storage.IDMap(ctx)
	if err != nil {
		return nil, err
	}

	snapshotDir := filepath.Join(o.GetRoot(), "snapshots")
	fd, err := os.Open(snapshotDir)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	dirs, err := fd.Readdirnames(0)
	if err != nil {
		return nil, err
	}

	cleanup := []string{}
	gcRootsDir := filepath.Join(o.GetRoot(), "gcroots")
	for _, d := range dirs {
		if _, ok := ids[d]; ok {
			continue
		}
		// Cleanup the snapshot and its corresponding nix gc roots.
		cleanup = append(cleanup, filepath.Join(snapshotDir, d))
		cleanup = append(cleanup, filepath.Join(gcRootsDir, d))
	}

	return cleanup, nil
}

func (o *nixSnapshotter) createSnapshot(ctx context.Context, kind snapshots.Kind, key, parent string, opts []snapshots.Opt) (_ []mount.Mount, err error) {
	ctx, t, err := o.GetMs().TransactionContext(ctx, true)
	if err != nil {
		return nil, err
	}

	var td, path string
	defer func() {
		if err != nil {
			if td != "" {
				if err1 := os.RemoveAll(td); err1 != nil {
					log.G(ctx).WithError(err1).Warn("failed to cleanup temp snapshot directory")
				}
			}
			if path != "" {
				if err1 := os.RemoveAll(path); err1 != nil {
					log.G(ctx).WithError(err1).WithField("path", path).Error("failed to reclaim snapshot directory, directory may need removal")
					err = fmt.Errorf("failed to remove path: %v: %w", err1, err)
				}
			}
		}
	}()

	snapshotDir := filepath.Join(o.GetRoot(), "snapshots")
	td, err = o.prepareDirectory(ctx, snapshotDir, kind)
	if err != nil {
		if rerr := t.Rollback(); rerr != nil {
			log.G(ctx).WithError(rerr).Warn("failed to rollback transaction")
		}
		return nil, fmt.Errorf("failed to create prepare snapshot dir: %w", err)
	}
	rollback := true
	defer func() {
		if rollback {
			if rerr := t.Rollback(); rerr != nil {
				log.G(ctx).WithError(rerr).Warn("failed to rollback transaction")
			}
		}
	}()

	s, err := storage.CreateSnapshot(ctx, kind, key, parent, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create snapshot: %w", err)
	}

	if len(s.ParentIDs) > 0 {
		st, err := os.Stat(o.upperPath(s.ParentIDs[0]))
		if err != nil {
			return nil, fmt.Errorf("failed to stat parent: %w", err)
		}

		stat := st.Sys().(*syscall.Stat_t)

		if err := os.Lchown(filepath.Join(td, "fs"), int(stat.Uid), int(stat.Gid)); err != nil {
			return nil, fmt.Errorf("failed to chown: %w", err)
		}
	}

	path = filepath.Join(snapshotDir, s.ID)
	if err = os.Rename(td, path); err != nil {
		return nil, fmt.Errorf("failed to rename: %w", err)
	}
	td = ""

	rollback = false
	if err = t.Commit(); err != nil {
		return nil, fmt.Errorf("commit failed: %w", err)
	}

	return o.mounts(s), nil
}

func (o *nixSnapshotter) prepareDirectory(ctx context.Context, snapshotDir string, kind snapshots.Kind) (string, error) {
	td, err := os.MkdirTemp(snapshotDir, "new-")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}

	if err := os.Mkdir(filepath.Join(td, "fs"), 0755); err != nil {
		return td, err
	}

	if kind == snapshots.KindActive {
		if err := os.Mkdir(filepath.Join(td, "work"), 0711); err != nil {
			return td, err
		}
	}

	return td, nil
}

func (o *nixSnapshotter) mounts(s storage.Snapshot) []mount.Mount {
	if len(s.ParentIDs) == 0 {
		// if we only have one layer/no parents then just return a bind mount as overlay
		// will not work
		roFlag := "rw"
		if s.Kind == snapshots.KindView {
			roFlag = "ro"
		}

		return []mount.Mount{
			{
				Source: o.upperPath(s.ID),
				Type:   "bind",
				Options: []string{
					roFlag,
					"rbind",
				},
			},
		}
	}
	var options []string

	// set index=off when mount overlayfs
	if o.GetIndexOff() {
		options = append(options, "index=off")
	}

	if o.GetUserXattr() {
		options = append(options, "userxattr")
	}

	if s.Kind == snapshots.KindActive {
		options = append(options,
			fmt.Sprintf("workdir=%s", o.workPath(s.ID)),
			fmt.Sprintf("upperdir=%s", o.upperPath(s.ID)),
		)
	} else if len(s.ParentIDs) == 1 {
		return []mount.Mount{
			{
				Source: o.upperPath(s.ParentIDs[0]),
				Type:   "bind",
				Options: []string{
					"ro",
					"rbind",
				},
			},
		}
	}

	parentPaths := make([]string, len(s.ParentIDs))
	for i := range s.ParentIDs {
		parentPaths[i] = o.upperPath(s.ParentIDs[i])
	}

	options = append(options, fmt.Sprintf("lowerdir=%s", strings.Join(parentPaths, ":")))
	return []mount.Mount{
		{
			Type:    o.overlayMountType(),
			Source:  "overlay",
			Options: options,
		},
	}
}

func (o *nixSnapshotter) upperPath(id string) string {
	return filepath.Join(o.GetRoot(), "snapshots", id, "fs")
}

func (o *nixSnapshotter) workPath(id string) string {
	return filepath.Join(o.GetRoot(), "snapshots", id, "work")
}

func (o *nixSnapshotter) overlayMountType() string {
	if o.fuse {
		return "fuse3.fuse-overlayfs"
	}
	return "overlay"
}

func (o *nixSnapshotter) withNixBindMounts(ctx context.Context, key string, mounts []mount.Mount) ([]mount.Mount, error) {
	ctx, t, err := o.GetMs().TransactionContext(ctx, false)
	if err != nil {
		return nil, err
	}
	defer t.Rollback()

	// Add a read only bind mount for every nix path required for the current
	// snapshot and all its parents.
	pathsSeen := make(map[string]struct{})
	for currentKey := key; currentKey != ""; {
		_, info, _, err := storage.GetInfo(ctx, currentKey)
		if err != nil {
			return nil, err
		}

		for label, nixHash := range info.Labels {
			if !strings.HasPrefix(label, nix2container.NixStorePrefixAnnotation) {
				continue
			}

			// Avoid duplicate mounts.
			_, ok := pathsSeen[nixHash]
			if ok {
				continue
			}
			pathsSeen[nixHash] = struct{}{}

			storePath := filepath.Join(o.nixStoreDir, nixHash)
			mounts = append(mounts, mount.Mount{
				Source: storePath,
				Type:   "bind",
				Target: storePath,
				Options: []string{
					"ro",
					"rbind",
				},
			})
		}

		currentKey = info.Parent
	}
	return mounts, nil
}
