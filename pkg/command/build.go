package command

import (
	"fmt"

	"github.com/pdtpartners/nix-snapshotter/pkg/nix2container"
	cli "github.com/urfave/cli/v2"
)

var buildCommand = &cli.Command{
	Name:  "build",
	Usage: "builds a container image JSON for nix store paths",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "from-image",
			Usage: "base image to use",
		},
	},
	Action: func(c *cli.Context) error {
		if c.NArg() != 4 {
			return fmt.Errorf("must provide exactly 4 args")
		}

		args := c.Args()
		configPath, storePathsPath, copyToRootPath, outPath := args.Get(0), args.Get(1), args.Get(2), args.Get(3)

		var opts []nix2container.BuildOpt
		if c.IsSet("from-image") {
			opts = append(opts, nix2container.WithFromImage(c.String("from-image")))
		}

		fmt.Printf(
			"nix2container build --from-image %q %s %s %s %s\n",
			c.String("from-image"),
			configPath,
			storePathsPath,
			copyToRootPath,
			outPath,
		)
		return nix2container.Build(c.Context, configPath, storePathsPath, copyToRootPath, outPath, opts...)
	},
}
