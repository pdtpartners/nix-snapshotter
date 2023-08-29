package nix2container

import (
	"context"

	"github.com/containerd/containerd/reference/docker"
	"github.com/containerd/containerd/remotes"
	"github.com/pdtpartners/nix-snapshotter/pkg/dockerconfigresolver"
)

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
