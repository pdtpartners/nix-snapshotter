#!/bin/bash
REPO_DIR=$(git rev-parse --show-toplevel)
export NERDCTL_TOML=$REPO_DIR/script/rootless/nerdctl.toml
export ROOTLESSKIT_STATE_DIR=$REPO_DIR/build/rootlesskit-containerd/ #This fixes a "Your aren't rootless error"
nsenter -U --preserve-credentials -m -n -t $(cat $REPO_DIR/build/rootlesskit-containerd/child_pid) nerdctl run --rm docker.io/hinshun/hello:nix 