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

	"github.com/containerd/containerd/log"
	"github.com/containerd/containerd/mount"
	"github.com/containerd/containerd/snapshots"
	"github.com/containerd/containerd/snapshots/storage"
	"github.com/pdtpartners/nix-snapshotter/pkg/nix2container"
	"github.com/pdtpartners/nix-snapshotter/pkg/overlayfork"
)

const (
	defaultNixTool = "nix"
)

// NixSnapshotterConfig is used to configure the nix snapshotter instance
type NixSnapshotterConfig struct {
	fuse bool
}

// Opt is an option to configure the nix snapshotter
type Opt func(config *NixSnapshotterConfig) error

// WithFuseOverlayfs changes the overlay mount type used to fuse-overlayfs, an
// FUSE implementation for overlayfs.
//
// See: https://github.com/containers/fuse-overlayfs
func WithFuseOverlayfs(config *NixSnapshotterConfig) error {
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
	var config NixSnapshotterConfig
	for _, opt := range opts {
		fmt.Printf("type: %+v\n", opt)
		if err := opt(&config); err != nil {
			return nil, err
		}
	}

	//TODO Add support for ops to NewShapshotter

	generalSnapshotter, err := overlayfork.NewSnapshotter(root)
	if err != nil {
		return nil, err
	}

	overlaySnapshotter, ok := generalSnapshotter.(*overlayfork.Snapshotter)
	if ok {
		return &nixSnapshotter{
			Snapshotter: *overlaySnapshotter,
			nixStoreDir: nixStoreDir,
			fuse:        config.fuse,
		}, nil
	} else {
		return nil, fmt.Errorf("Failed to cast snapshotter")
	}

}

func (o *nixSnapshotter) Prepare(ctx context.Context, key, parent string, opts ...snapshots.Opt) ([]mount.Mount, error) {
	var base snapshots.Info
	for _, opt := range opts {
		if err := opt(&base); err != nil {
			return nil, err
		}
	}

	//TODO pass config into overlayfork snapshotter

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

	//TODO CONVERT OPTS AND PASS IN
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
	err = o.Snapshotter.Remove(ctx, key)
	if !o.GetAsyncRemove() {
		var removals []string
		removals, err = o.getCleanupNixDirectories(ctx)
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

	return
}

func (o *nixSnapshotter) getCleanupNixDirectories(ctx context.Context) ([]string, error) {
	ids, err := storage.IDMap(ctx)
	if err != nil {
		return nil, err
	}

	gcRootsDir := filepath.Join(o.GetRoot(), "gcroots")
	fd, err := os.Open(gcRootsDir)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	dirs, err := fd.Readdirnames(0)
	if err != nil {
		return nil, err
	}

	cleanup := []string{}
	for _, d := range dirs {
		if _, ok := ids[d]; ok {
			continue
		}
		// Cleanup the snapshot and its corresponding nix gc roots.
		cleanup = append(cleanup, filepath.Join(gcRootsDir, d))
	}

	return cleanup, nil
}

func (o *nixSnapshotter) upperPath(id string) string {
	return filepath.Join(o.GetRoot(), "snapshots", id, "fs")
}

func (o *nixSnapshotter) workPath(id string) string {
	return filepath.Join(o.GetRoot(), "snapshots", id, "work")
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
