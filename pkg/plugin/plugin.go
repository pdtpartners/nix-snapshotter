package plugin

import (
	"errors"
	"net"
	"os"
	"path/filepath"

	"github.com/containerd/containerd/v2/plugins"
	"github.com/containerd/log"
	"github.com/containerd/platforms"
	"github.com/containerd/plugin"
	"github.com/containerd/plugin/registry"
	"github.com/pdtpartners/nix-snapshotter/pkg/config"
	"github.com/pdtpartners/nix-snapshotter/pkg/nix"
	"google.golang.org/grpc"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func init() {
	registry.Register(&plugin.Registration{
		Type:   plugins.SnapshotPlugin,
		ID:     "nix",
		Config: &config.Config{},
		InitFn: func(ic *plugin.InitContext) (interface{}, error) {
			ic.Meta.Platforms = append(ic.Meta.Platforms, platforms.DefaultSpec())

			cfg, ok := ic.Config.(*config.Config)
			if !ok {
				return nil, errors.New("invalid nix configuration")
			}

			root := ic.Properties[plugins.PropertyRootDir]
			if cfg.Root != "" {
				root = cfg.Root
			}

			if cfg.ImageService.Enable {
				criAddr := ic.Properties[plugins.PropertyGRPCAddress]
				if containerdAddr := cfg.ImageService.ContainerdAddress; containerdAddr != "" {
					criAddr = containerdAddr
				}
				if criAddr == "" {
					return nil, errors.New("backend CRI service address is not specified")
				}

				ctx := ic.Context
				imageService, err := nix.NewImageService(ctx, criAddr)
				if err != nil {
					return nil, err
				}

				rpc := grpc.NewServer()
				runtime.RegisterImageServiceServer(rpc, imageService)

				// Prepare the directory for the socket.
				err = os.MkdirAll(filepath.Dir(cfg.Address), 0o700)
				if err != nil {
					return nil, err
				}

				// Try to remove the socket file to avoid EADDRINUSE.
				err = os.RemoveAll(cfg.Address)
				if err != nil {
					return nil, err
				}

				l, err := net.Listen("unix", cfg.Address)
				if err != nil {
					return nil, err
				}

				go func() {
					err := rpc.Serve(l)
					if err != nil {
						log.G(ctx).WithError(err).Warnf("error on serving nix-snapshotter image service via socket %q", cfg.Address)
					}
				}()
			}

			ic.Meta.Exports["root"] = root

			var snapshotterOpts []nix.SnapshotterOpt
			if cfg.ExternalBuilder != "" {
				snapshotterOpts = append(snapshotterOpts, nix.WithNixBuilder(nix.NewExternalBuilder(cfg.ExternalBuilder)))
			}

			return nix.NewSnapshotter(root, snapshotterOpts...)
		},
	})
}
