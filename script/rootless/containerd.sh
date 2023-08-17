#!/bin/bash
REPO_DIR=$(git rev-parse --show-toplevel)
rootlesskit --net=slirp4netns --disable-host-loopback --copy-up=/etc --copy-up=/run --state-dir=$REPO_DIR/build/rootlesskit-containerd sh -c "rm -f /run/containerd; exec containerd --config ./script/rootless/config.toml" 
