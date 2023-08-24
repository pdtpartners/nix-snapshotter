package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
	"github.com/sirupsen/logrus"
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
	if err := App().Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "nix-snapshotter: %s\n", err)
		os.Exit(1)
	}
}

func App() *cli.App {
	app := cli.NewApp()
	app.Name = "nix-snapshotter"
	app.Version = "1.0.0"
	app.Usage = "A containerd snapshotter that understands nix store paths natively"
	app.Description = `nix-snapshotter is a containerd proxy snapshotter whose
daemon can be started using this command. Containerd communicates with proxy
snapshotters over GRPC, so this daemon will start a GRPC server listening on
a unix domain socket.

This snapshotter depends on access to a "nix" binary to substitute store paths
and creating GC roots during unpacking of a container image with nix store path
annotations. At runtime, the container rootfs will be backed by a read-writable
overlayfs root along with bind mounts for every nix store path required.`
	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:    "log-level",
			Aliases: []string{"l"},
			Value:   logrus.InfoLevel.String(),
			Usage:   "Set the logging level [trace, debug, info, warn, error, fatal, panic]",
		},
		&cli.StringFlag{
			Name:    "config",
			Aliases: []string{"c"},
			Value:   filepath.Join(defaultConfigDir, "config.toml"),
			Usage:   "Path to the configuration file",
		},
		&cli.StringFlag{
			Name:    "address",
			Aliases: []string{"a"},
			Usage:   "Address for nix-snapshotter's GRPC server",
		},
		&cli.StringFlag{
			Name:  "root",
			Usage: "Directory where nix-snapshotter will store persistent data",
		},
	}
	app.Action = func(c *cli.Context) error {
		lvl, err := logrus.ParseLevel(c.String("log-level"))
		if err != nil {
			log.L.WithError(err).Fatal("failed to prepare logger")
		}
		logrus.SetLevel(lvl)
		logrus.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: log.RFC3339NanoFixed,
		})

		ctx := log.WithLogger(context.Background(), log.L)

		var cfg Config
		if _, err := os.Stat(c.String("config")); os.IsNotExist(err) {
			log.G(ctx).Infof("failed to find config at %q switching to default values", c.String("config"))
			cfg = *DefaultConfig()
		} else if err != nil {
			return err
		}
		data, err := os.ReadFile(c.String("config"))
		if err != nil {
			return err
		}
		err = toml.Unmarshal([]byte(data), &cfg)
		if err != nil {
			return err
		}

		//Flags always override
		if c.String("root") != "" {
			cfg.Root = c.String("root")
		}
		if c.String("address") != "" {
			cfg.Address = c.String("address")
		}
		return serve(ctx, cfg)
	}
	return app
}

func serve(ctx context.Context, cfg Config) error {

	log.G(ctx).WithField("root", cfg.Root).Info("Starting the nix-snapshotter")

	// Prepare the directory for the socket
	err := os.MkdirAll(filepath.Dir(cfg.Address), 0700)
	if err != nil {
		return fmt.Errorf("failed to create directory %q: %w", filepath.Dir(cfg.Address), err)
	}

	// Try to remove the socket file to avoid EADDRINUSE
	err = os.RemoveAll(cfg.Address)
	if err != nil {
		return fmt.Errorf("failed to remove %q: %w", cfg.Address, err)
	}

	sn, err := nix.NewSnapshotter(cfg.Root, "/nix/store")
	if err != nil {
		return err
	}

	service := snapshotservice.FromSnapshotter(sn)

	rpc := grpc.NewServer()
	snapshotsapi.RegisterSnapshotsServer(rpc, service)

	l, err := net.Listen("unix", cfg.Address)
	if err != nil {
		return err
	}

	errCh := make(chan error, 1)
	go func() {
		if err := rpc.Serve(l); err != nil {
			errCh <- fmt.Errorf("error on serving via socket %q: %w", cfg.Address, err)
		}
	}()

	log.G(ctx).WithField("address", cfg.Address).Info("Serving...")

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
