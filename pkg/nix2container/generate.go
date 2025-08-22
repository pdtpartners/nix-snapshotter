package nix2container

import (
	"archive/tar"
	"bytes"
	"context"
	_ "crypto/sha256" // required by go-digest
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/containerd/containerd/v2/core/content"
	"github.com/containerd/containerd/v2/core/remotes"
	"github.com/containerd/containerd/v2/pkg/archive"
	"github.com/containerd/containerd/v2/pkg/archive/compression"
	cfs "github.com/containerd/continuity/fs"
	"github.com/containerd/platforms"
	digest "github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pdtpartners/nix-snapshotter/types"
)

const (
	// NixLayerAnnotation is a remote snapshot OCI annotation to indicate that
	// it will also contain annotations with NixStorePrefixAnnotation.
	NixLayerAnnotation = "containerd.io/snapshot/nix-layer"

	// NixStorePrefixAnnotation is a prefix for remote snapshot OCI annotations
	// for each nix store path that the layer will need.
	NixStorePrefixAnnotation = "containerd.io/snapshot/nix-store-path."
)

// TempDir returns the location of a temporary dir or XDG_RUNTIME_DIR if it is
// defined.
func TempDir() string {
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		return xdg
	}
	return os.TempDir()
}

// Generate adds a nix-snapshotter container image to store and returns its
// descriptor.
func Generate(ctx context.Context, image *types.Image, store content.Store) (desc ocispec.Descriptor, err error) {
	// Initialize the manifest and manifest config from its base image.
	var (
		mfst ocispec.Manifest
		cfg  ocispec.Image
	)
	mfst, cfg, err = initializeManifest(ctx, image, store)
	if err != nil {
		return
	}

	// Generate and add layer to store.
	buf := new(bytes.Buffer)
	diffID, err := writeNixClosureLayer(ctx, buf, image.NixStorePaths, image.CopyToRoots)
	if err != nil {
		return
	}

	cfg.RootFS.DiffIDs = append(cfg.RootFS.DiffIDs, diffID)

	layerDesc, err := writeBlob(ctx, store, ocispec.MediaTypeImageLayerGzip, buf.Bytes())
	if err != nil {
		return
	}

	layerDesc.Annotations = map[string]string{
		NixLayerAnnotation: "true",
	}
	for i, nixStorePath := range image.NixStorePaths {
		key := NixStorePrefixAnnotation + strconv.Itoa(i)
		layerDesc.Annotations[key] = nixStorePath
	}
	mfst.Layers = append(mfst.Layers, layerDesc)

	// Add manifest config to store.
	configDesc, err := writeBlob(ctx, store, ocispec.MediaTypeImageConfig, &cfg)
	if err != nil {
		return
	}
	mfst.Config = configDesc

	// Add manifest to store.
	return writeBlob(ctx, store, mfst.MediaType, &mfst)
}

func writeBlob(ctx context.Context, store content.Store, mediaType string, v interface{}) (ocispec.Descriptor, error) {
	blob, ok := v.([]byte)
	if !ok {
		var err error
		blob, err = json.MarshalIndent(v, "", "  ")
		if err != nil {
			return ocispec.Descriptor{}, err
		}
	}

	desc := ocispec.Descriptor{
		MediaType: mediaType,
		Digest:    digest.FromBytes(blob),
		Size:      int64(len(blob)),
	}

	ref := remotes.MakeRefKey(ctx, desc)
	return desc, content.WriteBlob(ctx, store, ref, bytes.NewReader(blob), desc)
}

// initializeManifest initializes a manifest and manifest config based on the
// image's base image.
func initializeManifest(ctx context.Context, image *types.Image, store content.Store) (ocispec.Manifest, ocispec.Image, error) {
	mfst := ocispec.Manifest{
		MediaType: ocispec.MediaTypeImageManifest,
		Versioned: specs.Versioned{
			SchemaVersion: 2,
		},
		Annotations: make(map[string]string),
	}

	cfg := ocispec.Image{
		Config: image.Config,
		Platform: platforms.Platform{
			Architecture: image.Architecture,
			OS:           image.OS,
		},
		RootFS: ocispec.RootFS{
			Type: "layers",
		},
	}

	// If the base image is non-empty, add the base image's layers, annotations
	// and diff IDs to the new image's manifest and manifest config.
	if image.BaseImage != "" {
		baseMfst, baseCfg, err := parseOCITarball(ctx, store, image.BaseImage)
		if err != nil {
			return mfst, cfg, err
		}

		// Inherit layers, annotations and diff IDs from base image.
		mfst.Layers = append(mfst.Layers, baseMfst.Layers...)
		for k, v := range mfst.Annotations {
			mfst.Annotations[k] = v
		}
		cfg.RootFS.DiffIDs = append(cfg.RootFS.DiffIDs, baseCfg.RootFS.DiffIDs...)
	}

	return mfst, cfg, nil
}

