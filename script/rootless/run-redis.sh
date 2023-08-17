#!/bin/bash
export CONTAINERD_SNAPSHOTTER=nix 
export CONTAINERD_ADDRESS=$XDG_RUNTIME_DIR/containerd/containerd.sock
nsenter -U --preserve-credentials -m -n -t $(cat $XDG_RUNTIME_DIR/rootlesskit-containerd/child_pid) nerdctl pull docker.io/library/redis:alpine
nsenter -U --preserve-credentials -m -n -t $(cat $XDG_RUNTIME_DIR/rootlesskit-containerd/child_pid) ctr run -rm --cgroup "" docker.io/library/redis:alpine redis