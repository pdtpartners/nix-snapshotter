package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"

	"golang.org/x/sys/unix"
	"google.golang.org/grpc"

	snapshotsapi "github.com/containerd/containerd/api/services/snapshots/v1"
	"github.com/containerd/containerd/contrib/snapshotservice"
	"github.com/containerd/containerd/log"
	"github.com/coreos/go-systemd/v22/daemon"
	"github.com/pdtpartners/nix-snapshotter/pkg/nix"
)

const (
	defaultAddress = "/run/containerd-nix/containerd-nix.sock"
	defaultRootDir = "/var/lib/containerd-nix"
)

func main() {
	addr := defaultAddress
	root := defaultRootDir
	if len(os.Args) == 3 {
		addr = os.Args[1]
		root = os.Args[2]
	}

	err := run(context.Background(), addr, root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "nix-snapshotter: %s\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, addr, root string) error {
	// Prepare the directory for the socket
	err := os.MkdirAll(filepath.Dir(addr), 0700)
	if err != nil {
		return fmt.Errorf("failed to create directory %q: %w", filepath.Dir(addr), err)
	}

	// Try to remove the socket file to avoid EADDRINUSE
	err = os.RemoveAll(addr)
	if err != nil {
		return fmt.Errorf("failed to remove %q: %w", addr, err)
	}

	sn, err := nix.NewSnapshotter(root, "/nix/store")
	if err != nil {
		return err
	}
	service := snapshotservice.FromSnapshotter(sn)

	rpc := grpc.NewServer()
	snapshotsapi.RegisterSnapshotsServer(rpc, service)

	l, err := net.Listen("unix", addr)
	if err != nil {
		return err
	}

	errCh := make(chan error, 1)
	go func() {
		if err := rpc.Serve(l); err != nil {
			errCh <- fmt.Errorf("error on serving via socket %q: %w", addr, err)
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
