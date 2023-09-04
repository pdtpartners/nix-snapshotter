package config

import (
	"context"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfig(t *testing.T) {
	type testCase struct {
		name  string
		setup func(ctx context.Context, testDir string) (*Config, error)
		delta *Config
	}

	for _, tc := range []testCase{
		{
			"defaults",
			func(ctx context.Context, testDir string) (*Config, error) {
				return New(), nil
			},
			&Config{},
		},
		{
			"merge",
			func(ctx context.Context, testDir string) (*Config, error) {
				cfg := New()
				override := &Config{
					Address: "/run/foobar/foobar.sock",
				}
				return cfg, cfg.Merge(override)
			},
			&Config{
				Address: "/run/foobar/foobar.sock",
			},
		},
		{
			"load",
			func(ctx context.Context, testDir string) (*Config, error) {
				cfg := New()

				config := []byte(`address = "/run/foobar/foobar.sock"`)
				configPath := filepath.Join(testDir, "config.toml")
				err := ioutil.WriteFile(configPath, config, 0o755)
				if err != nil {
					return nil, err
				}

				return cfg, cfg.Load(ctx, configPath)
			},
			&Config{
				Address: "/run/foobar/foobar.sock",
			},
		},
		{
			"load and merge",
			func(ctx context.Context, testDir string) (*Config, error) {
				cfg := New()

				config := []byte(`address = "/run/foobar/foobar.sock"`)
				configPath := filepath.Join(testDir, "config.toml")
				err := ioutil.WriteFile(configPath, config, 0o755)
				if err != nil {
					return nil, err
				}

				err = cfg.Load(ctx, configPath)
				if err != nil {
					return nil, err
				}

				flagCfg := &Config{
					Address: "/run/barbaz/barbaz.sock",
					Root:    "/var/lib/barbaz",
				}

				return cfg, cfg.Merge(flagCfg)
			},
			&Config{
				Address: "/run/barbaz/barbaz.sock",
				Root:    "/var/lib/barbaz",
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			testDir := t.TempDir()
			actual, err := tc.setup(context.Background(), testDir)
			require.NoError(t, err)

			expected := New()
			err = expected.Merge(tc.delta)
			require.NoError(t, err)

			require.Equal(t, expected, actual)
		})
	}
}
