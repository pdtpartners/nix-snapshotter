package command

import (
	"fmt"

	"github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/pkg/namespaces"
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

		client, err := client.New(c.String("address"))
		if err != nil {
			return err
		}

		archivePath := c.Args().Get(0)

		ctx := namespaces.WithNamespace(c.Context, c.String("namespace"))
		_, err = nix2container.Load(ctx, client, archivePath)
		return err
	},
}
