package command

import (
	"context"

	"github.com/containerd/log"
	"github.com/sirupsen/logrus"
	cli "github.com/urfave/cli/v2"
)

var defaultLogLevel = logrus.InfoLevel

func NewApp(ctx context.Context) *cli.App {
	return &cli.App{
		Name:  "nix2container",
		Usage: "convert nix store paths into a container image",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "log-level",
				Aliases: []string{"l"},
				Value:   defaultLogLevel.String(),
				Usage:   "Set the logging level [trace, debug, info, warn, error, fatal, panic]",
			},
			&cli.StringFlag{
				Name:    "address",
				Aliases: []string{"a"},
				Value:   "/run/containerd/containerd.sock",
				Usage:   "containerd address",
				EnvVars: []string{"CONTAINERD_ADDRESS"},
			},
			&cli.StringFlag{
				Name:    "namespace",
				Aliases: []string{"n"},
				Value:   "default",
				Usage:   "containerd namespace",
				EnvVars: []string{"CONTAINERD_NAMESPACE"},
			},
		},
		Before: func(c *cli.Context) error {
			lvl, err := logrus.ParseLevel(c.String("log-level"))
			if err != nil {
				return err
			}
			logrus.SetLevel(lvl)

			logrus.SetFormatter(&logrus.TextFormatter{
				FullTimestamp:   true,
				TimestampFormat: log.RFC3339NanoFixed,
			})
			c.Context = log.WithLogger(ctx, log.L)
			return nil
		},
		Commands: []*cli.Command{
			buildCommand,
			pushCommand,
			loadCommand,
		},
	}
}
