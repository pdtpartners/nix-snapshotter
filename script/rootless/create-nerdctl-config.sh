#!/bin/bash

source "${BASH_SOURCE%/*}/common.sh"

mkdir -p $(dirname $NERDCTL_TOML)
cat <<EOM > $NERDCTL_TOML
address         = "${REPO_DIR}/build/containerd/containerd.sock"
snapshotter     = "nix"
data_root       = "${REPO_DIR}/build/nerdctl/root/"
cni_netconfpath = "${REPO_DIR}/build/cni/net"
cni_path        = "${REPO_DIR}/build/cni/bin"
EOM
