package nix2container

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/containerd/containerd/pkg/cri/config"
	"github.com/containerd/containerd/pkg/cri/server"
	"github.com/containerd/containerd/remotes"
	"github.com/containerd/containerd/remotes/docker"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// defaultPusher returns a remotes.Pusher that automatically authenticates
// using docker login credentials.
func defaultPusher(ctx context.Context, ref string) (remotes.Pusher, error) {
	aopts, err := defaultAuthorizerOpts()
	if err != nil {
		return nil, err
	}

	ropts := []docker.RegistryOpt{
		docker.WithAuthorizer(docker.NewDockerAuthorizer(aopts...)),
	}
	resolver := docker.NewResolver(docker.ResolverOptions{
		Hosts: docker.ConfigureDefaultRegistries(ropts...),
	})

	return resolver.Pusher(ctx, ref)
}

// defaultAuthorizerOpts returns docker authorizer options to authenticate
// using docker login credentials.
func defaultAuthorizerOpts() ([]docker.AuthorizerOpt, error) {
	dt, err := ioutil.ReadFile(os.Getenv("HOME") + "/.docker/config.json")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	var aopts []docker.AuthorizerOpt
	if len(dt) > 0 {
		var registry config.Registry
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
