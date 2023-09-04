package nix2container

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/images/archive"
	"github.com/containerd/containerd/log"
	tarchive "github.com/containerd/containerd/pkg/transfer/archive"
	"github.com/containerd/containerd/pkg/transfer/image"
	"github.com/containerd/containerd/platforms"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

func Load(ctx context.Context, client *containerd.Client, store content.Store, archivePath string) (containerd.Image, error) {
	dt, err := os.ReadFile(archivePath)
	if err != nil {
		return nil, err
	}
	src := tarchive.NewImageImportStream(bytes.NewReader(dt), ocispec.MediaTypeImageIndex)

	platSpec := platforms.DefaultSpec()
	storeOpts := []image.StoreOpt{
		image.WithUnpack(platSpec, "nix"),
		image.WithPlatforms(platSpec),
	}

	target, err := archive.ImportIndex(ctx, store, bytes.NewReader(dt))
	if err != nil {
		return nil, err
	}

	ref, err := refFromArchive(ctx, store, target)
	if err != nil {
		return nil, err
	}

	dest := image.NewStore(ref, storeOpts...)

	// pf, done := images.ProgressHandler(ctx, os.Stderr)
	// defer done()

	log.G(ctx).WithField("ref", ref).Info("Importing image")
	err = client.Transfer(ctx, src, dest)
	if err != nil {
		return nil, err
	}

	log.G(ctx).WithField("ref", ref).Info("Creating image")
	img := images.Image{Name: ref, Target: target}
	_, err = createImage(ctx, client.ImageService(), img)
	if err != nil {
		return nil, err
	}

	log.G(ctx).WithField("ref", ref).Info("Created image")
	return client.GetImage(ctx, ref)
}

func refFromArchive(ctx context.Context, store content.Store, target ocispec.Descriptor) (ref string, err error) {
	blob, err := content.ReadBlob(ctx, store, target)
	if err != nil {
		return
	}

	var idx ocispec.Index
	if err = json.Unmarshal(blob, &idx); err != nil {
		return
	}

	if len(idx.Manifests) != 1 {
		return "", fmt.Errorf("OCI index had %d manifests", len(idx.Manifests))
	}

	mfst := idx.Manifests[0]
	return mfst.Annotations[images.AnnotationImageName], nil
}

func createImage(ctx context.Context, store images.Store, img images.Image) (images.Image, error) {
	for {
		if created, err := store.Create(ctx, img); err != nil {
			if !errdefs.IsAlreadyExists(err) {
				return images.Image{}, err
			}

			updated, err := store.Update(ctx, img)
			if err != nil {
				// if image was removed, try create again
				if errdefs.IsNotFound(err) {
					continue
				}
				return images.Image{}, err
			}

			img = updated
		} else {
			img = created
		}

		return img, nil
	}
}
