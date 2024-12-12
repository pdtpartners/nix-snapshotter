#!/bin/bash

# shellcheck disable=SC1091
source "${BASH_SOURCE%/*}/common.sh"

mkdir -p "$(dirname "$CONTAINERD_CONFIG_FILE")"
cat <<EOM > "$CONTAINERD_CONFIG_FILE"
version = 2
root = "$REPO_DIR/build/containerd/root"
state = "$REPO_DIR/build/containerd/state"

[grpc]
address = "$REPO_DIR/build/containerd/containerd.sock"

[plugins."io.containerd.grpc.v1.cri".containerd]
snapshotter = "nix"

[[plugins."io.containerd.transfer.v1.local".unpack_config]]
platform = "linux/amd64"
snapshotter = "nix"

[proxy_plugins.nix]
type = "snapshot"
address = "$REPO_DIR/build/nix-snapshotter/nix-snapshotter.sock"
EOM
