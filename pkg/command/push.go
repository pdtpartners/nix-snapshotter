package command

import (
	"fmt"
	"os"

	"github.com/containerd/containerd/v2/plugins/content/local"
	"github.com/pdtpartners/nix-snapshotter/pkg/nix2container"
	cli "github.com/urfave/cli/v2"
)

var pushCommand = &cli.Command{
	Name:  "push",
	Usage: "pushes an OCI archive to a Docker Registry",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "plain-http",
			Value: false,
			Usage: "Allow connections using plain HTTP",
		},
		&cli.StringFlag{
			Name:  "ref",
			Usage: "Image reference to push image to",
		},
	},
	Action: func(c *cli.Context) error {
		if c.NArg() != 1 {
			return fmt.Errorf("must provide exactly 1 args")
		}

		var pushOpts []nix2container.PushOpt
		if c.Bool("plain-http") {
			pushOpts = append(pushOpts, nix2container.WithPlainHTTP())
		}

		root, err := os.MkdirTemp(nix2container.TempDir(), "nix2container-push")
		if err != nil {
			return err
		}

		store, err := local.NewStore(root)
		if err != nil {
			return err
		}

		archivePath := c.Args().Get(0)
		return nix2container.Push(c.Context, store, archivePath, c.String("ref"), pushOpts...)
	},
}
