package nix2container

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

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

	saved := newSavedImageName()

	log.G(ctx).Info("Importing image")
	err = client.Transfer(ctx, src, dest, transfer.WithProgress(saved.progress))
	if err != nil {
		return nil, err
	}

	ref, err := saved.wait(ctx, savedImageNameTimeout)
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

// savedImageNameTimeout bounds how long Load waits, after Transfer returns,
// for the "saved" progress event that names the imported image.
const savedImageNameTimeout = 30 * time.Second

// savedImageName records the image name from a transfer's "saved" progress
// event. containerd's transfer client delivers progress events from a
// goroutine that Transfer does not wait for, so the event can arrive after
// Transfer has returned. Reading the name straight away could then look up
// image "" and fail with `image "": not found`.
type savedImageName struct {
	once  sync.Once
	name  string
	saved chan struct{}
}

func newSavedImageName() *savedImageName {
	return &savedImageName{saved: make(chan struct{})}
}

// progress is a transfer.ProgressFunc that records the first "saved" event.
func (s *savedImageName) progress(p transfer.Progress) {
	if p.Event != "saved" {
		return
	}
	s.once.Do(func() {
		s.name = p.Name
		close(s.saved)
	})
}

// wait returns the saved image name once its event has been delivered.
func (s *savedImageName) wait(ctx context.Context, timeout time.Duration) (string, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-s.saved:
		return s.name, nil
	case <-timer.C:
		return "", fmt.Errorf("image import finished without reporting the saved image name within %s", timeout)
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
