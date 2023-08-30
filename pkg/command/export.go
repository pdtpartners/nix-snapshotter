package command

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/pdtpartners/nix-snapshotter/pkg/nix2container"
	"github.com/pdtpartners/nix-snapshotter/types"
	cli "github.com/urfave/cli/v2"
)

var exportCommand = &cli.Command{
	Name:  "export",
	Usage: "exports image into a OCI archive",
	Action: func(c *cli.Context) error {
		if c.NArg() != 3 {
			return fmt.Errorf("must provide exactly 2 args")
		}

		args := c.Args()
		imageJSONPath, ref, outPath := args.Get(0), args.Get(1), args.Get(2)

		fmt.Printf("nix2container export %s %s %s\n", imageJSONPath, ref, outPath)
		return export(c.Context, imageJSONPath, ref, outPath)
	},
}

func export(ctx context.Context, imageJSONPath, ref, outPath string) (err error) {
	dt, err := os.ReadFile(imageJSONPath)
	if err != nil {
		return err
	}

	var image types.Image
	err = json.Unmarshal(dt, &image)
	if err != nil {
		return err
	}

	var w io.Writer
	if outPath == "-" {
		w = os.Stdout
	} else {
		f, err := os.Create(outPath)
		if err != nil {
			return err
		}
		defer func() {
			if cerr := f.Close(); err != nil {
				err = cerr
			}
		}()
		w = f
	}

	return nix2container.Export(ctx, image, ref, w)
}
