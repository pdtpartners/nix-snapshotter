package nix

import (
	"context"
	"errors"
	"os/exec"

	"github.com/containerd/containerd/log"
)

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
	args := []string{"build"}
	if outLink == "" {
		args = append(args, "--no-link")
	} else {
		args = append(args, "--out-link", outLink)
	}
	args = append(args, nixStorePath)

	_, err := exec.Command("nix", args...).Output()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		log.G(ctx).
			WithField("nixStorePath", nixStorePath).
			Debugf("Failed to create gc root:\n%s", string(exitErr.Stderr))
	}
	return err
}
