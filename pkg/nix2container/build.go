package nix2container

import (
	"bufio"
	"encoding/json"
	"os"
	"runtime"

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

// Build writes an image JSON to the nix out path.
func Build(configPath, closurePath, copyToRootPath, outPath string, opts ...BuildOpt) error {
	var bOpts BuildOpts
	for _, opt := range opts {
		opt(&bOpts)
	}

	image := types.Image{
		Architecture: runtime.GOARCH,
		OS:           runtime.GOOS,
		BaseImage:    bOpts.FromImage,
	}

	dt, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

	err = json.Unmarshal(dt, &image.Config)
	if err != nil {
		return err
	}

	image.NixStorePaths, err = readClosure(configPath, closurePath)
	if err != nil {
		return err
	}

	dt, err = os.ReadFile(copyToRootPath)
	if err != nil {
		return err
	}

	err = json.Unmarshal(dt, &image.CopyToRoots)
	if err != nil {
		return err
	}

	dt, err = json.MarshalIndent(image, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(outPath, dt, 0o644)
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
