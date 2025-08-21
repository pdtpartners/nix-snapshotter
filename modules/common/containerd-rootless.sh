#! @runtimeShell@
# shellcheck shell=bash

# Forked from https://github.com/containerd/nerdctl/blob/v1.5.0/extras/rootless/containerd-rootless.sh
# Copyright The containerd Authors.
# Copyright The Moby Authors.
# Licensed under the Apache License, Version 2.0
# NOTICE: https://github.com/containerd/nerdctl/blob/v1.5.0/NOTICE

set -e

export PATH="@path@"


if ! [ -w "$XDG_RUNTIME_DIR" ]; then
    echo "XDG_RUNTIME_DIR needs to be set and writable"
    exit 1
fi

# `selinuxenabled` always returns false in RootlessKit child, so we execute
# `selinuxenabled` in the parent.
# https://github.com/rootless-containers/rootlesskit/issues/94
if selinuxenabled; then
    _CONTAINERD_ROOTLESS_SELINUX=1
    export _CONTAINERD_ROOTLESS_SELINUX
fi

rootlesskit \
    --net=slirp4netns \
    --disable-host-loopback \
    --copy-up=/etc \
    --copy-up=/run \
    --copy-up=/var/lib \
    --port-driver=slirp4netns \
    --state-dir="${XDG_RUNTIME_DIR}/containerd-rootless" \
    sh -c "containerd-rootless-child.sh @containerdArgs@"
