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
	"errors"
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

// NixBuilder is a function that is able to substitute a nix store path and
// optionally create an out-link. outLink may be empty in which case out-links
// are not needed.
//
// Typically this is implemented by `nix build --out-link ${outLink} ${nixStorePath}`,
// however it can also be done by `nix copy` and alternate implementations.
type NixBuilder func(ctx context.Context, outLink, nixStorePath string) error

// NixConfig is used to configure the nix snapshotter instance
type NixConfig struct {
	fuse        bool
	nixBuilder  NixBuilder
	overlayOpts []overlay.Opt
}

// NixOpt is an option to configure the nix snapshotter
type NixOpt func(config *NixConfig)

// WithNixBuilder is an option to override the default NixBuilder.
func WithNixBuilder(nixBuilder NixBuilder) NixOpt {
	return func(config *NixConfig) {
		config.nixBuilder = nixBuilder
	}
}

func defaultNixBuilder(ctx context.Context, outLink, nixStorePath string) error {
	args := []string{"build"}
	if outLink == "" {
		args = append(args, "--no-link")
	} else {
		args = append(args, "--out-link", outLink)
	}
	args = append(args, "nixStorePath")

	_, err := exec.Command("nix", args...).Output()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		log.G(ctx).
			WithField("nixStorePath", nixStorePath).
			Debugf("Failed to create gc root:\n%s", string(exitErr.Stderr))
	}
	return err
}

// WithFuseOverlayfs changes the overlay mount type used to fuse-overlayfs, an
// FUSE implementation for overlayfs.
//
// See: https://github.com/containers/fuse-overlayfs
func WithFuseOverlayfs() NixOpt {
	return func(config *NixConfig) {
		config.fuse = true
	}
}

func WithOverlayOpts(opts ...overlay.Opt) NixOpt {
	return func(config *NixConfig) {
		config.overlayOpts = append(config.overlayOpts, opts...)
	}
}

type nixSnapshotter struct {
	snapshots.Snapshotter
	ms          *storage.MetaStore
	asyncRemove bool
	root        string
	fuse        bool
	nixBuilder  NixBuilder
}

// NewSnapshotter returns a Snapshotter which uses overlayfs. The overlayfs
// diffs are stored under the provided root. A metadata file is stored under
// the root.
func NewSnapshotter(root string, opts ...NixOpt) (snapshots.Snapshotter, error) {
	config := NixConfig{
		nixBuilder: defaultNixBuilder,
	}
	for _, opt := range opts {
		opt(&config)
	}

	ms, err := storage.NewMetaStore(filepath.Join(root, "metadata.db"))
	if err != nil {
		return nil, err
	}
	config.overlayOpts = append(config.overlayOpts, overlay.WithMetaStore(ms))

	overlaySnapshotter, err := overlay.NewSnapshotter(root, config.overlayOpts...)
	if err != nil {
		return nil, err
	}

	return &nixSnapshotter{
		Snapshotter: overlaySnapshotter,
		ms:          ms,
		asyncRemove: false,
		root:        root,
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
		nixStorePath := labels[labelKey]
		outLink := filepath.Join(gcRootsDir, filepath.Base(nixStorePath))
		err = o.nixBuilder(ctx, outLink, nixStorePath)
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
			nixStorePath := info.Labels[labelKey]
			_, ok := pathsSeen[nixStorePath]
			if ok {
				continue
			}
			pathsSeen[nixStorePath] = struct{}{}

			mounts = append(mounts, mount.Mount{
				Type:   "bind",
				Source: nixStorePath,
				Target: nixStorePath,
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
