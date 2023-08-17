#!/bin/bash
rootlesskit --net=slirp4netns --disable-host-loopback --copy-up=/etc --copy-up=/run --state-dir=$XDG_RUNTIME_DIR/rootlesskit-containerd sh -c "rm -f /run/containerd; exec containerd --config ./script/rootless/config.toml" 
