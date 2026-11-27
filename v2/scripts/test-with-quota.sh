#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Fail closed before starting the workload unless the actual scope is bounded.
set -euo pipefail
root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)
test "$(id -u)" -ne 0 || { echo "Use an unprivileged user." >&2; exit 1; }
test -r /sys/fs/cgroup/cgroup.controllers || { echo "cgroup v2 required." >&2; exit 1; }
command -v systemd-run >/dev/null || { echo "systemd-run required." >&2; exit 1; }
exec systemd-run --user --scope --expand-environment=no --property=MemoryMax=4G --property=CPUQuota=200% -- /bin/bash -eu -c '
    cg=$(awk -F: '\''$1 == 0 { print $3 }'\'' /proc/self/cgroup)
    case "$cg" in /*) ;; *) exit 1 ;; esac
    d="/sys/fs/cgroup$cg"
    test -r "$d/memory.max" && test -r "$d/cpu.max"
    mem=$(cat "$d/memory.max")
    read -r quota period < "$d/cpu.max"
    [[ $mem =~ ^[0-9]+$ && $quota =~ ^[0-9]+$ && $period =~ ^[0-9]+$ ]]
    (( mem > 0 && mem <= 4294967296 && period > 0 && quota > 0 && quota <= 2 * period ))
    exec /bin/bash "$1"
' bash "$root/scripts/test-isolated.sh"
