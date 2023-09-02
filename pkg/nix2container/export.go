package nix2container

import (
	"context"
	"io"

	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/images/archive"
	"github.com/pdtpartners/nix-snapshotter/types"
)

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
