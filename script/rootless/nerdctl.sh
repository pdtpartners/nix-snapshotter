#!/bin/bash

# shellcheck disable=SC1091
source "${BASH_SOURCE%/*}/common.sh"

nerdctl "$@"
