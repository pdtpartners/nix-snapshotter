#!/bin/bash
UID=$(id -u) 

if [ ! -f ./script/rootless/config.toml ]
then
echo "
version = 2
root = \"${HOME}/.local/share/containerd\"
state = \"/run/user/${UID}/containerd\"

[grpc]
address = \"/run/user/1001/containerd/containerd.sock\"

# - Set default runtime handler to v2, which has a per-pod shim
# - Enable to use nix snapshotter
[plugins.\"io.containerd.grpc.v1.cri\".containerd]
default_runtime_name = \"runc\"
snapshotter = \"nix\"
disable_snapshot_annotations = false
[plugins.\"io.containerd.grpc.v1.cri\".containerd.runtimes.runc]
runtime_type = \"io.containerd.runc.v2\"

# Setup a runtime with the magic name (\"test-handler\") used for Kubernetes
# runtime class tests ...
[plugins.\"io.containerd.grpc.v1.cri\".containerd.runtimes.test-handler]
runtime_type = \"io.containerd.runc.v2\"

# Use nix snapshotter
[proxy_plugins]
[proxy_plugins.nix]
    type = \"snapshot\"
    address = \"/run/user/${UID}/containerd-nix/containerd-nix.sock\"
" >> ./script/rootless/config.toml
fi