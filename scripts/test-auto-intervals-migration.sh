#!/bin/bash
# Tests for meta/plugin/scripts/migrate-auto-intervals and the "auto" interval
# translation in meta/plugin/scripts/auto-intervals (#261).
#
# Usage: bash scripts/test-auto-intervals-migration.sh
# Must run as a non-root user so the unwritable-config case is meaningful.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MIGRATE="${REPO_ROOT}/meta/plugin/scripts/migrate-auto-intervals"
AUTO_LIB="${REPO_ROOT}/meta/plugin/scripts/auto-intervals"

# Prints an error and exits non-zero.
fail() {
    echo "✗ $*" >&2
    exit 1
}

[ "$(id -u)" -ne 0 ] || fail "Run as a non-root user (root ignores file permissions)"

work=$(mktemp -d)
trap 'chmod -R u+w "$work" 2>/dev/null; rm -rf "$work"' EXIT HUP INT TERM

# Creates a config directory for one case; $2 is the config.cfg content.
# Set MIGRATED=1 to create the marker.
new_case() {
    dir="${work}/$1"
    mkdir -p "$dir"
    printf '%s' "$2" >"${dir}/config.cfg"
    chmod 0600 "${dir}/config.cfg"
    if [ "${MIGRATED:-0}" = 1 ]; then touch "${dir}/.auto-intervals-migrated"; fi
    echo "$dir"
}

# Runs the migration against a config directory and returns its exit status.
migrate() {
    CONFIG_DIR="$1" bash "$MIGRATE" >/dev/null 2>&1
}

# Fails unless key $2 in the config of $1 is exactly $3 and appears once.
assert_value() {
    local count line
    count=$(grep -c "^$2=" "$1/config.cfg" || true)
    [ "$count" -eq 1 ] || fail "$(basename "$1"): $2 appears ${count} time(s)"
    line=$(grep "^$2=" "$1/config.cfg")
    [ "$line" = "$2=\"$3\"" ] || fail "$(basename "$1"): got '${line}', want '$2=\"$3\"'"
}

base='PORT="8043"
INTERVAL_SYSTEM="15"
'

dir=$(new_case missing "$base")
migrate "$dir" || fail "missing: migration failed"
for key in INTERVAL_NIC INTERVAL_STORAGE_TOPOLOGY INTERVAL_GPU INTERVAL_UPS INTERVAL_NUT INTERVAL_ZFS; do
    assert_value "$dir" "$key" auto
done
assert_value "$dir" INTERVAL_SYSTEM 15
[ "$(stat -c '%a' "${dir}/config.cfg" 2>/dev/null || stat -f '%Lp' "${dir}/config.cfg")" = "600" ] ||
    fail "missing: config.cfg mode changed"
echo "✓ Missing hardware keys become auto; other settings and the file mode are kept"

