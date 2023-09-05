#!/bin/bash

image=$1

source "${BASH_SOURCE%/*}/common.sh"

outPath=$(nix build --print-out-paths .#image-${image})
${BIN_DIR}/nix2container --address $CONTAINERD_ADDRESS load ${outPath}
