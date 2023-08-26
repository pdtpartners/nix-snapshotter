//nolint
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
	"sort"
	"strings"

	"github.com/containerd/containerd/log"
	"github.com/containerd/containerd/mount"
	"github.com/containerd/containerd/snapshots"
	"github.com/containerd/containerd/snapshots/overlay"
	"github.com/containerd/containerd/snapshots/storage"
	"github.com/pdtpartners/nix-snapshotter/pkg/nix2container"
)

// NixBuilder is a `nix build --out-link` implementation.
type NixBuilder func(ctx context.Context, gcRootPath, nixStorePath string) error

// NixSnapshotterConfig is used to configure the nix snapshotter instance
type NixSnapshotterConfig struct {
	fuse        bool
	nixBuilder  NixBuilder
	nixStoreDir string
}

// NixOpt is an option to configure the nix snapshotter
type NixOpt func(config *NixSnapshotterConfig) error

// WithNixBuilder is an option to specify how to subsitute a nix store path
// and create a GC root out-link.
func WithNixBuilder(nixBuilder NixBuilder) NixOpt {
	return func(config *NixSnapshotterConfig) error {
		config.nixBuilder = nixBuilder
		return nil
	}
}

// WithNixStoreDir is an option to specify the directory of the nix store
// The default is "/nix/store"
func WithNixStoreDir(nixStoreDir string) NixOpt {
	return func(config *NixSnapshotterConfig) error {
		config.nixStoreDir = nixStoreDir
		return nil
	}
}

// WithFuseOverlayfs changes the overlay mount type used to fuse-overlayfs, an
// FUSE implementation for overlayfs.
//
// See: https://github.com/containers/fuse-overlayfs
func WithFuseOverlayfs(config *NixSnapshotterConfig) error {
	config.fuse = true
	return nil
}

type nixSnapshotter struct {
	snapshots.Snapshotter
	ms          *storage.MetaStore
	asyncRemove bool
	root        string
	fuse        bool
	nixStoreDir string
	nixBuilder  NixBuilder
}

func defaultNixBuilder(ctx context.Context, gcRootPath, nixStorePath string) error {
	return exec.Command(
		"nix",
		"build",
		"--out-link",
		gcRootPath,
		nixStorePath,
	).Run()
}

// NewSnapshotter returns a Snapshotter which uses overlayfs. The overlayfs
// diffs are stored under the provided root. A metadata file is stored under
// the root.
func NewSnapshotter(root string, opts ...interface{}) (snapshots.Snapshotter, error) {
	config := NixSnapshotterConfig{
		nixBuilder:  defaultNixBuilder,
		nixStoreDir: "/nix/store",
	}
	overlayOpts := []overlay.Opt{}
	for _, opt := range opts {
		switch safeOpt := opt.(type) {
		// Checking the NixOpt here does not work for some cases
		case func(config *NixSnapshotterConfig) error:
			if err := safeOpt(&config); err != nil {
				return nil, err
			}
		// But it does for others (when a func explicitly returns NixOpt like
		// WithNixBuilder)
		case NixOpt:
			if err := safeOpt(&config); err != nil {
				return nil, err
			}

		// Checking the overlay.Opt here does not work but expanding does.
		// However if func returns an opt then only overlay.opt works
		case func(config *overlay.SnapshotterConfig) error:
			overlayOpts = append(overlayOpts, safeOpt)
		case overlay.Opt:
			overlayOpts = append(overlayOpts, safeOpt)

		default:
			return nil, fmt.Errorf("Unexpected opt type: %T", safeOpt)
		}
	}

	ms, err := storage.NewMetaStore(filepath.Join(root, "metadata.db"))
	if err != nil {
		return nil, err
	}
	overlayOpts = append(overlayOpts, overlay.WithMetaStore(ms))
	overlaySnapshotter, err := overlay.NewSnapshotter(root, overlayOpts...)
	if err != nil {
		return nil, err
	}

	return &nixSnapshotter{
		Snapshotter: overlaySnapshotter,
		ms:          ms,
		asyncRemove: false,
		root:        root,
		nixStoreDir: config.nixStoreDir,
		fuse:        config.fuse,
		nixBuilder:  config.nixBuilder,
	}, nil

}

