package command

import (
	"fmt"
	"os"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/content/local"
	"github.com/containerd/containerd/namespaces"
	"github.com/pdtpartners/nix-snapshotter/pkg/nix2container"
	cli "github.com/urfave/cli/v2"
)

var loadCommand = &cli.Command{
	Name:  "load",
	Usage: "loads an OCI archive into containerd",
	Flags: []cli.Flag{},
	Action: func(c *cli.Context) error {
		if c.NArg() != 1 {
			return fmt.Errorf("must provide exactly 1 args")
		}

		root, err := os.MkdirTemp(nix2container.TempDir(), "nix2container-load")
		if err != nil {
			return err
		}

		store, err := local.NewStore(root)
		if err != nil {
			return err
		}

		client, err := containerd.New(c.String("address"))
		if err != nil {
			return err
		}

		archivePath := c.Args().Get(0)

		ctx := namespaces.WithNamespace(c.Context, c.String("namespace"))
		_, err = nix2container.Load(ctx, client, store, archivePath)
		return err
	},
}
