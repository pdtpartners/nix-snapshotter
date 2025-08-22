package nix2container

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"runtime"

	"github.com/containerd/log"
	"github.com/pdtpartners/nix-snapshotter/types"
)

// BuildOpt applies changes to BuildOptions
type BuildOpt func(*BuildOpts)

// BuildOpts contains options concerning how nix images are built.
type BuildOpts struct {
	FromImage string
}

// WithFromImage specifies a base image to build the image from.
func WithFromImage(fromImage string) BuildOpt {
	return func(o *BuildOpts) {
		o.FromImage = fromImage
	}
}

// Build builds an image specification.
func Build(ctx context.Context, configPath, closurePath, copyToRootPath string, opts ...BuildOpt) (*types.Image, error) {
	var bOpts BuildOpts
	for _, opt := range opts {
		opt(&bOpts)
	}

	image := &types.Image{
		Architecture: runtime.GOARCH,
		OS:           runtime.GOOS,
		BaseImage:    bOpts.FromImage,
	}
	log.G(ctx).
		WithField("arch", image.Architecture).
		WithField("os", image.OS).
		WithField("base-image", image.BaseImage).
		Infof("Building image")

	dt, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(dt, &image.Config)
	if err != nil {
		return nil, err
	}

	image.NixStorePaths, err = readClosure(configPath, closurePath)
	if err != nil {
		return nil, err
	}
	log.G(ctx).
		WithField("closure-count", len(image.NixStorePaths)).
		Infof("Read runtime inputs from closure file")

	dt, err = os.ReadFile(copyToRootPath)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(dt, &image.CopyToRoots)
	if err != nil {
		return nil, err
	}

	return image, nil
}

func readClosure(configPath, closurePath string) ([]string, error) {
	f, err := os.Open(closurePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var nixStorePaths []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		nixStorePath := scanner.Text()
		if nixStorePath == configPath {
			continue
		}
		nixStorePaths = append(nixStorePaths, nixStorePath)
	}
	if scanner.Err() != nil {
		return nil, scanner.Err()
	}

	return nixStorePaths, nil
}
