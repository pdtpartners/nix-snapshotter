#!/bin/bash

source "${BASH_SOURCE%/*}/common.sh"

rootlesskit \
    --net=slirp4netns \
    --disable-host-loopback \
    --copy-up=/etc \
    --copy-up=/run \
    --copy-up=/var/lib \
    --port-driver=slirp4netns \
    --state-dir=$REPO_DIR/build/rootlesskit-containerd \
    sh -c "containerd --config ${CONTAINERD_CONFIG_FILE}"
