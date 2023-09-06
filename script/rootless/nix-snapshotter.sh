REPO_DIR=$(git rev-parse --show-toplevel)

./build/bin/nix-snapshotter \
    $REPO_DIR/build/nix-snapshotter/nix-snapshotter.sock \
    $REPO_DIR/build/nix-snapshotter/root
