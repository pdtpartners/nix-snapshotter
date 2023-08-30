package nix2container

import (
	"context"
	"io"

	"github.com/containerd/containerd/images/archive"
	"github.com/pdtpartners/nix-snapshotter/types"
)

func Export(ctx context.Context, image types.Image, ref string, w io.Writer) error {
	provider := NewInmemoryProvider()
	desc, err := Generate(ctx, image, provider)
	if err != nil {
		return err
	}

	exportOpts := []archive.ExportOpt{
		archive.WithManifest(desc, ref),
	}

	return archive.Export(ctx, provider, w, exportOpts...)
}
