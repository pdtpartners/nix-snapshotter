package nix2container

import (
	"context"
	"os"

	"github.com/containerd/containerd/v2/core/content"
	"github.com/containerd/containerd/v2/core/images/archive"
	"github.com/containerd/containerd/v2/core/remotes"
	"github.com/containerd/log"
	"github.com/containerd/platforms"
	reference "github.com/distribution/reference"
	"github.com/pdtpartners/nix-snapshotter/pkg/dockerconfigresolver"
)

type PushOpt func(*PushConfig)

type PushConfig struct {
	PlainHTTP bool
}

func WithPlainHTTP() PushOpt {
	return func(cfg *PushConfig) {
		cfg.PlainHTTP = true
	}
}

// Push generates a nix-snapshotter image and pushes it to a remote.
func Push(ctx context.Context, store content.Store, archivePath, ref string, opts ...PushOpt) error {
	cfg := &PushConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	desc, err := archive.ImportIndex(ctx, store, f)
	if err != nil {
		return err
	}

	pusher, err := newPusher(ctx, cfg, ref)
	if err != nil {
		return err
	}

	log.G(ctx).WithField("ref", ref).Info("Pushing nix image to registry")
	// Push image and its blobs to a registry.
	return remotes.PushContent(ctx, pusher, desc, store, nil, platforms.All, nil)
}

// newPusher returns a remotes.Pusher that automatically authenticates
// using docker login credentials.
func newPusher(ctx context.Context, cfg *PushConfig, ref string) (remotes.Pusher, error) {
	named, err := reference.ParseDockerRef(ref)
	if err != nil {
		return nil, err
	}
	domain := reference.Domain(named)

	var dockerconfigOpts []dockerconfigresolver.Opt
	if cfg.PlainHTTP {
		dockerconfigOpts = append(dockerconfigOpts, dockerconfigresolver.WithPlainHTTP(true))
	}

	resolver, err := dockerconfigresolver.New(ctx, domain, dockerconfigOpts...)
	if err != nil {
		return nil, err
	}

	return resolver.Pusher(ctx, ref)
}
