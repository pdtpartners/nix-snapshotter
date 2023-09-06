REPO_DIR=$(git rev-parse --show-toplevel)
CONFIG_FILE="${REPO_DIR}/build/containerd/config.toml"

rootlesskit \
    --net=slirp4netns \
    --disable-host-loopback \
    --copy-up=/etc \
    --copy-up=/run \
    --copy-up=/var/lib \
    --port-driver=slirp4netns \
    --state-dir=$REPO_DIR/build/rootlesskit-containerd \
    sh -c "containerd --config ${CONFIG_FILE}"
