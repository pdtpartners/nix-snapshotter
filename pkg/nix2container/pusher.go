package nix2container

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	criconfig "github.com/containerd/containerd/pkg/cri/config"
	"github.com/containerd/containerd/pkg/cri/server"
	"github.com/containerd/containerd/remotes"
	"github.com/containerd/containerd/remotes/docker"
	"github.com/containerd/containerd/remotes/docker/config"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// newPusher returns a remotes.Pusher that automatically authenticates
// using docker login credentials.
func newPusher(ctx context.Context, cfg *PushConfig, ref string) (remotes.Pusher, error) {
	authOpts, err := defaultAuthorizerOpts()
	if err != nil {
		return nil, err
	}

	// Use local docker login credentials if available to push to DockerHub
	// repositories.
	registryOpts := []docker.RegistryOpt{
		docker.WithAuthorizer(docker.NewDockerAuthorizer(authOpts...)),
	}
	resolverOpts := docker.ResolverOptions{
		Hosts: docker.ConfigureDefaultRegistries(registryOpts...),
	}

	// Allow insecure registries via `WithPlainHTTP()`.
	hostOpts := config.HostOptions{
		DefaultScheme: cfg.DefaultScheme,
	}
	resolverOpts.Hosts = config.ConfigureHosts(ctx, hostOpts)

	resolver := docker.NewResolver(resolverOpts)
	return resolver.Pusher(ctx, ref)
}

// defaultAuthorizerOpts returns docker authorizer options to authenticate
// using docker login credentials.
func defaultAuthorizerOpts() ([]docker.AuthorizerOpt, error) {
	dt, err := os.ReadFile(os.Getenv("HOME") + "/.docker/config.json")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	var aopts []docker.AuthorizerOpt
	if len(dt) > 0 {
		var registry criconfig.Registry
		err = json.Unmarshal(dt, &registry)
		if err != nil {
			return nil, err
		}

		aopts = append(aopts, docker.WithAuthCreds(func(host string) (string, string, error) {
			if host != "registry-1.docker.io" {
				return "", "", fmt.Errorf("unrecognized host %s", host)
			}
			authConfig := registry.Auths["https://index.docker.io/v1/"]
			auth := &runtime.AuthConfig{
				Username:      authConfig.Username,
				Password:      authConfig.Password,
				Auth:          authConfig.Auth,
				IdentityToken: authConfig.IdentityToken,
			}
			return server.ParseAuth(auth, host)
		}))
	}

	return aopts, nil
}
