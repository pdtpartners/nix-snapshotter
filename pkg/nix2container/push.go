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

	"github.com/containerd/containerd/archive"
	"github.com/containerd/containerd/archive/compression"
	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/platforms"
	"github.com/containerd/containerd/remotes"
	cfs "github.com/containerd/continuity/fs"
	digest "github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pdtpartners/nix-snapshotter/types"
	"golang.org/x/sync/semaphore"
)

const (
	// NixLayerAnnotation is a remote snapshot OCI annotation to indicate that
	// it will also contain annotations with NixStorePrefixAnnotation.
	NixLayerAnnotation = "containerd.io/snapshot/nix-layer"

	// NixStorePrefixAnnotation is a prefix for remote snapshot OCI annotations
	// for each nix store path that the layer will need.
	NixStorePrefixAnnotation = "containerd.io/snapshot/nix/store."
)

type PushOpt func(*PushOpts)

type PushOpts struct {
	GetPusher      func(context.Context, string) (remotes.Pusher, error)
	GetPushContent func(context.Context, remotes.Pusher, ocispec.Descriptor, content.Provider, *semaphore.Weighted, platforms.MatchComparer, func(h images.Handler) images.Handler) error
}

// Push generates a nix-snapshotter image and pushes it to a remote.
func Push(ctx context.Context, image types.Image, ref string, opts ...PushOpt) error {
	var pOpts PushOpts
	pOpts.GetPusher = defaultPusher
	pOpts.GetPushContent = remotes.PushContent

	// Replaces pOpts with mock objects if testing
	for _, opt := range opts {
		opt(&pOpts)
	}

	provider := NewInmemoryProvider()
	desc, err := generateImage(ctx, image, provider)

	if err != nil {
		return err
	}

	pusher, err := pOpts.GetPusher(ctx, ref)
	if err != nil {
		return err
	}

	// Push image and its blobs to a registry.
	return pOpts.GetPushContent(ctx, pusher, desc, provider, nil, platforms.All, nil)
}

// generateImage adds a nix-snapshotter container image to provider and returns
// its descriptor.
func generateImage(ctx context.Context, image types.Image, provider *InmemoryProvider) (desc ocispec.Descriptor, err error) {
	// Initialize the manifest and manifest config from its base image.
	var (
		mfst ocispec.Manifest
		cfg  ocispec.Image
	)
	mfst, cfg, err = initializeManifest(ctx, image, provider)
	if err != nil {
		return
	}

	// Generate and add layer to provider.
	buf := new(bytes.Buffer)
	diffID, err := writeNixClosureLayer(ctx, buf, image.StorePaths, image.CopyToRoots)
	if err != nil {
		return
	}

	cfg.RootFS.DiffIDs = append(cfg.RootFS.DiffIDs, diffID)

	layerDesc, err := provider.AddBlob(ocispec.MediaTypeImageLayerGzip, buf.Bytes())
	if err != nil {
		return
	}

	layerDesc.Annotations = map[string]string{
		NixLayerAnnotation: "true",
	}
	for i, storePath := range image.StorePaths {
		key := NixStorePrefixAnnotation + strconv.Itoa(i)
		layerDesc.Annotations[key] = filepath.Base(storePath)
	}
	mfst.Layers = append(mfst.Layers, layerDesc)

	// Add manifest config to provider.
	configDesc, err := provider.AddBlob(ocispec.MediaTypeImageConfig, &cfg)
	if err != nil {
		return
	}

	mfst.Config = configDesc

	// Add manifest to provider.
	return provider.AddBlob(mfst.MediaType, &mfst)
}

