#!/bin/bash
# Execute the shipped preflight against isolated Unraid version fixtures.
set -euo pipefail

repo="$(cd "$(dirname "$0")/.." && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
manifest="$repo/unraid-management-agent.plg"

fail() {
    printf '✗ %s\n' "$*" >&2
    exit 1
}

awk -v version_file="$work/version" '
    /<!\[CDATA\[/ { capture = 1; script = "" }
    /\]\]>/ && capture {
        if (script ~ /\/etc\/unraid-version/) {
            blocks++
            replacements += gsub(/\/etc\/unraid-version/, version_file, script)
            printf "%s", script
        }
        capture = 0
        next
    }
    capture && !/<!\[CDATA\[/ { script = script $0 "\n" }
    END { if (blocks != 1 || replacements != 2) exit 1 }
' "$manifest" > "$work/preflight.sh" || fail "expected one version-check block with two version file references"
cd "$work"

expect() {
    local fixture=$1 code=$2 message=$3 output actual
    printf '%s\n' "$fixture" > "$work/version"
    actual=0
    output=$(env -i PATH=/usr/bin:/bin bash "$work/preflight.sh" 2>&1) || actual=$?
    [ "$actual" -eq "$code" ] || fail "$fixture returned $actual, expected $code"
    case "$output" in
        *"$message"*) ;;
        *) fail "$fixture did not report $message" ;;
    esac
}

expect 'version="7.1.9"' 1 'requires Unraid 7.2.0 or higher'
expect 'version="6.9.0"' 1 'requires Unraid 7.2.0 or higher'
expect 'version="7.2.0"' 0 'version check passed'
expect 'version="7.4.0-rc.1"' 0 'version check passed'
expect 'version="7.4.0"' 0 'version check passed'
expect 'version="8.0.0"' 0 'version check passed'
expect 'version="7.x.0"' 1 'requires Unraid 7.2.0 or higher'
expect 'version="7.2.99999999999999999999"' 0 'version check passed'
expect 'version="$(touch should-not-execute)"' 1 'requires Unraid 7.2.0 or higher'
test ! -e should-not-execute || fail 'the preflight executed text from the version file'
rm "$work/version"
missing=$(env -i PATH=/usr/bin:/bin bash "$work/preflight.sh" 2>&1)
case "$missing" in
    *"Could not verify Unraid version"*) ;;
    *) fail 'missing version did not warn' ;;
esac

manifest_min=$(sed -nE 's/^<PLUGIN .* min="([^"]+)".*/\1/p' "$manifest")
listing_min=$(sed -nE 's/.*<MinVer>([^<]+)<\/MinVer>.*/\1/p' "$repo/unraid-management-agent.xml")
preflight_min=$(sed -nE 's/.*requires Unraid ([0-9.]+) or higher.*/\1/p' "$work/preflight.sh")
[ -n "$manifest_min" ] && [ "$manifest_min" = "$listing_min" ] && [ "$manifest_min" = "$preflight_min" ] ||
    fail "minimum versions differ (manifest $manifest_min, listing $listing_min, preflight $preflight_min)"
printf '✓ Installer accepts Unraid %s and later, rejects older and malformed versions, and matches both manifests\n' "$preflight_min"
