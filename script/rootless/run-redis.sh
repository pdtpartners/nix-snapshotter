#!/bin/bash
UID=$(id -u) 

export CONTAINERD_SNAPSHOTTER=nix 
export CONTAINERD_ADDRESS=/run/user/$UID/containerd/containerd.sock
nsenter -U --preserve-credentials -m -n -t $(cat /run/user/$UID/rootlesskit-containerd/child_pid) nerdctl pull docker.io/library/redis:alpine
nsenter -U --preserve-credentials -m -n -t $(cat /run/user/$UID/rootlesskit-containerd/child_pid) ctr run -rm --cgroup "" docker.io/library/redis:alpine redis