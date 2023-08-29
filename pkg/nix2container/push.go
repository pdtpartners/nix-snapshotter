package nix2container

import (
	"context"

	"github.com/containerd/containerd/platforms"
	"github.com/containerd/containerd/reference/docker"
	"github.com/containerd/containerd/remotes"
	"github.com/pdtpartners/nix-snapshotter/pkg/dockerconfigresolver"
	"github.com/pdtpartners/nix-snapshotter/types"
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
func Push(ctx context.Context, image types.Image, ref string, opts ...PushOpt) error {
	cfg := &PushConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	provider := NewInmemoryProvider()
	desc, err := Generate(ctx, image, provider)
	if err != nil {
		return err
	}

	pusher, err := newPusher(ctx, cfg, ref)
	if err != nil {
		return err
	}

	// Push image and its blobs to a registry.
	return remotes.PushContent(ctx, pusher, desc, provider, nil, platforms.All, nil)
}

// newPusher returns a remotes.Pusher that automatically authenticates
// using docker login credentials.
func newPusher(ctx context.Context, cfg *PushConfig, ref string) (remotes.Pusher, error) {
	named, err := docker.ParseDockerRef(ref)
	if err != nil {
		return nil, err
	}
	domain := docker.Domain(named)

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
