REPO_DIR=$(git rev-parse --show-toplevel)
CONFIG_FILE="${REPO_DIR}/build/nix-snapshotter/nix-snapshotter.toml"

if [ -f $CONFIG_FILE ]; then
    exit 0
fi

mkdir -p $(dirname $CONFIG_FILE)
cat <<EOM > $CONFIG_FILE
address = "${REPO_DIR}/build/nix-snapshotter/nix-snapshotter.sock"
root    = "${REPO_DIR}/build/nix-snapshotter/root"
EOM
