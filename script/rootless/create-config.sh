#!/bin/bash
CUR_DIR=$(pwd)
if [ ! -f ./script/rootless/config.toml ]
then
echo "
version = 2
root = \"$CUR_DIR/build/containerd/root\"
state = \"$CUR_DIR/build/containerd/state\"

[grpc]
    address = \"$XDG_RUNTIME_DIR/containerd/containerd.sock\"

# - Enable to use nix snapshotter
[plugins.\"io.containerd.grpc.v1.cri\".containerd]
    snapshotter = \"nix\"

# Use nix snapshotter
[proxy_plugins]
    [proxy_plugins.nix]
        type = \"snapshot\"
        address = \"$XDG_RUNTIME_DIR/containerd-nix/containerd-nix.sock\"
    " >> ./script/rootless/config.toml
fi