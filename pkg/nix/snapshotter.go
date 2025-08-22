package nix

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/containerd/containerd/v2/core/mount"
	"github.com/containerd/containerd/v2/core/snapshots"
	"github.com/containerd/containerd/v2/core/snapshots/storage"
	"github.com/containerd/containerd/v2/plugins/snapshots/overlay"
	"github.com/containerd/log"
	"github.com/pdtpartners/nix-snapshotter/pkg/nix2container"
)

// SnapshotterConfig is used to configure the nix snapshotter instance.
type SnapshotterConfig struct {
	Config
	fuse        bool
	overlayOpts []overlay.Opt
}

// SnapshotterOpt is an option for NewSnapshotter.
type SnapshotterOpt interface {
	SetSnapshotterOpt(sc *SnapshotterConfig)
}

type snapshotterOptFn func(*SnapshotterConfig)

func (fn snapshotterOptFn) SetSnapshotterOpt(sc *SnapshotterConfig) {
	fn(sc)
}

// WithFuseOverlayfs changes the overlay mount type used to fuse-overlayfs, an
// FUSE implementation for overlayfs.
//
// See: https://github.com/containers/fuse-overlayfs
func WithFuseOverlayfs() SnapshotterOpt {
	return snapshotterOptFn(func(sc *SnapshotterConfig) {
		sc.fuse = true
	})
}

// WithOverlayOpts provides overlay options to the embedded overlay snapshotter.
func WithOverlayOpts(opts ...overlay.Opt) SnapshotterOpt {
	return snapshotterOptFn(func(sc *SnapshotterConfig) {
		sc.overlayOpts = append(sc.overlayOpts, opts...)
	})
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
func NewSnapshotter(root string, opts ...SnapshotterOpt) (snapshots.Snapshotter, error) {
	cfg := SnapshotterConfig{
		Config: Config{
			nixBuilder: defaultNixBuilder,
		},
	}
	for _, opt := range opts {
		opt.SetSnapshotterOpt(&cfg)
	}

	ms, err := storage.NewMetaStore(filepath.Join(root, "metadata.db"))
	if err != nil {
		return nil, err
	}
	cfg.overlayOpts = append(cfg.overlayOpts, overlay.WithMetaStore(ms))

	overlaySnapshotter, err := overlay.NewSnapshotter(root, cfg.overlayOpts...)
	if err != nil {
		return nil, err
	}

	return &nixSnapshotter{
		Snapshotter: overlaySnapshotter,
		ms:          ms,
		asyncRemove: false,
		root:        root,
		fuse:        cfg.fuse,
		nixBuilder:  cfg.nixBuilder,
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
	if closurePath, ok := base.Labels[nix2container.NixClosureAnnotation]; ok {
		err = o.prepareNixGCRoot(ctx, key, closurePath)
		return mounts, err
	}

	// If image doesn't NixClosureAnnotation, we maintain backwards-compat with
	// older nix-snapshotter images that have an annotation per store path in its
	// closure.
	if _, ok := base.Labels[nix2container.NixLayerAnnotation]; ok {
		err = o.prepareNixGCRootsLegacy(ctx, key, base.Labels)
		return mounts, err
	}

	return o.withNixBindMounts(ctx, key, mounts)
}

func (o *nixSnapshotter) prepareNixGCRoot(ctx context.Context, key, closurePath string) (err error) {
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

	gcRootsDir := filepath.Join(o.root, "gcroots", id)
	outLink := filepath.Join(gcRootsDir, filepath.Base(closurePath))
	log.G(ctx).Infof("[nix-snapshotter] Preparing nix gc roots for %s at %s", key, closurePath)
	return o.nixBuilder(ctx, outLink, closurePath)
}

func (o *nixSnapshotter) prepareNixGCRootsLegacy(ctx context.Context, key string, labels map[string]string) (err error) {
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
	log.G(ctx).Infof("[nix-snapshotter] Preparing %d nix gc roots at %s", len(sortedLabels), gcRootsDir)
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

		var nixStorePaths []string
		closurePath, ok := info.Labels[nix2container.NixClosureAnnotation]
		if ok {
			nixStorePaths, err = nix2container.ReadClosure(filepath.Join(closurePath, "store-paths"))
			if err != nil {
				return nil, err
			}
		} else if _, ok := info.Labels[nix2container.NixLayerAnnotation]; ok {
			// Maintain backwards-compat support for NixLayerAnnotation.

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
				nixStorePath := info.Labels[labelKey]
				nixStorePaths = append(nixStorePaths, nixStorePath)
			}
		}

		for _, nixStorePath := range nixStorePaths {
			// Avoid duplicate mounts.
			_, ok := pathsSeen[nixStorePath]
			if ok {
				continue
			}
			pathsSeen[nixStorePath] = struct{}{}

			log.G(ctx).Debugf("[nix-snapshotter] Bind mounting nix store path %s", nixStorePath)
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
