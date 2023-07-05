package nix2container

import (
	"archive/tar"
	"errors"
	"os"
)

// ImageType defines the base image formats that nix2container supports.
type ImageType int

const (
	// ImageTypeUnknown is an unrecognized base image format.
	ImageTypeUnknown ImageType = iota

	// ImageTypeOCITarball indicates the base image path contains an OCI archive
	// tarball.
	ImageTypeOCITarball

	// ImageTypeNix indicates the base image path contains a JSON artifact
	// produced by another nix2container image.
	ImageTypeNix
)

// DetectImageType returns the ImageType contained at imagePath.
func DetectImageType(imagePath string) (ImageType, error) {
	f, err := os.Open(imagePath)
	if err != nil {
		return ImageTypeUnknown, err
	}
	defer f.Close()

	// Peak ahead to see if the file is a tarball.
	tr := tar.NewReader(f)
	_, err = tr.Next()
	if err != nil {
		if errors.Is(err, tar.ErrHeader) {
			return ImageTypeNix, nil
		}
		return ImageTypeUnknown, err
	}

	// Assume the tarball is of an OCI archive layout.
	return ImageTypeOCITarball, nil
}
