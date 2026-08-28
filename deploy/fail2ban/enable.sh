#!/usr/bin/env bash
#
# Install the VKAI Panel fail2ban filter and jail.
#
# This is entirely optional. The panel's own limiter protects it whether or not
# fail2ban is installed; this only moves the enforcement down to the firewall.
#
# Usage: sudo ./enable.sh [--log-path PATH] [--port PORT]

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

FILTER_SRC="${SCRIPT_DIR}/filter.d/vkai-panel-auth.conf"
JAIL_SRC="${SCRIPT_DIR}/jail.d/vkai-panel.conf"
FILTER_DST="/etc/fail2ban/filter.d/vkai-panel-auth.conf"
JAIL_DST="/etc/fail2ban/jail.d/vkai-panel.conf"

LOG_PATH=""
PORT=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --log-path) LOG_PATH="${2:-}"; shift 2 ;;
    --port)     PORT="${2:-}";     shift 2 ;;
    -h|--help)  sed -n '2,10p' "$0"; exit 0 ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
done

if [[ "$(id -u)" != "0" ]]; then
  echo "This script must run as root." >&2
  exit 1
fi

if ! command -v fail2ban-client >/dev/null 2>&1; then
  echo "fail2ban is not installed. Install it first:" >&2
  echo "  apt install fail2ban    # Debian/Ubuntu" >&2
  echo "  dnf install fail2ban    # RHEL/Rocky/Alma" >&2
  echo "The panel is protected without it; this only adds firewall-level blocking." >&2
  exit 1
fi

install -d -m 0755 /etc/fail2ban/filter.d /etc/fail2ban/jail.d
install -m 0644 "${FILTER_SRC}" "${FILTER_DST}"
install -m 0644 "${JAIL_SRC}" "${JAIL_DST}"

if [[ -n "${LOG_PATH}" ]]; then
  sed -i "s#^logpath = .*#logpath = ${LOG_PATH}#" "${JAIL_DST}"
fi
if [[ -n "${PORT}" ]]; then
  sed -i "s#^port = .*#port = ${PORT}#" "${JAIL_DST}"
fi

CONFIGURED_LOG="$(awk -F'= *' '/^logpath/ {print $2; exit}' "${JAIL_DST}")"
if [[ ! -e "${CONFIGURED_LOG}" ]]; then
  echo "Warning: ${CONFIGURED_LOG} does not exist yet." >&2
  echo "It is created the first time somebody authenticates. The jail will pick it up." >&2
fi

# Verify the filter parses before reloading, so a bad edit does not take the
# whole fail2ban service down with it.
if ! fail2ban-regex "${CONFIGURED_LOG:-/dev/null}" "${FILTER_DST}" >/dev/null 2>&1; then
  echo "Warning: fail2ban-regex could not test the filter against ${CONFIGURED_LOG}." >&2
  echo "Run it by hand once the log exists:" >&2
  echo "  fail2ban-regex ${CONFIGURED_LOG} ${FILTER_DST}" >&2
fi

systemctl reload fail2ban 2>/dev/null || systemctl restart fail2ban

echo "Installed:"
echo "  ${FILTER_DST}"
echo "  ${JAIL_DST}"
echo
fail2ban-client status vkai-panel-auth || true
