#!/usr/bin/env bash
# =============================================================================
# VKAI Panel - compatibility shim for the old path "scripts/install.sh"
# HiTechCloud (hitechcloud.vn)
#
# The real installer lives at deploy/install.sh. There used to be two parallel
# installers (scripts/install.sh and deploy/install.sh) with different paths,
# service names and secret generation - running the wrong one produced a system
# that did not match the documentation. There is only one now; this file
# forwards every argument to it.
#
#   bash scripts/install.sh --port 8888
# is the same as
#   bash deploy/install.sh --port 8888
# =============================================================================

set -Eeuo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly REPO_ROOT
readonly INSTALLER="${REPO_ROOT}/deploy/install.sh"

if [[ ! -f "$INSTALLER" ]]; then
    echo "ERROR: installer not found at ${INSTALLER}" >&2
    echo "Run this script from a complete, unpacked VKAI Panel source tree." >&2
    exit 1
fi

echo "==> scripts/install.sh is only a shortcut. Running: deploy/install.sh $*"
echo

exec bash "$INSTALLER" "$@"
