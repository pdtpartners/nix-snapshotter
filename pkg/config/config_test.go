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
		name     string
		setup    func(ctx context.Context, testDir string) (*Config, error)
		expected *Config
	}

	for _, tc := range []testCase{
		{
			"defaults",
			func(ctx context.Context, testDir string) (*Config, error) {
				return New(), nil
			},
			New(),
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
				Root:    defaultRoot,
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
				Root:    defaultRoot,
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
				}

				return cfg, cfg.Merge(flagCfg)
			},
			&Config{
				Address: "/run/barbaz/barbaz.sock",
				Root:    defaultRoot,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			testDir := t.TempDir()
			actual, err := tc.setup(context.Background(), testDir)
			require.NoError(t, err)
			require.Equal(t, tc.expected, actual)
		})
	}
}
