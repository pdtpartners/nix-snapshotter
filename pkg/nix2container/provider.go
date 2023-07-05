package nix2container

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/errdefs"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// InmemoryProvider implements the content.Provider interface in memory.
type InmemoryProvider struct {
	content map[digest.Digest][]byte
	mu      sync.RWMutex
}

// NewInmemoryProvider returns a new instance of an InmemoryProvider.
func NewInmemoryProvider() *InmemoryProvider {
	return &InmemoryProvider{
		content: make(map[digest.Digest][]byte),
	}
}

// AddBlob adds a data blob to the provider and returns its descriptor.
func (ip *InmemoryProvider) AddBlob(mediaType string, v interface{}) (desc ocispec.Descriptor, err error) {
	dt, ok := v.([]byte)
	if !ok {
		dt, err = json.MarshalIndent(v, "", "  ")
		if err != nil {
			return
		}
	}

	desc = ocispec.Descriptor{
		MediaType: mediaType,
		Digest:    digest.FromBytes(dt),
		Size:      int64(len(dt)),
	}
	ip.mu.Lock()
	ip.content[desc.Digest] = dt
	return
}

func (ip *InmemoryProvider) ReaderAt(ctx context.Context, desc ocispec.Descriptor) (content.ReaderAt, error) {
	dt, ok := ip.content[desc.Digest]
	if !ok {
		return nil, fmt.Errorf("blob not found %s [%s]: %w", desc.Digest, desc.MediaType, errdefs.ErrNotFound)
	}

	r := bytes.NewReader(dt)
	return &readerAt{Reader: r, Closer: io.NopCloser(r), size: int64(r.Len())}, nil
}

// unmarshalFromProvider unmarshals the data retrievable by the provided
// descriptor and stores the result in the value pointed by v.
func unmarshalFromProvider(ctx context.Context, provider content.Provider, desc ocispec.Descriptor, v interface{}) error {
	ra, err := provider.ReaderAt(ctx, desc)
	if err != nil {
		return err
	}

	r := io.NewSectionReader(ra, 0, desc.Size)
	dt, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	return json.Unmarshal(dt, v)
}
