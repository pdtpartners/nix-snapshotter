#!/bin/bash

source "${BASH_SOURCE%/*}/common.sh"

mkdir -p $(dirname $NIX_SNAPSHOTTER_CONFIG_FILE)
cat <<EOM > $NIX_SNAPSHOTTER_CONFIG_FILE
address = "${REPO_DIR}/build/nix-snapshotter/nix-snapshotter.sock"
root    = "${REPO_DIR}/build/containerd/root/io.containerd.snapshotter.v1.nix"

[image_service]
containerd_address = "${REPO_DIR}/build/containerd/containerd.sock"
EOM
