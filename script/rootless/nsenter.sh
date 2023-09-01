#!/bin/bash

source "${BASH_SOURCE%/*}/common.sh"

pid=$(cat "$REPO_DIR/build/rootlesskit-containerd/child_pid")
nsenter -U --preserve-credentials -m -n -t "$pid" $@
