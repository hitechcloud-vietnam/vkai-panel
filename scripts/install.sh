#!/usr/bin/env bash
# =============================================================================
# VKAI Panel - lop tuong thich cho duong dan cu "scripts/install.sh"
# HiTechCloud (hitechcloud.vn)
#
# Bo cai dat THAT nam o deploy/install.sh. Truoc day co hai ban cai song song
# (scripts/install.sh va deploy/install.sh) voi duong dan, ten dich vu va cach
# sinh bi mat khac nhau - chay nham ban se ra mot he thong khong khop voi tai
# lieu. Nay chi con MOT ban; file nay chuyen tiep moi tham so sang do.
#
#   bash scripts/install.sh --port 8888 --yes
# tuong duong
#   bash deploy/install.sh --port 8888 --yes
# =============================================================================

set -Eeuo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly REPO_ROOT
readonly INSTALLER="${REPO_ROOT}/deploy/install.sh"

if [[ ! -f "$INSTALLER" ]]; then
    echo "LOI: khong tim thay bo cai ${INSTALLER}" >&2
    echo "Hay chay script nay tu trong ma nguon VKAI Panel da giai nen day du." >&2
    exit 1
fi

echo "==> scripts/install.sh chi la loi tat. Dang chay: deploy/install.sh $*"
echo

exec bash "$INSTALLER" "$@"
