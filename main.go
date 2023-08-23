package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"

	snapshotsapi "github.com/containerd/containerd/api/services/snapshots/v1"
	"github.com/containerd/containerd/contrib/snapshotservice"
	"github.com/containerd/containerd/log"
	"github.com/coreos/go-systemd/v22/daemon"
	"github.com/pdtpartners/nix-snapshotter/pkg/nix"
	"github.com/urfave/cli/v2"
)

type Config struct {
	Address string
	Root    string
}

func DefaultConfig() *Config {
	return &Config{
		Address: "/run/nix-snapshotter/nix-snapshotter.sock",
		Root:    "/var/lib/nix-snapshotter",
	}
}

func main() {
	var configLocation, root, address string
	ctx := context.Background()
	app := &cli.App{
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "config",
				Aliases:     []string{"conf", "config-path"},
				Value:       "/etc/nix-snapshotter/config.toml",
				Usage:       "Path to the configuration file",
				Destination: &configLocation,
			},
			&cli.StringFlag{
				Name:        "address",
				Aliases:     []string{"a"},
				Usage:       "Address for nix-shnapshotter's GRPC server",
				Destination: &address,
			},
			&cli.StringFlag{
				Name:        "root",
				Usage:       "nix-snapshotter root directory",
				Destination: &root,
			},
		},
		Commands: []*cli.Command{
			{
				Name:  "start",
				Usage: "start the nix snapshotter",
				Action: func(*cli.Context) error {
					err := run(ctx, configLocation, root, address)
					if err != nil {
						fmt.Fprintf(os.Stderr, "nix-snapshotter: %s\n", err)
						os.Exit(1)
					}
					return nil
				},
			},
		},
		Name:  "nix-snapshotter",
		Usage: "A containerd remote snapshotter that prepares container rootfs from nix store directly",
	}
	if err := app.Run(os.Args); err != nil {
		log.G(ctx).Fatal(err)
	}
}

func run(ctx context.Context, configLocation, root, address string) error {
	var conf Config

	if _, err := os.Stat(configLocation); os.IsNotExist(err) {
		log.G(ctx).Infof("failed to find config at %q switching to default values", configLocation)
		conf = *DefaultConfig()
	} else if err != nil {
		return err
	}

	data, err := os.ReadFile(configLocation)
	if err != nil {
		return err
	}
	err = toml.Unmarshal([]byte(data), &conf)
	if err != nil {
		return err
	}

	if root != "" {
		conf.Root = root
	}
	if address != "" {
		conf.Address = address
	}

	// Prepare the directory for the socket
	err = os.MkdirAll(filepath.Dir(conf.Address), 0700)
	if err != nil {
		return fmt.Errorf("failed to create directory %q: %w", filepath.Dir(conf.Address), err)
	}

	// Try to remove the socket file to avoid EADDRINUSE
	err = os.RemoveAll(conf.Address)
	if err != nil {
		return fmt.Errorf("failed to remove %q: %w", conf.Address, err)
	}

	sn, err := nix.NewSnapshotter(conf.Root, "/nix/store")
	if err != nil {
		return err
	}
	service := snapshotservice.FromSnapshotter(sn)

	rpc := grpc.NewServer()
	snapshotsapi.RegisterSnapshotsServer(rpc, service)

	l, err := net.Listen("unix", conf.Address)
	if err != nil {
		return err
	}

	errCh := make(chan error, 1)
	go func() {
		if err := rpc.Serve(l); err != nil {
			errCh <- fmt.Errorf("error on serving via socket %q: %w", conf.Address, err)
		}
	}()

	// If NOTIFY_SOCKET is set, nix-snapshotter is run as a systemd service.
	// Notify systemd that the service is ready.
	if os.Getenv("NOTIFY_SOCKET") != "" {
		notified, notifyErr := daemon.SdNotify(false, daemon.SdNotifyReady)
		log.G(ctx).Debugf("SdNotifyReady notified=%v, err=%v", notified, notifyErr)
		defer func() {
			notified, notifyErr := daemon.SdNotify(false, daemon.SdNotifyStopping)
			log.G(ctx).Debugf("SdNotifyStopping notified=%v, err=%v", notified, notifyErr)
		}()
	}

	var s os.Signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, unix.SIGINT, unix.SIGTERM)
	select {
	case s = <-sigCh:
		log.G(ctx).Infof("Got %v", s)
	case err := <-errCh:
		return err
	}
	// if s == unix.SIGINT {
	// 	return nil
	// }
	return nil
}