// parseOCITarball extracts a ocispec.Manifest and ocispec.Image from an OCI
// archive tarball at the given tarballPath.
func parseOCITarball(ctx context.Context, store content.Store, tarballPath string) (mfst ocispec.Manifest, cfg ocispec.Image, err error) {
	// Untar OCI tarball into temp directory.
	root, err := os.MkdirTemp(TempDir(), "nix2container-oci")
	if err != nil {
		return mfst, cfg, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(root)

	f, err := os.Open(tarballPath)
	if err != nil {
		return
	}
	defer f.Close()

	// Don't preserve the owner specified in the tar archive because it will try
	// to lchown(2) the archive but the archive is usually from a read only nix
	// store.
	_, err = archive.Apply(ctx, root, f, archive.WithNoSameOwner())
	if err != nil {
		return
	}

	// Unmarshal manifest.
	manifestPath := filepath.Join(root, "manifest.json")
	dt, err := os.ReadFile(manifestPath)
	if err != nil {
		return
	}

	var ociMfsts []types.OCIManifest
	err = json.Unmarshal(dt, &ociMfsts)
	if err != nil {
		return
	}
	if len(ociMfsts) != 1 {
		return mfst, cfg, fmt.Errorf("expected %d number of manifests, got %d", 1, len(ociMfsts))
	}
	ociMfst := ociMfsts[0]

	// Unmarshal manifest config.
	configPath := filepath.Join(root, ociMfst.Config)
	dt, err = os.ReadFile(configPath)
	if err != nil {
		return
	}

	err = json.Unmarshal(dt, &cfg)
	if err != nil {
		return
	}

	mfst.Config, err = writeBlob(ctx, store, ocispec.MediaTypeImageConfig, dt)
	if err != nil {
		return
	}

	// Load layers into store.
	for _, layer := range ociMfst.Layers {
		layerPath := filepath.Join(root, layer)
		dt, err = os.ReadFile(layerPath)
		if err != nil {
			return
		}

		var desc ocispec.Descriptor
		desc, err = writeBlob(ctx, store, ocispec.MediaTypeImageLayer, dt)
		if err != nil {
			return
		}
		mfst.Layers = append(mfst.Layers, desc)
	}
	return
}

// writeNixClosureLayer generates a tarball that creates a mountpoint for every
// store path. When the snapshotter prepares this layer, it will then mount the
// store paths into these mountpoints.
//
// Each store path in copyToRoots will also be walked to generate symlinks
// relative to root. Note that these symlinks will be broken until the
// containerd-shim finally mounts what nix-snapshotter has generated.
func writeNixClosureLayer(ctx context.Context, w io.Writer, nixStorePaths, copyToRoots []string) (digest.Digest, error) {
	root, err := os.MkdirTemp(TempDir(), "nix2container-closure")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(root)

	for _, nixStorePath := range nixStorePaths {
		fi, err := os.Stat(nixStorePath)
		if err != nil {
			return "", err
		}

		relStorePath := filepath.Join(root, nixStorePath)
		if fi.IsDir() {
			err = os.MkdirAll(relStorePath, 0o755)
		} else {
			err = os.MkdirAll(filepath.Dir(relStorePath), 0o755)
			if err != nil {
				return "", err
			}

			err = os.WriteFile(relStorePath, nil, 0o555)
		}
		if err != nil {
			return "", err
		}

	}

	// For each copyToRoot, walk the store path locally and create a symlink for
	// each file from the store path to a path relative to the rootfs' root.
	//
	// Example:
	// /nix/store/knn6wc1a89c47yb70qwv56rmxylia6wx-hello-2.12/bin/hello
	// =>
	// /bin/hello
	for _, copyToRoot := range copyToRoots {
		err = filepath.WalkDir(copyToRoot, func(path string, dentry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			rootPath := filepath.Join(root, strings.TrimPrefix(path, copyToRoot))
			if dentry.IsDir() {
				return os.MkdirAll(rootPath, 0o755)
			}

			if rootPath == root {
				return fmt.Errorf("copyToRoot expected to be directory but got %s", copyToRoot)
			}

			return os.Symlink(path, rootPath)
		})
		if err != nil {
			return "", err
		}
	}
	return tarDir(ctx, w, root, true)
}

func tarDir(ctx context.Context, w io.Writer, root string, gzip bool) (digest.Digest, error) {
	if gzip {
		compressed, err := compression.CompressStream(w, compression.Gzip)
		if err != nil {
			return "", err
		}
		defer compressed.Close()
		w = compressed
	}
	// Set upper bound for timestamps to be epoch 0 for reproducibility.

	opts := []archive.ChangeWriterOpt{
		archive.WithModTimeUpperBound(time.Time{}),
	}
	dgstr := digest.SHA256.Digester()
	cw := archive.NewChangeWriter(io.MultiWriter(w, dgstr.Hash()), root, opts...)
	err := cfs.Changes(ctx, "", root, rootUidGidChangeFunc(cw.HandleChange))
	// Finish archiving data before completing compression.
	cwErr := cw.Close()

	if err != nil {
		return "", fmt.Errorf("failed to create diff tar stream: %w", err)
	}
	if cwErr != nil {
		return "", cwErr
	}
	return dgstr.Digest(), nil
}

// rootFileInfo is a wrapped fs.FileInfo that forces Uid and Gid to be 0.
type rootFileInfo struct {
	fs.FileInfo
}

func (rfi *rootFileInfo) Sys() any {
	sys := rfi.FileInfo.Sys()
	switch s := sys.(type) {
	case *tar.Header:
		s.Uid = 0
		s.Gid = 0
		return sys
	case *syscall.Stat_t:
		s.Uid = 0
		s.Gid = 0
		return sys
	default:
		return sys
	}
}

// rootUidGidChangeFunc is a fs.ChangeFunc that wraps underlying os.FileInfo
// with one that forces Uid and Gid to be 0.
func rootUidGidChangeFunc(fn cfs.ChangeFunc) cfs.ChangeFunc {
	return func(k cfs.ChangeKind, p string, f os.FileInfo, err error) error {
		rf := &rootFileInfo{f}
		return fn(k, p, rf, err)
	}
}
