package nix

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/log"
	"github.com/pdtpartners/nix-snapshotter/pkg/nix2container"
	"google.golang.org/grpc"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
)

var (
	ErrNotInitialized = errors.New("Nix-snapshotter Image Service not yet initialized")
)

// ImageServiceConfig is used to configure the image service instance.
type ImageServiceConfig struct {
	Config
}

// ImageServiceOpt is an option for NewImageService.
type ImageServiceOpt interface {
	SetImageServiceOpt(cfg *ImageServiceConfig)
}

type imageService struct {
	runtime.UnimplementedImageServiceServer
	mu                 sync.Mutex
	client             *client.Client
	imageServiceClient runtime.ImageServiceClient
	nixBuilder         NixBuilder
}

func NewImageService(ctx context.Context, containerdAddr string, opts ...ImageServiceOpt) (runtime.ImageServiceServer, error) {
	cfg := ImageServiceConfig{
		Config: Config{
			nixBuilder: defaultNixBuilder,
		},
	}
	for _, opt := range opts {
		opt.SetImageServiceOpt(&cfg)
	}

	service := &imageService{
		nixBuilder: cfg.nixBuilder,
	}

	go func() {
		log.G(ctx).Debugf("Waiting for CRI service is started...")
		for i := 0; i < 100; i++ {
			client, err := client.New(containerdAddr)
			if err == nil {
				service.mu.Lock()
				service.client = client
				service.imageServiceClient = runtime.NewImageServiceClient(client.Conn().(*grpc.ClientConn))
				service.mu.Unlock()
				log.G(ctx).Info("Connected to backend CRI service")
				return
			}
			log.G(ctx).WithError(err).Warnf("Failed to connect to CRI")
			time.Sleep(10 * time.Second)
		}
		log.G(ctx).Warnf("No connection is available to CRI")
	}()

	return service, nil
}

func (is *imageService) getClient() runtime.ImageServiceClient {
	is.mu.Lock()
	client := is.imageServiceClient
	is.mu.Unlock()
	return client
}

// ListImages lists existing images.
func (is *imageService) ListImages(ctx context.Context, req *runtime.ListImagesRequest) (*runtime.ListImagesResponse, error) {
	client := is.getClient()
	if client == nil {
		return nil, ErrNotInitialized
	}
	return client.ListImages(ctx, req)
}

// ImageStatus returns the status of the image. If the image is not
// present, returns a response with ImageStatusResponse.Image set to
// nil.
func (is *imageService) ImageStatus(ctx context.Context, req *runtime.ImageStatusRequest) (*runtime.ImageStatusResponse, error) {
	client := is.getClient()
	if client == nil {
		return nil, ErrNotInitialized
	}
	return client.ImageStatus(ctx, req)
}

// PullImage pulls an image with authentication config.
func (is *imageService) PullImage(ctx context.Context, req *runtime.PullImageRequest) (*runtime.PullImageResponse, error) {
	client := is.getClient()
	if client == nil {
		return nil, ErrNotInitialized
	}

	ref := req.Image.Image
	if !strings.HasPrefix(ref, nix2container.ImageRefPrefix) {
		log.G(ctx).WithField("ref", ref).Info("[image-service] Falling back to CRI pull image")
		resp, err := client.PullImage(ctx, req)
		return resp, err
	}
	archivePath := strings.TrimSuffix(
		strings.TrimPrefix(ref, nix2container.ImageRefPrefix),
		":latest",
	)

	_, err := os.Stat(archivePath)
	if errors.Is(err, os.ErrNotExist) {
		log.G(ctx).Info("[image-service] Pulling nix image archive")
		err := is.nixBuilder(ctx, "", archivePath)
		if err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}

	log.G(ctx).Info("[image-service] Loading nix image archive")
	ctx = namespaces.WithNamespace(ctx, "k8s.io")
	img, err := nix2container.Load(ctx, is.client, archivePath)
	if err != nil {
		return nil, err
	}

	// Create a fully-qualified Docker reference for the image so that
	// containerd's CRI checkpoint code can find it. The checkpoint check in
	// CreateContainer does LocalResolve → toContainerdImage → ImageService.Get()
	// where Get() normalizes bare names to "docker.io/library/<name>:latest".
	// Without this, CreateContainer fails with CreateContainerError for all
	// nix-snapshotter images on k8s 1.30+.
	is.ensureNormalizedRef(ctx, archivePath, img)

	configDesc, err := img.Config(ctx)
	if err != nil {
		return nil, err
	}
	imageRef := configDesc.Digest.String()

	log.G(ctx).WithField("imageRef", imageRef).Info("[image-service] Successfully pulled image")
	return &runtime.PullImageResponse{
		ImageRef: imageRef,
	}, nil
}

// normalizedRefFromArchive derives a fully-qualified Docker reference from a
// nix image archive path. The archive filename is expected to match the pattern
// "nix-image-<name>.tar", producing "docker.io/library/<name>:latest". Returns
// an empty string if the filename does not match the expected pattern.
func normalizedRefFromArchive(archivePath string) string {
	base := strings.TrimSuffix(filepath.Base(archivePath), ".tar")
	_, name, found := strings.Cut(base, "nix-image-")
	if !found || name == "" {
		return ""
	}
	return "docker.io/library/" + name + ":latest"
}

// ensureNormalizedRef creates a fully-qualified Docker reference in containerd's
// image store so that the CRI checkpoint code in CreateContainer can find it.
func (is *imageService) ensureNormalizedRef(ctx context.Context, archivePath string, img client.Image) {
	normalizedRef := normalizedRefFromArchive(archivePath)
	if normalizedRef == "" {
		return
	}

	imgService := is.client.ImageService()
	existing, err := imgService.Get(ctx, img.Name())
	if err != nil {
		return
	}
	existing.Name = normalizedRef
	if _, err := imgService.Create(ctx, existing); err != nil {
		// Ignore already-exists errors, update if target changed
		if cerr := imgService.Delete(ctx, normalizedRef); cerr == nil {
			imgService.Create(ctx, existing)
		}
	}
	log.G(ctx).WithField("ref", normalizedRef).Info("[image-service] Created normalized image ref")
}

// RemoveImage removes the image.
// This call is idempotent, and must not return an error if the image has
// already been removed.
func (is *imageService) RemoveImage(ctx context.Context, req *runtime.RemoveImageRequest) (*runtime.RemoveImageResponse, error) {
	client := is.getClient()
	if client == nil {
		return nil, ErrNotInitialized
	}
	return client.RemoveImage(ctx, req)
}

// ImageFSInfo returns information of the filesystem that is used to store images.
func (is *imageService) ImageFsInfo(ctx context.Context, req *runtime.ImageFsInfoRequest) (*runtime.ImageFsInfoResponse, error) {
	client := is.getClient()
	if client == nil {
		return nil, ErrNotInitialized
	}
	return client.ImageFsInfo(ctx, req)
}
