package command

import (
	"fmt"
	"os"

	"github.com/containerd/containerd/v2/plugins/content/local"
	"github.com/pdtpartners/nix-snapshotter/pkg/nix2container"
	cli "github.com/urfave/cli/v2"
)

var buildCommand = &cli.Command{
	Name:  "build",
	Usage: "builds a nix-snapshotter image as a OCI archive",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "from-image",
			Usage: "Specify a base image to build layer ontop of",
		},
		&cli.StringFlag{
			Name:  "config",
			Usage: "Path to an OCI image config JSON",
		},
		&cli.StringFlag{
			Name:  "closure",
			Usage: "Path to a newline delimited list of runtime inputs",
		},
		&cli.StringFlag{
			Name:  "copy-to-root",
			Usage: "Path to a JSON describing copy to root config",
		},
		&cli.StringFlag{
			Name:  "ref",
			Usage: "Specify an alternate image name.",
		},
	},
	Action: func(c *cli.Context) error {
		if c.NArg() != 1 {
			return fmt.Errorf("must provide exactly 1 arg")
		}

		outPath := c.Args().Get(0)
		ref := nix2container.ImageRefPrefix + outPath + ":latest"
		if c.IsSet("ref") {
			ref = c.String("ref")
		}

		var opts []nix2container.BuildOpt
		if c.IsSet("from-image") {
			opts = append(opts, nix2container.WithFromImage(c.String("from-image")))
		}

		ctx := c.Context
		img, err := nix2container.Build(ctx,
			c.String("config"),
			c.String("closure"),
			c.String("copy-to-root"),
			opts...,
		)
		if err != nil {
			return err
		}

		f, err := os.Create(outPath)
		if err != nil {
			return err
		}
		defer func() {
			if cerr := f.Close(); err != nil {
				err = cerr
			}
		}()

		root, err := os.MkdirTemp(nix2container.TempDir(), "nix2container-build")
		if err != nil {
			return err
		}

		store, err := local.NewStore(root)
		if err != nil {
			return err
		}

		return nix2container.Export(ctx, store, img, ref, f)
	},
}
