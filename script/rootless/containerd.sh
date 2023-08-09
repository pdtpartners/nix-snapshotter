#!/bin/bash
UID=$(id -u) 

rootlesskit --net=slirp4netns --disable-host-loopback --copy-up=/etc --copy-up=/run --state-dir=/run/user/$UID/rootlesskit-containerd sh -c "rm -f /run/containerd; exec containerd --config ./script/rootless/config.toml" 
