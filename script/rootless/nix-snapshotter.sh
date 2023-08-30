REPO_DIR=$(git rev-parse --show-toplevel)

./build/bin/nix-snapshotter --log-level debug --config ./build/nix-snapshotter/nix-snapshotter.toml
