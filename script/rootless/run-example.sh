#!/bin/bash
UID=$(id -u) 

export CONTAINERD_SNAPSHOTTER=nix 
export CONTAINERD_ADDRESS=/run/user/$UID/containerd/containerd.sock
nsenter -U --preserve-credentials -m -n -t $(cat /run/user/$UID/rootlesskit-containerd/child_pid) nerdctl -n k8s.io pull docker.io/hinshun/hello:nix 
nsenter -U --preserve-credentials -m -n -t $(cat /run/user/$UID/rootlesskit-containerd/child_pid) ctr -n k8s.io run -t --rm --cgroup "" docker.io/hinshun/hello:nix example 