package nix

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizedRefFromArchive(t *testing.T) {
	for _, tc := range []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "nix store path with hash prefix",
			path:     "/nix/store/abc123-nix-image-seed-web.tar",
			expected: "docker.io/library/seed-web:latest",
		},
		{
			name:     "simple name",
			path:     "/tmp/nix-image-hello.tar",
			expected: "docker.io/library/hello:latest",
		},
		{
			name:     "bare filename",
			path:     "nix-image-myapp.tar",
			expected: "docker.io/library/myapp:latest",
		},
		{
			name:     "no nix-image prefix",
			path:     "/nix/store/abc123-other-image.tar",
			expected: "",
		},
		{
			name:     "no tar suffix still matches prefix",
			path:     "/tmp/nix-image-hello",
			expected: "docker.io/library/hello:latest",
		},
		{
			name:     "unrelated file",
			path:     "/tmp/something-else.tar",
			expected: "",
		},
		{
			name:     "empty path",
			path:     "",
			expected: "",
		},
		{
			name:     "empty name after prefix",
			path:     "/tmp/nix-image-.tar",
			expected: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ref := normalizedRefFromArchive(tc.path)
			require.Equal(t, tc.expected, ref)
		})
	}
}
