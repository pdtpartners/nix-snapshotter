#!/bin/bash
REPO_DIR=$(git rev-parse --show-toplevel)
./out/nix-snapshotter $REPO_DIR/build/nix-snapshotter/state/nix-snapshotter.sock $REPO_DIR/build/containerd/root