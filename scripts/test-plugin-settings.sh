#!/bin/bash
# Tests the settings-page save hook (meta/plugin/include/update.php) for the
# security, privacy and display toggles (#245, #252, #261). Run by `make test-packaging`.
#
# Usage: bash scripts/test-plugin-settings.sh

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
HOOK="${REPO_ROOT}/meta/plugin/include/update.php"

# Prints an error and exits non-zero.
fail() {
    echo "✗ $*" >&2
    exit 1
}

if ! command -v php >/dev/null 2>&1; then
    echo "- php not installed; skipped the settings save-hook tests"
    exit 0
fi

# Prints the saved value of key $1 after the hook runs on a form post with
# $1=$2 and its checkbox field, or "<unset>" if the key is not posted.
# shellcheck disable=SC2016 # PHP code, not shell expansions
saved() {
    php -r '
        $_POST = [];
        if ($argv[2] !== "<absent>") { $_POST[$argv[1]] = $argv[2]; }
        $_POST[$argv[1] . "_CB"] = "on";
        $plugin = "unraid-management-agent";
        include $argv[3];
        if (isset($_POST[$argv[1] . "_CB"])) { fwrite(STDERR, "checkbox field was saved\n"); exit(1); }
        echo $_POST[$argv[1]] ?? "<unset>";
    ' -- "$1" "$2" "$HOOK"
}

check() {
    local got
    got=$(saved "$1" "$2") || fail "$1=$2: hook failed"
    [ "$got" = "$3" ] || fail "$1='$2' saved as '$got', want '$3'"
}

# WAN IP lookup: only an explicit "false" turns it off (default on).
check WAN_IP_LOOKUP true true
check WAN_IP_LOOKUP false false
check WAN_IP_LOOKUP yes true
check WAN_IP_LOOKUP "" true
check WAN_IP_LOOKUP "<absent>" "<unset>"
echo "✓ WAN_IP_LOOKUP saves as true unless explicitly false; its checkbox field is dropped"

# Unauthenticated writes: only an explicit "true" turns them on (default off).
check ALLOW_UNAUTHENTICATED_WRITES true true
check ALLOW_UNAUTHENTICATED_WRITES false false
check ALLOW_UNAUTHENTICATED_WRITES yes false
check ALLOW_UNAUTHENTICATED_WRITES "" false
check ALLOW_UNAUTHENTICATED_WRITES "<absent>" "<unset>"
echo "✓ ALLOW_UNAUTHENTICATED_WRITES saves as false unless explicitly true; its checkbox field is dropped"

# Show all collectors: only an explicit "true" turns it on (default off).
check SHOW_ALL_COLLECTORS true true
check SHOW_ALL_COLLECTORS false false
check SHOW_ALL_COLLECTORS yes false
check SHOW_ALL_COLLECTORS "" false
check SHOW_ALL_COLLECTORS "<absent>" "<unset>"
echo "✓ SHOW_ALL_COLLECTORS saves as false unless explicitly true; its checkbox field is dropped"
