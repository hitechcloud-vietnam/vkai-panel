#!/usr/bin/env bash
# =============================================================================
# VKAI Panel - compatibility shim for the old path "scripts/setup.sh"
# HiTechCloud (hitechcloud.vn)
#
# The real script is setup-dev.sh at the root of the source tree. This file
# used to do part of the work itself (downloading dependencies) and then told
# the user to run docker-compose for PostgreSQL and Redis. The panel no longer
# uses Docker to run itself: setup-dev.sh installs PostgreSQL and Redis natively
# through the system package manager, exactly like deploy/install.sh does on a
# server.
#
#   bash scripts/setup.sh --yes
# is the same as
#   bash setup-dev.sh --yes
#
# To install on a real server: sudo bash deploy/install.sh
# =============================================================================

set -Eeuo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly REPO_ROOT
readonly SETUP_DEV="${REPO_ROOT}/setup-dev.sh"

if [[ ! -f "$SETUP_DEV" ]]; then
    echo "ERROR: ${SETUP_DEV} not found" >&2
    echo "Run this script from a complete VKAI Panel source tree." >&2
    exit 1
fi

echo "==> scripts/setup.sh is only a shortcut. Running: setup-dev.sh $*"
echo

exec bash "$SETUP_DEV" "$@"
