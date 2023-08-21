package command

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/pdtpartners/nix-snapshotter/pkg/nix2container"
	"github.com/pdtpartners/nix-snapshotter/types"
	cli "github.com/urfave/cli/v2"
)

var pushCommand = &cli.Command{
	Name:  "push",
	Usage: "pushes the OCI manifest generated from a container image JSON",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name: "plain-http",
			Value: false,
			Usage: "Allow connections using plain HTTP",
		},
	},
	Action: func(c *cli.Context) error {
		if c.NArg() != 2 {
			return fmt.Errorf("must provide exactly 2 args")
		}

		args := c.Args()
		imageJSONPath, ref := args.Get(0), args.Get(1)

		var pushOpts []nix2container.PushOpt
		if c.Bool("plain-http") {
			pushOpts = append(pushOpts, nix2container.WithPlainHTTP())
		}

		fmt.Printf("nix2container push %s %s\n", imageJSONPath, ref)
		return push(c.Context, imageJSONPath, ref, pushOpts...)
	},
}

func push(ctx context.Context, imageJSONPath, ref string, opts ...nix2container.PushOpt) error {
	dt, err := os.ReadFile(imageJSONPath)
	if err != nil {
		return err
	}

	var image types.Image
	err = json.Unmarshal(dt, &image)
	if err != nil {
		return err
	}

	return nix2container.Push(ctx, image, ref, opts...)
}
