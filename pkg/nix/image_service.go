package nix

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/content/local"
	"github.com/containerd/containerd/log"
	"github.com/containerd/containerd/namespaces"
	"github.com/pdtpartners/nix-snapshotter/pkg/nix2container"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
)

type imageService struct {
	client             *containerd.Client
	imageServiceClient runtime.ImageServiceClient
	nixBuilder         NixBuilder
}

func NewImageService(ctx context.Context, containerdAddr string, opts ...NixOpt) (runtime.ImageServiceServer, error) {
	config := NixConfig{
		nixBuilder: defaultNixBuilder,
	}
	for _, opt := range opts {
		opt(&config)
	}

	client, err := containerd.New(containerdAddr)
	if err != nil {
		return nil, err
	}

	return &imageService{
		client:             client,
		imageServiceClient: runtime.NewImageServiceClient(client.Conn()),
		nixBuilder:         config.nixBuilder,
	}, nil
}

// ListImages lists existing images.
func (is *imageService) ListImages(ctx context.Context, req *runtime.ListImagesRequest) (*runtime.ListImagesResponse, error) {
	return is.imageServiceClient.ListImages(ctx, req)
}

// ImageStatus returns the status of the image. If the image is not
// present, returns a response with ImageStatusResponse.Image set to
// nil.
func (is *imageService) ImageStatus(ctx context.Context, req *runtime.ImageStatusRequest) (*runtime.ImageStatusResponse, error) {
	return is.imageServiceClient.ImageStatus(ctx, req)
}

// PullImage pulls an image with authentication config.
func (is *imageService) PullImage(ctx context.Context, req *runtime.PullImageRequest) (*runtime.PullImageResponse, error) {
	ref := req.Image.Image
	if !strings.HasPrefix(ref, "nix:0") {
		log.G(ctx).WithField("ref", ref).Info("[image-service] Falling back to CRI pull image")
		resp, err := is.imageServiceClient.PullImage(ctx, req)
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

	root, err := os.MkdirTemp(nix2container.TempDir(), "nix-snapshotter-pull")
	if err != nil {
		return nil, err
	}

	store, err := local.NewStore(root)
	if err != nil {
		return nil, err
	}

	log.G(ctx).Info("[image-service] Loading nix image archive")
	ctx = namespaces.WithNamespace(ctx, "k8s.io")
	img, err := nix2container.Load(ctx, is.client, store, archivePath)
	if err != nil {
		return nil, err
	}

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

// RemoveImage removes the image.
// This call is idempotent, and must not return an error if the image has
// already been removed.
func (is *imageService) RemoveImage(ctx context.Context, req *runtime.RemoveImageRequest) (*runtime.RemoveImageResponse, error) {
	return is.imageServiceClient.RemoveImage(ctx, req)
}

// ImageFSInfo returns information of the filesystem that is used to store images.
func (is *imageService) ImageFsInfo(ctx context.Context, req *runtime.ImageFsInfoRequest) (*runtime.ImageFsInfoResponse, error) {
	return is.imageServiceClient.ImageFsInfo(ctx, req)
}
