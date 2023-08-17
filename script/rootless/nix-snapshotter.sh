#!/bin/bash
CUR_DIR=$(pwd)
mkdir -p $HOME/.local/share/containerd/
mkdir -p $XDG_RUNTIME_DIR/containerd-nix/
./out/nix-snapshotter $XDG_RUNTIME_DIR/containerd-nix/containerd-nix.sock $CUR_DIR/build/containerd/root