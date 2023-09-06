package main

import (
	"context"
	"fmt"
	"os"

	"github.com/hinshun/nix-snapshotter/pkg/command"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	app := command.NewApp(ctx)
	err := app.Run(os.Args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "nix2container: %s\n", err)
		os.Exit(1)
	}
}
