REPO_DIR=$(git rev-parse --show-toplevel)

./build/bin/nix-snapshotter --config ./build/nix-snapshotter/nix-snapshotter.toml