// initializeManifest initializes a manifest and manifest config based on the
// image's base image.
func initializeManifest(ctx context.Context, image types.Image, provider *InmemoryProvider) (ocispec.Manifest, ocispec.Image, error) {
	mfst := ocispec.Manifest{
		MediaType: ocispec.MediaTypeImageManifest,
		Versioned: specs.Versioned{
			SchemaVersion: 2,
		},
		Annotations: make(map[string]string),
	}

	cfg := ocispec.Image{
		Config:       image.Config,
		Architecture: image.Architecture,
		OS:           image.OS,
		RootFS: ocispec.RootFS{
			Type: "layers",
		},
	}

	// If the base image is non-empty, add the base image's layers, annotations
	// and diff IDs to the new image's manifest and manifest config.
	if image.BaseImage != "" {
		imageType, err := DetectImageType(image.BaseImage)
		if err != nil {
			return mfst, cfg, err
		}

		var (
			baseMfst ocispec.Manifest
			baseCfg  ocispec.Image
		)
		switch imageType {
		case ImageTypeNix:
			baseMfst, baseCfg, err = parseNixImageJSON(ctx, provider, image.BaseImage)
			if err != nil {
				return mfst, cfg, err
			}
		case ImageTypeOCITarball:
			baseMfst, baseCfg, err = parseOCITarball(ctx, provider, image.BaseImage)
			if err != nil {
				return mfst, cfg, err
			}
		default:
			return mfst, cfg, fmt.Errorf("unknown image type at %s", image.BaseImage)
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
func parseOCITarball(ctx context.Context, provider *InmemoryProvider, tarballPath string) (mfst ocispec.Manifest, cfg ocispec.Image, err error) {
	// Untar OCI tarball into temp directory.
	root, err := os.MkdirTemp(getTempDir(), "nix2container-root")
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

	mfst.Config, err = provider.AddBlob(ocispec.MediaTypeImageConfig, dt)
	if err != nil {
		return
	}

	// Load layers into provider.
	for _, layer := range ociMfst.Layers {
		layerPath := filepath.Join(root, layer)
		dt, err = os.ReadFile(layerPath)
		if err != nil {
			return
		}

		var desc ocispec.Descriptor
		desc, err = provider.AddBlob(ocispec.MediaTypeImageLayer, dt)
		if err != nil {
			return
		}
		mfst.Layers = append(mfst.Layers, desc)
	}
	return
}

// parseNixImageJSON generates the base image from the given imagePath. Since
// image contents are only generated at push time, this is done on the fly for
// its base images recursively. Nix store path contents aren't actually
// packaged into docker layers, so this is cheap and avoids writing to the nix
// store.
func parseNixImageJSON(ctx context.Context, provider *InmemoryProvider, imagePath string) (mfst ocispec.Manifest, cfg ocispec.Image, err error) {
	dt, err := os.ReadFile(imagePath)
	if err != nil {
		return
	}

	var image types.Image
	err = json.Unmarshal(dt, &image)
	if err != nil {
		return
	}

	desc, err := generateImage(ctx, image, provider)
	if err != nil {
		return
	}
	fmt.Printf("Generated image from %s with %s\n", imagePath, desc.Digest)

	err = unmarshalFromProvider(ctx, provider, desc, &mfst)
	if err != nil {
		return
	}

	err = unmarshalFromProvider(ctx, provider, mfst.Config, &cfg)
	return
}

// writeNixClosureLayer generates a tarball that creates a mountpoint for every
// store path. When the snapshotter prepares this layer, it will then mount the
// store paths into these mountpoints.
//
// Each store path in copyToRoots will also be walked to generate symlinks
// relative to root. Note that these symlinks will be broken until the
// containerd-shim finally mounts what nix-snapshotter has generated.
func writeNixClosureLayer(ctx context.Context, w io.Writer, storePaths, copyToRoots []string) (digest.Digest, error) {
	root, err := os.MkdirTemp(getTempDir(), "nix2container-root")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(root)

	for _, storePath := range storePaths {
		fi, err := os.Stat(storePath)
		if err != nil {
			return "", err
		}

		relStorePath := filepath.Join(root, storePath)
		if !fi.IsDir() {
			relStorePath = filepath.Dir(relStorePath)
		}

		err = os.MkdirAll(relStorePath, 0o755)
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

func getTempDir() string {
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		return xdg
	}
	return os.TempDir()
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
