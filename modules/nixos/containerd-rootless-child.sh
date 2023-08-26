#! @runtimeShell@
# shellcheck shell=bash

# Forked from https://github.com/containerd/nerdctl/blob/v1.5.0/extras/rootless/containerd-rootless.sh
# Copyright The containerd Authors.
# Copyright The Moby Authors.
# Licensed under the Apache License, Version 2.0
# NOTICE: https://github.com/containerd/nerdctl/blob/v1.5.0/NOTICE

set -e

export PATH="@path@"


if ! [ -w $HOME ]; then
    echo "HOME needs to be set and writable"
    exit 1
fi
: "${XDG_DATA_HOME:=$HOME/.local/share}"
: "${XDG_CONFIG_HOME:=$HOME/.config}"

# Avoid sharing host iptables lock file.
rm -f /run/xtables.lock

# Bind-mount /etc/ssl.
# Workaround for "x509: certificate signed by unknown authority" on openSUSE
# Tumbleweed.
# https://github.com/rootless-containers/rootlesskit/issues/225
realpath_etc_ssl=$(realpath /etc/ssl)
rm -f /etc/ssl
mkdir /etc/ssl
mount --rbind "$realpath_etc_ssl" /etc/ssl

mountSources=(@mountSources@)
mountPoints=(@mountPoints@)

for i in "${!mountSources[@]}"; do
  mountSource=${mountSources[$i]}
  mountPoint=${mountPoints[$i]}

  # Remove the *symlinks* for the existing files in the parent namespace if any,
  # so that we can create our own files in our mount namespace.
  # The actual files in the parent namespace are *not removed* by this rm command.
  rm -f "$mountPoint"

  echo >&2 Bind mounting ${mountSource} to ${mountPoint} inside mount namespace
  mkdir -p "$mountSource" "$mountPoint"
  mount --bind "$mountSource" "$mountPoint"
done

if [ -n "$_CONTAINERD_ROOTLESS_SELINUX" ]; then
    # iptables requires /run in the child to be relabeled. The actual /run in the
    # parent is unaffected.
    # https://github.com/containers/podman/blob/e6fc34b71aa9d876b1218efe90e14f8b912b0603/libpod/networking_linux.go#L396-L401
    # https://github.com/moby/moby/issues/41230
    chcon system_u:object_r:iptables_var_run_t:s0 /run
fi

exec "containerd" "$@"
