package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"

	snapshotsapi "github.com/containerd/containerd/api/services/snapshots/v1"
	"github.com/containerd/containerd/v2/contrib/snapshotservice"
	"github.com/containerd/log"
	"github.com/coreos/go-systemd/v22/daemon"
	"github.com/pdtpartners/nix-snapshotter/pkg/config"
	"github.com/pdtpartners/nix-snapshotter/pkg/nix"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
)

var (
	defaultLogLevel   = logrus.InfoLevel
	defaultConfigPath = "/etc/nix-snapshotter/config.toml"
)

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
snapshotters over gRPC, so this daemon will start a gRPC server listening on
a unix domain socket.

This snapshotter depends on access to a "nix" binary to substitute store paths
and creating GC roots during unpacking of a container image with nix store path
annotations. At runtime, the container rootfs will be backed by a read-writable
overlayfs root along with bind mounts for every nix store path required.`

	// Allow flags to override config attributes.
	flagCfg := &config.Config{}
	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:    "log-level",
			Aliases: []string{"l"},
			Value:   defaultLogLevel.String(),
			Usage:   "Set the logging level [trace, debug, info, warn, error, fatal, panic]",
		},
		&cli.StringFlag{
			Name:    "config",
			Aliases: []string{"c"},
			Value:   defaultConfigPath,
			Usage:   "Path to the configuration file",
		},
		&cli.StringFlag{
			Name:        "address",
			Aliases:     []string{"a"},
			Usage:       "Address for nix-snapshotter's gRPC server",
			Destination: &flagCfg.Address,
		},
		&cli.StringFlag{
			Name:        "root",
			Usage:       "Directory where nix-snapshotter will store persistent data",
			Destination: &flagCfg.Root,
		},
	}

	app.Action = func(c *cli.Context) error {
		lvl, err := logrus.ParseLevel(c.String("log-level"))
		if err != nil {
			return err
		}
		logrus.SetLevel(lvl)
		logrus.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: log.RFC3339NanoFixed,
		})
		ctx := log.WithLogger(context.Background(), log.L)

		// Override defaults with configuration file settings.
		cfg := config.New()
		err = cfg.Load(ctx, c.String("config"))
		if err != nil {
			return err
		}

		// Override config with flag settings.
		err = cfg.Merge(flagCfg)
		if err != nil {
			return err
		}

		return serve(ctx, cfg)
	}

	return app
}

func serve(ctx context.Context, cfg *config.Config) error {
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

	rpc := grpc.NewServer()
	if cfg.ImageService.Enable {
		imageService, err := nix.NewImageService(ctx, cfg.ImageService.ContainerdAddress)
		if err != nil {
			return err
		}
		runtime.RegisterImageServiceServer(rpc, imageService)
	}

	var snapshotterOpts []nix.SnapshotterOpt
	if cfg.ExternalBuilder != "" {
		snapshotterOpts = append(snapshotterOpts, nix.WithNixBuilder(nix.NewExternalBuilder(cfg.ExternalBuilder)))
	}

	sn, err := nix.NewSnapshotter(cfg.Root, snapshotterOpts...)
	if err != nil {
		return err
	}

	service := snapshotservice.FromSnapshotter(sn)
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