func (o *nixSnapshotter) Prepare(ctx context.Context, key, parent string, opts ...snapshots.Opt) ([]mount.Mount, error) {
	var base snapshots.Info
	for _, opt := range opts {
		if err := opt(&base); err != nil {
			return nil, err
		}
	}

	mounts, err := o.Snapshotter.Prepare(ctx, key, parent, opts...)
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

func (o *nixSnapshotter) prepareNixGCRoots(ctx context.Context, key string, labels map[string]string) (err error) {
	ctx, t, err := o.ms.TransactionContext(ctx, false)
	if err != nil {
		return err
	}
	defer func() {
		err = t.Rollback()
	}()
	id, _, _, err := storage.GetInfo(ctx, key)
	if err != nil {
		return err
	}

	// Make the order of nix substitution deterministic
	sortedLabels := []string{}
	for label := range labels {
		sortedLabels = append(sortedLabels, label)
	}
	sort.Strings(sortedLabels)

	gcRootsDir := filepath.Join(o.root, "gcroots", id)
	log.G(ctx).Infof("Preparing nix gc roots at %s", gcRootsDir)
	for _, labelKey := range sortedLabels {
		if !strings.HasPrefix(labelKey, nix2container.NixStorePrefixAnnotation) {
			continue
		}

		// nix build with a store path fetches a store path from the configured
		// substituters, if it doesn't already exist.
		nixHash := labels[labelKey]
		gcRootPath := filepath.Join(gcRootsDir, nixHash)
		nixStorePath := filepath.Join(o.nixStoreDir, nixHash)
		err = o.nixBuilder(ctx, gcRootPath, nixStorePath)
		if err != nil {
			return err
		}
	}

	return nil
}

func (o *nixSnapshotter) View(ctx context.Context, key, parent string, opts ...snapshots.Opt) ([]mount.Mount, error) {

	mounts, err := o.Snapshotter.View(ctx, key, parent, opts...)
	if err != nil {
		return nil, err
	}
	return o.withNixBindMounts(ctx, key, o.convertToOverlayMountType(mounts))
}

// Mounts returns the mounts for the transaction identified by key. Can be
// called on an read-write or readonly transaction.
//
// This can be used to recover mounts after calling View or Prepare.
func (o *nixSnapshotter) Mounts(ctx context.Context, key string) ([]mount.Mount, error) {
	mounts, err := o.Snapshotter.Mounts(ctx, key)
	if err != nil {
		return nil, err
	}
	return o.withNixBindMounts(ctx, key, o.convertToOverlayMountType(mounts))
}

// Remove abandons the snapshot identified by key. The snapshot will
// immediately become unavailable and unrecoverable. Disk space will
// be freed up on the next call to `Cleanup`.
func (o *nixSnapshotter) Remove(ctx context.Context, key string) (err error) {
	ctx, t, err := o.ms.TransactionContext(ctx, true)
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

	if !o.asyncRemove {
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

// Cleanup cleans up disk resources from removed or abandoned snapshots
func (o *nixSnapshotter) Cleanup(ctx context.Context) error {
	cleanup, err := o.cleanupDirectories(ctx)
	if err != nil {
		return err
	}

	for _, dir := range cleanup {
		if err := os.RemoveAll(dir); err != nil {
			log.G(ctx).WithError(err).WithField("path", dir).Warn("failed to remove directory")
		}
	}

	return nil
}

func (o *nixSnapshotter) cleanupDirectories(ctx context.Context) ([]string, error) {
	// Get a write transaction to ensure no other write transaction can be entered
	// while the cleanup is scanning.
	ctx, t, err := o.ms.TransactionContext(ctx, true)
	if err != nil {
		return nil, err
	}

	defer func() {
		err = t.Rollback()
	}()

	return o.getCleanupDirectories(ctx)
}

func (o *nixSnapshotter) getCleanupDirectories(ctx context.Context) ([]string, error) {
	ids, err := storage.IDMap(ctx)
	if err != nil {
		return nil, err
	}

	snapshotDir := filepath.Join(o.root, "snapshots")
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
	gcRootsDir := filepath.Join(o.root, "gcroots")
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

func (o *nixSnapshotter) convertToOverlayMountType(mounts []mount.Mount) []mount.Mount {
	if o.fuse {
		for _, mount := range mounts {
			mount.Type = "fuse3.fuse-overlayfs"
		}
	}
	return mounts
}

func (o *nixSnapshotter) withNixBindMounts(ctx context.Context, key string, mounts []mount.Mount) ([]mount.Mount, error) {
	ctx, t, err := o.ms.TransactionContext(ctx, false)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = t.Rollback()
	}()

	// Add a read only bind mount for every nix path required for the current
	// snapshot and all its parents.
	pathsSeen := make(map[string]struct{})
	for currentKey := key; currentKey != ""; {
		_, info, _, err := storage.GetInfo(ctx, currentKey)
		if err != nil {
			return nil, err
		}

		// Make the order of the bind mounts deterministic
		sortedLabels := []string{}
		for label := range info.Labels {
			sortedLabels = append(sortedLabels, label)
		}
		sort.Strings(sortedLabels)

		for _, labelKey := range sortedLabels {
			if !strings.HasPrefix(labelKey, nix2container.NixStorePrefixAnnotation) {
				continue
			}

			// Avoid duplicate mounts.
			nixHash := info.Labels[labelKey]
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
