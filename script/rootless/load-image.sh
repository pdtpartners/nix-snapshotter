#!/bin/bash

image=$1

source "${BASH_SOURCE%/*}/common.sh"

outPath=$(nix build --print-out-paths .#archive-${image})
ctr image import --local=false ${outPath}
