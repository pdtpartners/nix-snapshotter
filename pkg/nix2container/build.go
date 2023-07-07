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
func Build(configPath, storePathsPath, copyToRootPath, outPath string, opts ...BuildOpt) error {
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

	image.StorePaths, err = readStorePaths(configPath, storePathsPath)
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

func readStorePaths(configPath, storePathsPath string) ([]string, error) {
	f, err := os.Open(storePathsPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var storePaths []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		storePath := scanner.Text()
		if storePath == configPath {
			continue
		}
		storePaths = append(storePaths, storePath)
	}
	if scanner.Err() != nil {
		return nil, scanner.Err()
	}

	return storePaths, nil
}
