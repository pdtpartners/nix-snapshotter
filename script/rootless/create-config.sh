#!/bin/bash
REPO_DIR=$(git rev-parse --show-toplevel)
if [ ! -f ./script/rootless/config.toml ]
then
echo "
version = 2
root = \"$REPO_DIR/build/containerd/root\"
state = \"$REPO_DIR/build/containerd/state\"

[grpc]
    address = \"$REPO_DIR/build/containerd/state/containerd.sock\"

# - Enable to use nix snapshotter
[plugins.\"io.containerd.grpc.v1.cri\".containerd]
    snapshotter = \"nix\"

# Use nix snapshotter
[proxy_plugins]
    [proxy_plugins.nix]
        type = \"snapshot\"
        address = \"$REPO_DIR/build/nix-snapshotter/state/nix-snapshotter.sock\"
    " >> ./script/rootless/config.toml
fi