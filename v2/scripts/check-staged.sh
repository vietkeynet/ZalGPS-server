#!/usr/bin/env bash
# Staged-only secret scan. Fail-closed: missing tool, scan error, or any
# finding -> exit 1. CI classifies findings; this script does not.
# Scanner output is suppressed, including errors and finding details.
set -euo pipefail

if ! command -v gitleaks >/dev/null 2>&1; then
    echo "gitleaks not found" >&2
    exit 1
fi

if gitleaks git --staged --redact=100 >/dev/null 2>&1; then
    echo "staged secret scan: PASS"
    exit 0
else
    echo "staged secret scan: FAIL"
    exit 1
fi
