package nix2container

import (
	"context"
	"io"

	"github.com/containerd/containerd/v2/core/content"
	"github.com/containerd/containerd/v2/core/images/archive"
	"github.com/pdtpartners/nix-snapshotter/types"
)

var (
	// ImageRefPrefix is part of the canonical image reference for images built
	// for nix-snapshotter in the format "nix:0/nix/store/*.tar".
	//
	// Leading slash is not allowed for image references, so we needed a distinct
	// prefix for nix-snapshotter to distinguish regular references from nix
	// references. If nix-snapshotter is configured as the CRI image service,
	// it will be able to resolve the image manifest with nix rather than a
	// Docker Registry.
	ImageRefPrefix = "nix:0"
)

// Export writes an OCI archive to the writer using the provided nix image
// spec.
func Export(ctx context.Context, store content.Store, image *types.Image, ref string, w io.Writer) error {
	desc, err := Generate(ctx, image, store)
	if err != nil {
		return err
	}

	exportOpts := []archive.ExportOpt{
		archive.WithManifest(desc, ref),
	}

	return archive.Export(ctx, store, w, exportOpts...)
}
