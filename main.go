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

const defaultConfigDir = "/etc/nix-snapshotter"

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
	ctx := context.Background()
	app := App(ctx)
	if err := app.Run(os.Args); err != nil {
		log.G(ctx).Fatal(err)
	}
}

func App(ctx context.Context) *cli.App {
	var configLocation, root, address string
	var logging bool
	app := cli.NewApp()
	app.Name = "nix-snapshotter"
	app.Version = "1.0.0"
	app.Usage = "A containerd remote snapshotter that prepares container rootfs from nix store directly"
	app.Description = `The easiest way to try this out is to run a NixOS VM with containerd and nix-snapshotter pre-configured. Run nix run .#vm to launch a graphic-less NixOS VM that you can play around with immediately.

nix run \".#vm\"
nixos login: admin (Ctrl-A then X to quit)
Password: admin
sudo nerdctl --snapshotter nix run hinshun/hello:nix`
	app.Flags = []cli.Flag{
		&cli.BoolFlag{
			Name:        "logging",
			Aliases:     []string{"l"},
			Value:       true,
			Usage:       "Enable logging",
			Destination: &logging,
		},
		&cli.StringFlag{
			Name:        "config",
			Aliases:     []string{"c"},
			Value:       filepath.Join(defaultConfigDir, "config.toml"),
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
	}
	app.Commands = []*cli.Command{
		{
			Name:  "start",
			Usage: "start the nix snapshotter",
			Action: func(*cli.Context) error {
				err := run(ctx, configLocation, root, address, logging)
				if err != nil {
					fmt.Fprintf(os.Stderr, "nix-snapshotter: %s\n", err)
					os.Exit(1)
				}
				return nil
			},
		},
	}
	return app
}

func run(ctx context.Context, configLocation, root, address string, logging bool) error {
	var conf Config
	if logging {
		log.G(ctx).Infof("starting nix-snapshotter")
	}

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

	//Flags always override
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

	if logging {
		log.G(ctx).Infof("created snapshotter... 		        \033[35m root_dir=\033[39m%v", conf.Root)
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

	if logging {
		log.G(ctx).Infof("serving... 				 	\033[35m address=\033[39m%v", conf.Address)
	}

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
