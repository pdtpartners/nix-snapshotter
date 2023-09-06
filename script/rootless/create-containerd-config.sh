REPO_DIR=$(git rev-parse --show-toplevel)
CONFIG_FILE="${REPO_DIR}/build/containerd/config.toml"

if [ -f $CONFIG_FILE ]; then
    exit 0
fi

mkdir -p $(dirname $CONFIG_FILE)
cat <<EOM > $CONFIG_FILE
version = 2
root = "$REPO_DIR/build/containerd/root"
state = "$REPO_DIR/build/containerd/state"

[grpc]
address = "$REPO_DIR/build/containerd/containerd.sock"

[plugins."io.containerd.grpc.v1.cri".containerd]
snapshotter = "nix"

[proxy_plugins.nix]
type = "snapshot"
address = "$REPO_DIR/build/nix-snapshotter/nix-snapshotter.sock"
EOM
