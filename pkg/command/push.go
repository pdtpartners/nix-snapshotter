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
	Action: func(c *cli.Context) error {
		if c.NArg() != 2 {
			return fmt.Errorf("must provide exactly 2 args")
		}

		args := c.Args()
		imageJSONPath, ref := args.Get(0), args.Get(1)

		fmt.Printf("nix2container push %s %s\n", imageJSONPath, ref)
		return push(c.Context, imageJSONPath, ref)
	},
}

func push(ctx context.Context, imageJSONPath, ref string) error {
	dt, err := os.ReadFile(imageJSONPath)
	if err != nil {
		return err
	}

	var image types.Image
	err = json.Unmarshal(dt, &image)
	if err != nil {
		return err
	}

	return nix2container.Push(ctx, image, ref)
}
