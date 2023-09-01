#!/bin/bash

source "${BASH_SOURCE%/*}/common.sh"

./build/bin/nix-snapshotter \
  --log-level debug \
  --config "${NIX_SNAPSHOTTER_CONFIG_FILE}"
