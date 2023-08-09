#!/bin/bash
UID=$(id -u) 

mkdir -p $HOME/.local/share/containerd/
mkdir -p /run/user/$UID/containerd-nix/
./out/nix-snapshotter /run/user/$UID/containerd-nix/containerd-nix.sock ~/.local/share/containerd