#!/bin/bash
set -euo pipefail

REPO_DIR="$(git rev-parse --show-toplevel)"
export REPO_DIR

export BIN_DIR="${REPO_DIR}/build/bin"
export CONTAINERD_ADDRESS="${REPO_DIR}/build/containerd/containerd.sock"
export CONTAINERD_CONFIG_FILE="${REPO_DIR}/build/containerd/config.toml"
export CONTAINERD_SNAPSHOTTER="nix"
export NERDCTL_TOML="${REPO_DIR}/build/nerdctl/nerdctl.toml"
export NIX_SNAPSHOTTER_CONFIG_FILE="${REPO_DIR}/build/nix-snapshotter/nix-snapshotter.toml"
export ROOTLESSKIT_STATE_DIR=$REPO_DIR/build/rootlesskit-containerd/
