REPO_DIR=$(git rev-parse --show-toplevel)
CONFIG_FILE="${REPO_DIR}/build/nerdctl/nerdctl.toml"

if [ -f $CONFIG_FILE ]; then
    exit 0
fi

mkdir -p $(dirname $CONFIG_FILE)
cat <<EOM > $CONFIG_FILE
address         = "${REPO_DIR}/build/containerd/containerd.sock"
snapshotter     = "nix"
data_root       = "${REPO_DIR}/build/nerdctl/root/"
cni_netconfpath = "${REPO_DIR}/build/cni/net"
cni_path        = "${REPO_DIR}/build/cni/bin"
EOM
