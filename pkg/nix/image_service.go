package nix

import (
	"context"
	"errors"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/log"
	"github.com/pdtpartners/nix-snapshotter/pkg/nix2container"
	"google.golang.org/grpc"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
	goruntime "runtime"
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
	nixSystem          string
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

	var system string
	if goruntime.GOOS == "linux" && goruntime.GOARCH == "amd64" {
		system = "x86_64-linux"
	} else if goruntime.GOOS == "linux" && goruntime.GOARCH == "arm64" {
		system = "aarch64-linux"
	} else {
		log.G(ctx).Fatalf("Cannot derive Nix platform from Go runtime")
	}

	service := &imageService{
		nixBuilder: cfg.nixBuilder,
		nixSystem:  system,
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
	archivePath := getNixStorePath(ctx, ref, is.nixSystem)

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

// getNixStorePath extracts the store path from the image ref.
func getNixStorePath(ctx context.Context, ref string, system string) string {
	path := strings.TrimSuffix(
		strings.TrimPrefix(ref, nix2container.ImageRefPrefix),
		":latest",
	)

	if strings.HasPrefix(path, "/multiarch/") {
		path, _ = strings.CutPrefix(path, "/multiarch/")

		pathsPerSystem := map[string]string{}

		storePathRegex := regexp.MustCompile("/nix/store/[0-9a-z]{32}-[-.+_0-9a-zA-Z]+")

		for path != "" {
			// Extract and remove leading <system>
			// from <system>/nix/store/<hash>/<other_system>/nix/store/<hash>
			system := strings.Split(path, "/")[0]
			path, _ = strings.CutPrefix(path, system)

			// Extract and remove store path
			// from </nix/store/<hash>>/<system>/nix/store/<hash>
			systemPath := string(storePathRegex.Find([]byte(path)))
			path, _ = strings.CutPrefix(path, systemPath)
			path, _ = strings.CutPrefix(path, "/") // remove leading "/"

			pathsPerSystem[system] = systemPath
		}

		var ok bool
		path, ok = pathsPerSystem[system]
		if !ok {
			log.G(ctx).Errorf("Failed to extract path from %s", ref)
		}

	}

	return path
}
