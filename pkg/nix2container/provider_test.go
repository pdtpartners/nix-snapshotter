package nix2container

import (
	"context"
	"encoding/json"
	"runtime"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pdtpartners/nix-snapshotter/pkg/testutil"
	"github.com/pdtpartners/nix-snapshotter/types"
	"github.com/stretchr/testify/require"
)

func verifyProviderData(t *testing.T, providerContent []byte, sourceData interface{}) {
	data, ok := sourceData.([]byte)
	if !ok {
		var err error
		data, err = json.MarshalIndent(sourceData, "", "  ")
		require.NoError(t, err)
	}
	testutil.IsIdentical(t, data, providerContent)
}

func TestAddBlob(t *testing.T) {
	type testCase struct {
		name          string
		setUp         func(t *testing.T, provider *InmemoryProvider) []ocispec.Descriptor
		expectedDescs []ocispec.Descriptor
	}

	for _, tc := range []testCase{
		{
			"empty",
			func(t *testing.T, provider *InmemoryProvider) []ocispec.Descriptor {
				desc, err := provider.AddBlob("", nil)
				require.NoError(t, err)
				return []ocispec.Descriptor{desc}
			},
			[]ocispec.Descriptor{
				{
					Size: 4,
				}},
		},
		{
			"ints",
			func(t *testing.T, provider *InmemoryProvider) []ocispec.Descriptor {
				testInts := []int{1, 2, 3, 4, 5}
				desc, err := provider.AddBlob("ints", testInts)
				require.NoError(t, err)
				verifyProviderData(t, provider.content[desc.Digest], testInts)
				return []ocispec.Descriptor{desc}
			},
			[]ocispec.Descriptor{
				{
					MediaType: "ints",
					Size:      27,
				},
			},
		},
		{
			"string",
			func(t *testing.T, provider *InmemoryProvider) []ocispec.Descriptor {
				testString := `When I was back there in seminary school,
				there was a person there who put forth the proposition that you
				can petition the Lord with prayer`
				desc, err := provider.AddBlob("string", testString)
				require.NoError(t, err)
				verifyProviderData(t, provider.content[desc.Digest], testString)
				return []ocispec.Descriptor{desc}
			},
			[]ocispec.Descriptor{
				{
					MediaType: "string",
					Size:      159,
				},
			},
		},
		{
			"image",
			func(t *testing.T, provider *InmemoryProvider) []ocispec.Descriptor {
				testImage := types.Image{
					Config: ocispec.ImageConfig{
						Entrypoint: []string{
							"/some/file/location1",
						},
					},
					Architecture: runtime.GOARCH,
					OS:           runtime.GOOS,
					StorePaths:   []string{"/some/file/location2", "/some/file/location3"},
					CopyToRoots:  []string{"/some/file/location4", "/some/file/location5"},
					BaseImage:    "someImage",
				}
				desc, err := provider.AddBlob("image", testImage)
				require.NoError(t, err)
				verifyProviderData(t, provider.content[desc.Digest], testImage)
				return []ocispec.Descriptor{desc}
			},
			[]ocispec.Descriptor{
				{
					MediaType: "image",
					Size:      309,
				},
			},
		},
		{
			"multiple_blobs",
			func(t *testing.T, provider *InmemoryProvider) []ocispec.Descriptor {
				testBytes := [][]byte{
					{'A', 'B', 'C'},
					{'D', 'E', 'F'},
					{'G', 'H', 'I'},
					{'J', 'K', 'L'}}

				var (
					desc  ocispec.Descriptor
					err   error
					descs []ocispec.Descriptor
				)
				for _, bytes := range testBytes {
					desc, err = provider.AddBlob("bytes"+string(bytes), bytes)
					require.NoError(t, err)
					descs = append(descs, desc)
				}
				var providerBytes [][]byte
				for _, desc := range descs {
					providerBytes = append(providerBytes, provider.content[desc.Digest])
				}
				testutil.IsIdentical(t, testBytes, providerBytes)
				return descs
			},
			[]ocispec.Descriptor{
				{
					MediaType: "bytesABC",
					Size:      3,
				},
				{
					MediaType: "bytesDEF",
					Size:      3,
				},
				{
					MediaType: "bytesGHI",
					Size:      3,
				},
				{
					MediaType: "bytesJKL",
					Size:      3,
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := NewInmemoryProvider()
			descs := tc.setUp(t, provider)

			// Reset the digest for ease of testing
			for idx, desc := range descs {
				desc.Digest = ""
				testutil.IsIdentical(t, desc, tc.expectedDescs[idx])
			}

		})
	}
}

func TestUnmarshalFromProvider(t *testing.T) {
	type testCase struct {
		name  string
		input []any
	}
	ctx := context.Background()
	for _, tc := range []testCase{
		{
			"strings",
			[]any{
				"Well my heart's in The Highlands, gentle and fair",
				"Honey suckle bloomin' in the wildwood air",
				"Bluebells blazing where the Aberdeen waters flow",
				"Well my heart's in The Highlands",
				"I'm gonna go there when I feel good enough to go"},
		},
		{
			"floats",
			[]any{1.0, 3.0, 4.0, 5.0, 6.0, 7.0, 1.0},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := NewInmemoryProvider()
			var descs []ocispec.Descriptor
			for _, testString := range tc.input {
				desc, err := provider.AddBlob("data", testString)
				require.NoError(t, err)
				descs = append(descs, desc)
			}
			var outputItem any
			var output []any
			for _, desc := range descs {
				unmarshalFromProvider(ctx, provider, desc, &outputItem)
				output = append(output, outputItem)
			}
			testutil.IsIdentical(t, tc.input, output)
		})
	}
}
