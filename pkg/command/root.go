package command

import (
	"context"

	cli "github.com/urfave/cli/v2"
)

func NewApp(ctx context.Context) *cli.App {
	return &cli.App{
		Name:  "nix2container",
		Usage: "convert nix store paths into a container image",
		Before: func(c *cli.Context) error {
			c.Context = ctx
			return nil
		},
		Commands: []*cli.Command{
			buildCommand,
			exportCommand,
			pushCommand,
		},
	}
}
