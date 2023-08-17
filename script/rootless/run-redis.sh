#!/bin/bash
REPO_DIR=$(git rev-parse --show-toplevel)
export CONTAINERD_SNAPSHOTTER=nix 
export CONTAINERD_ADDRESS=$REPO_DIR/build/containerd/state/containerd.sock
nsenter -U --preserve-credentials -m -n -t $(cat $REPO_DIR/build/rootlesskit-containerd/child_pid) nerdctl pull docker.io/library/redis:alpine
nsenter -U --preserve-credentials -m -n -t $(cat $REPO_DIR/build/rootlesskit-containerd/child_pid) ctr run -rm --cgroup "" docker.io/library/redis:alpine redis