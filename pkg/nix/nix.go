package nix

import (
	"context"
	"os/exec"
	"strings"

	"github.com/containerd/containerd/v2/plugins/snapshots/overlay/overlayutils"
	"github.com/containerd/log"
)

// Supported returns nil when the remote snapshotter is functional on the system with the root directory.
// Supported is not called during plugin initialization, but exposed for downstream projects which uses
// this snapshotter as a library.
func Supported(root string) error {
	return overlayutils.Supported(root)
}

// Config is used to configure common options.
type Config struct {
	nixBuilder NixBuilder
}

func (c *Config) apply(fn func(c *Config)) {
	fn(c)
}

// Opt is a common option for nix related services.
type Opt interface {
	SnapshotterOpt
	ImageServiceOpt
}

type optFn func(*Config)

func (fn optFn) SetSnapshotterOpt(cfg *SnapshotterConfig) {
	cfg.apply(fn)
}

func (fn optFn) SetImageServiceOpt(cfg *ImageServiceConfig) {
	cfg.apply(fn)
}

// WithNixBuilder is an option to override the default NixBuilder.
func WithNixBuilder(nixBuilder NixBuilder) Opt {
	return optFn(func(c *Config) {
		c.nixBuilder = nixBuilder
	})
}

// NixBuilder is a function that is able to substitute a nix store path and
// optionally create an out-link. outLink may be empty in which case out-links
// are not needed.
//
// Typically this is implemented by `nix build --out-link ${outLink} ${nixStorePath}`,
// however it can also be done by `nix copy` and alternate implementations.
type NixBuilder func(ctx context.Context, outLink, nixStorePath string) error

func defaultNixBuilder(ctx context.Context, outLink, nixStorePath string) error {
	var args []string
	if outLink != "" {
		args = append(args, "--add-root", outLink)
	}
	args = append(args, "--realise", nixStorePath)

	log.G(ctx).Infof("[nix-snapshotter] Calling nix-store %s", strings.Join(args, " "))
	out, err := exec.Command("nix-store", args...).CombinedOutput()
	if err != nil {
		log.G(ctx).
			WithField("nixStorePath", nixStorePath).
			Errorf("Failed to create gc root: %s\n%s", err, string(out))
	}
	return err
}

// NewExternalBuilder returns a NixBuilder from an external executable with
// two arguments: an out-link path, and a Nix store path.
func NewExternalBuilder(name string) NixBuilder {
	return func(ctx context.Context, outLink, nixStorePath string) error {
		out, err := exec.Command(name, outLink, nixStorePath).CombinedOutput()
		if err != nil {
			log.G(ctx).
				WithField("nixStorePath", nixStorePath).
				Errorf("Failed to run external nix builder: %s\n%s", err, string(out))
		}
		return err
	}
}
