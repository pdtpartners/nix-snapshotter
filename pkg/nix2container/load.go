package nix2container

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/transfer"
	tarchive "github.com/containerd/containerd/v2/core/transfer/archive"
	"github.com/containerd/containerd/v2/core/transfer/image"
	"github.com/containerd/log"
	"github.com/containerd/platforms"
)

func Load(ctx context.Context, client *client.Client, archivePath string) (client.Image, error) {
	log.G(ctx).WithField("archive", archivePath).Info("Loading archive")
	f, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	src := tarchive.NewImageImportStream(f, "")

	platSpec := platforms.DefaultSpec()
	prefix := fmt.Sprintf("import-%s", filepath.Base(archivePath))
	storeOpts := []image.StoreOpt{
		image.WithPlatforms(platSpec),
		image.WithUnpack(platSpec, "nix"),
		// WithNamedPrefix is necessary for containerd's Transfer service to create
		// an image with the reference found inside the OCI tarball.
		image.WithNamedPrefix(prefix, true),
	}

	dest := image.NewStore("", storeOpts...)

	var ref string
	progressFunc := func(p transfer.Progress) {
		if p.Event == "saved" {
			ref = p.Name
		}
	}

	log.G(ctx).Info("Importing image")
	err = client.Transfer(ctx, src, dest, transfer.WithProgress(progressFunc))
	if err != nil {
		return nil, err
	}

	img, err := client.GetImage(ctx, ref)
	if err != nil {
		return nil, err
	}

	// Workaround for k8s to ensure that the reference is tied to the index
	// descriptor.
	//
	// TODO: Containerd's transfer importer does call `Store` as well but has a
	// bug that skips or has an issue with the index descriptor.
	_, err = image.NewStore(ref).Store(ctx, img.Target(), client.ImageService())
	if err != nil {
		return nil, err
	}

	log.G(ctx).WithField("ref", ref).Info("Created image")
	return img, nil
}
