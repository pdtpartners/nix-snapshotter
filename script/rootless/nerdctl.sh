REPO_DIR=$(git rev-parse --show-toplevel)

export NERDCTL_TOML=$REPO_DIR/build/nerdctl/nerdctl.toml

# This fixes a "Your aren't rootless error"
export ROOTLESSKIT_STATE_DIR=$REPO_DIR/build/rootlesskit-containerd/

pid=$(cat "$REPO_DIR/build/rootlesskit-containerd/child_pid")
nsenter -U --preserve-credentials -m -n -t "$pid" nerdctl $@
