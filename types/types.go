package types

import (
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

type Image struct {
	Config        ocispec.ImageConfig `json:"config"`
	BaseImage     string              `json:"base-image,omitempty"`
	Architecture  string              `json:"architecture"`
	OS            string              `json:"os"`
	ClosurePath   string              `json:"closure-path"`
	NixStorePaths []string            `json:"nix-store-paths,omitempty"`
	CopyToRoots   []string            `json:"copy-to-roots,omitempty"`
}

type OCIManifest struct {
	Config   string   `json:"Config"`
	RepoTags []string `json:"RepoTags"`
	Layers   []string `json:"Layers"`
}