dir=$(new_case defaults "${base}INTERVAL_NIC=\"60\"
INTERVAL_STORAGE_TOPOLOGY=300
INTERVAL_GPU='60'
INTERVAL_UPS=\"60\"
INTERVAL_NUT=\"0\"
INTERVAL_ZFS=\"30\"
")
migrate "$dir" || fail "defaults: migration failed"
for key in INTERVAL_NIC INTERVAL_STORAGE_TOPOLOGY INTERVAL_GPU INTERVAL_UPS INTERVAL_NUT INTERVAL_ZFS; do
    assert_value "$dir" "$key" auto
done
echo "✓ Values equal to a previously shipped default (quoted or not) become auto"

dir=$(new_case zero-defaults "${base}INTERVAL_NIC=\"0\"
INTERVAL_STORAGE_TOPOLOGY=\"0\"
INTERVAL_ZFS=\"0\"
")
migrate "$dir" || fail "zero-defaults: migration failed"
assert_value "$dir" INTERVAL_NIC auto
assert_value "$dir" INTERVAL_STORAGE_TOPOLOGY auto
assert_value "$dir" INTERVAL_ZFS auto
echo "✓ The 0 default of NIC, storage topology and ZFS becomes auto"

dir=$(new_case explicit "${base}INTERVAL_NIC=\"120\"
INTERVAL_STORAGE_TOPOLOGY=\"600\"
INTERVAL_GPU=\"0\"
INTERVAL_UPS=\"30\"
INTERVAL_NUT=\"60\"
INTERVAL_ZFS=\"300\"
")
before=$(cat "${dir}/config.cfg")
migrate "$dir" || fail "explicit: migration failed"
[ "$(cat "${dir}/config.cfg")" = "$before" ] || fail "explicit: config.cfg changed"
echo "✓ Explicit non-default values, including GPU Disabled (0), are left unchanged"

dir=$(new_case already-auto "${base}INTERVAL_NIC=\"auto\"
INTERVAL_STORAGE_TOPOLOGY=\"auto\"
INTERVAL_GPU=\"auto\"
INTERVAL_UPS=\"auto\"
INTERVAL_NUT=\"auto\"
INTERVAL_ZFS=\"auto\"
")
before=$(cat "${dir}/config.cfg")
migrate "$dir" || fail "already-auto: migration failed"
[ "$(cat "${dir}/config.cfg")" = "$before" ] || fail "already-auto: config.cfg changed"
dir=$(new_case idempotent "$base")
migrate "$dir" || fail "idempotent: first run failed"
first=$(cat "${dir}/config.cfg")
migrate "$dir" || fail "idempotent: second run failed"
[ "$(cat "${dir}/config.cfg")" = "$first" ] || fail "idempotent: second run changed config.cfg"
echo "✓ Migration is idempotent"

if command -v php >/dev/null 2>&1; then
    # shellcheck disable=SC2016 # PHP code, not shell expansions
    php -r '$c = parse_ini_file($argv[1], false, INI_SCANNER_RAW); exit(($c["INTERVAL_NIC"] ?? "") === "auto" && ($c["PORT"] ?? "") === "8043" ? 0 : 1);' \
        "${dir}/config.cfg" 2>/dev/null || fail "migrated config.cfg is not readable by parse_ini_file"
    echo "✓ Migrated config.cfg parses with PHP parse_ini_file"
else
    echo "- php not installed; skipped the parse_ini_file check"
fi

dir=$(MIGRATED=1 new_case marker "$base")
before=$(cat "${dir}/config.cfg")
migrate "$dir" || fail "marker: migration failed"
[ "$(cat "${dir}/config.cfg")" = "$before" ] || fail "marker: config.cfg changed"
echo "✓ Migration does nothing once the marker exists"

mkdir -p "${work}/fresh"
migrate "${work}/fresh" || fail "fresh: migration failed without a config.cfg"
[ ! -e "${work}/fresh/config.cfg" ] || fail "fresh: migration created config.cfg"
echo "✓ Without a config.cfg the migration does nothing (start defaults to auto)"

dir=$(new_case readonly "$base")
chmod 0400 "${dir}/config.cfg"
if migrate "$dir"; then
    fail "readonly: migration succeeded on an unwritable config"
fi
grep -q '^INTERVAL_NIC=' "${dir}/config.cfg" && fail "readonly: config.cfg changed"
echo "✓ Unwritable config fails non-zero"

if [ "$(id -u)" -ne 0 ]; then
    dir=$(new_case unreadable "$base")
    before=$(cat "${dir}/config.cfg")
    chmod 0200 "${dir}/config.cfg"
    if migrate "$dir"; then
        fail "unreadable: migration succeeded on an unreadable config"
    fi
    chmod 0600 "${dir}/config.cfg"
    [ "$(cat "${dir}/config.cfg")" = "$before" ] || fail "unreadable: config.cfg changed"
    [ ! -e "${dir}/.auto-intervals-migrated" ] || fail "unreadable: marker created after a failure"
    echo "✓ Unreadable config fails non-zero without changing it"
fi

# Start-script translation: auto -> 0 plus a CSV entry; numbers unchanged.
# shellcheck disable=SC2016 # expansions happen in the child shell
out=$(bash -c '
    set -euo pipefail
    source "$1"
    INTERVAL_NIC=auto INTERVAL_STORAGE_TOPOLOGY=0 INTERVAL_GPU=auto INTERVAL_UPS=60 INTERVAL_NUT=auto INTERVAL_ZFS=300
    resolve_auto_intervals
    echo "$INTERVAL_NIC $INTERVAL_STORAGE_TOPOLOGY $INTERVAL_GPU $INTERVAL_UPS $INTERVAL_NUT $INTERVAL_ZFS $UNRAID_AUTO_COLLECTORS"
' -- "$AUTO_LIB")
[ "$out" = "0 0 0 60 0 300 nic,gpu,nut" ] || fail "translation: got '${out}'"
# shellcheck disable=SC2016 # expansions happen in the child shell
out=$(bash -c '
    set -euo pipefail
    source "$1"
    INTERVAL_NIC=60 INTERVAL_STORAGE_TOPOLOGY=300 INTERVAL_GPU=60 INTERVAL_UPS=60 INTERVAL_NUT=0 INTERVAL_ZFS=0
    resolve_auto_intervals
    echo "[$UNRAID_AUTO_COLLECTORS] $INTERVAL_NIC $INTERVAL_STORAGE_TOPOLOGY"
' -- "$AUTO_LIB")
[ "$out" = "[] 60 300" ] || fail "translation without auto: got '${out}'"
echo "✓ Start translation turns auto into 0 plus UNRAID_AUTO_COLLECTORS and keeps numeric values"

# PSU automatic mode is optional for older configs and is preserved when explicit.
# shellcheck disable=SC2016
out=$(bash -c '
    set -euo pipefail
    source "$1"
    INTERVAL_PSU=auto
    resolve_auto_intervals
    echo "$INTERVAL_PSU $UNRAID_AUTO_COLLECTORS"
    INTERVAL_PSU=0
    resolve_auto_intervals
    echo "$INTERVAL_PSU [$UNRAID_AUTO_COLLECTORS]"
' -- "$AUTO_LIB")
[ "$out" = $'0 psu\n0 []' ] || fail "PSU translation: got '${out}'"
echo "✓ PSU automatic detection translates correctly and explicit Disabled stays off"
