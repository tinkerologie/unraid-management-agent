#!/bin/bash
# Tests for meta/plugin/scripts/migrate-collection-mode (#280).
#
# Usage: bash scripts/test-collection-mode-migration.sh
# Must run as a non-root user so the unwritable-config case is meaningful.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MIGRATE="${REPO_ROOT}/meta/plugin/scripts/migrate-collection-mode"
DEFAULT_CFG="${REPO_ROOT}/meta/plugin/default.cfg"
PLG="${REPO_ROOT}/unraid-management-agent.plg"

# Prints an error and exits non-zero.
fail() {
    echo "✗ $*" >&2
    exit 1
}

[ "$(id -u)" -ne 0 ] || fail "Run as a non-root user (root ignores file permissions)"

work=$(mktemp -d)
trap 'chmod -R u+w "$work" 2>/dev/null; rm -rf "$work"' EXIT HUP INT TERM

# Creates an installed config directory for one case; $2 is the config.cfg
# content. Set FRESH=1 to leave out .installed, MIGRATED=1 to create the marker.
new_case() {
    dir="${work}/$1"
    mkdir -p "$dir"
    printf '%s' "$2" >"${dir}/config.cfg"
    chmod 0600 "${dir}/config.cfg"
    if [ "${FRESH:-0}" != 1 ]; then touch "${dir}/.installed"; fi
    if [ "${MIGRATED:-0}" = 1 ]; then touch "${dir}/.collection-mode-migrated"; fi
    echo "$dir"
}

# Runs the migration the way the .plg does: the marker is created only when
# the script exits 0. Returns the script's exit status.
migrate() {
    if CONFIG_DIR="$1" bash "$MIGRATE" >/dev/null 2>&1; then
        touch "$1/.collection-mode-migrated"
    else
        return 1
    fi
}

# Fails unless COLLECTION_MODE in the config of $1 is exactly $2 and appears once.
assert_mode() {
    local count line
    count=$(grep -c '^COLLECTION_MODE=' "$1/config.cfg" || true)
    [ "$count" -eq 1 ] || fail "$(basename "$1"): COLLECTION_MODE appears ${count} time(s)"
    line=$(grep '^COLLECTION_MODE=' "$1/config.cfg")
    [ "$line" = "COLLECTION_MODE=\"$2\"" ] || fail "$(basename "$1"): got '${line}', want 'COLLECTION_MODE=\"$2\"'"
}

# Fails unless the INTERVAL_* lines of $1 equal $2.
assert_intervals() {
    [ "$(grep '^INTERVAL_' "$1/config.cfg" || true)" = "$2" ] || fail "$(basename "$1"): INTERVAL_* values changed"
}

# The shipped interval lines of default.cfg and of the .plg first-install config.
default_cfg=$(grep '^INTERVAL_' "$DEFAULT_CFG")
plg_cfg=$(grep '^INTERVAL_' "$PLG")

for name in default-cfg plg; do
    if [ "$name" = plg ]; then intervals="$plg_cfg"; else intervals="$default_cfg"; fi
    dir=$(new_case "$name" "PORT=\"8043\"
${intervals}
")
    migrate "$dir" || fail "${name}: migration failed"
    assert_mode "$dir" dynamic
    assert_intervals "$dir" "$intervals"
    [ -e "$dir/.collection-mode-migrated" ] || fail "${name}: no marker after success"
done
echo "✓ All intervals on a shipped default (default.cfg or the .plg first-install) -> dynamic; intervals kept"

dir=$(new_case missing 'PORT="8043"
')
migrate "$dir" || fail "missing: migration failed"
assert_mode "$dir" dynamic
echo "✓ Missing interval keys count as defaults -> dynamic"

intervals='INTERVAL_SYSTEM="15"
INTERVAL_GPU="auto"
INTERVAL_UPS=auto
INTERVAL_NUT="0"
INTERVAL_ZFS="30"
INTERVAL_NIC="0"
INTERVAL_STORAGE_TOPOLOGY='"'300'"'
INTERVAL_ARRAY="30"
INTERVAL_HARDWARE="300"'
dir=$(new_case gated "PORT=\"8043\"
${intervals}
")
migrate "$dir" || fail "gated: migration failed"
assert_mode "$dir" dynamic
assert_intervals "$dir" "$intervals"
echo "✓ Gated auto and historical shipped defaults (quoted or not) -> dynamic"

for custom in 'INTERVAL_SYSTEM="5"' 'INTERVAL_DOCKER="0"' 'INTERVAL_GPU="300"' 'INTERVAL_MOVER="120"'; do
    intervals="${default_cfg}
${custom}"
    dir=$(new_case "custom-${custom%%=*}" "PORT=\"8043\"
${intervals}
")
    migrate "$dir" || fail "custom ${custom}: migration failed"
    assert_mode "$dir" manual
    assert_intervals "$dir" "$intervals"
done
echo "✓ One custom value, including Disabled (0), -> manual; intervals kept"

for mode in dynamic manual; do
    dir=$(new_case "existing-${mode}" "PORT=\"8043\"
INTERVAL_SYSTEM=\"5\"
COLLECTION_MODE=\"${mode}\"
")
    before=$(cat "${dir}/config.cfg")
    migrate "$dir" || fail "existing ${mode}: migration failed"
    [ "$(cat "${dir}/config.cfg")" = "$before" ] || fail "existing ${mode}: config.cfg changed"
done
echo "✓ An existing COLLECTION_MODE key is left unchanged"

dir=$(FRESH=1 new_case fresh 'PORT="8043"
INTERVAL_SYSTEM="5"
')
before=$(cat "${dir}/config.cfg")
migrate "$dir" || fail "fresh: migration failed"
[ "$(cat "${dir}/config.cfg")" = "$before" ] || fail "fresh: config.cfg changed"
[ -e "$dir/.collection-mode-migrated" ] || fail "fresh: no marker, so a later upgrade would migrate"
echo "✓ Fresh install (no .installed) is skipped and gets the marker"

dir=$(MIGRATED=1 new_case marker 'PORT="8043"
')
before=$(cat "${dir}/config.cfg")
migrate "$dir" || fail "marker: migration failed"
[ "$(cat "${dir}/config.cfg")" = "$before" ] || fail "marker: config.cfg changed"
echo "✓ Migration does nothing once the marker exists"

mkdir -p "${work}/no-config"
touch "${work}/no-config/.installed"
migrate "${work}/no-config" || fail "no-config: migration failed without a config.cfg"
[ ! -e "${work}/no-config/config.cfg" ] || fail "no-config: migration created config.cfg"
echo "✓ Without a config.cfg the migration does nothing (start defaults to manual)"

dir=$(new_case php 'PORT="8043"
')
migrate "$dir" || fail "php: migration failed"
if command -v php >/dev/null 2>&1; then
    # shellcheck disable=SC2016 # PHP code, not shell expansions
    php -r '$c = parse_ini_file($argv[1], false, INI_SCANNER_RAW); exit(($c["COLLECTION_MODE"] ?? "") === "dynamic" && ($c["PORT"] ?? "") === "8043" ? 0 : 1);' \
        "${dir}/config.cfg" 2>/dev/null || fail "migrated config.cfg is not readable by parse_ini_file"
    echo "✓ Migrated config.cfg parses with PHP parse_ini_file"
else
    echo "- php not installed; skipped the parse_ini_file check"
fi

dir=$(new_case readonly 'PORT="8043"
')
chmod 0400 "${dir}/config.cfg"
if migrate "$dir"; then
    fail "readonly: migration succeeded on an unwritable config"
fi
grep -q '^COLLECTION_MODE=' "${dir}/config.cfg" && fail "readonly: config.cfg changed"
[ ! -e "${dir}/.collection-mode-migrated" ] || fail "readonly: marker created after a failure"
echo "✓ Unwritable config fails non-zero and creates no marker"

dir=$(new_case unreadable 'PORT="8043"
')
chmod 0200 "${dir}/config.cfg"
if migrate "$dir"; then
    fail "unreadable: migration succeeded on an unreadable config"
fi
chmod 0600 "${dir}/config.cfg"
grep -q '^COLLECTION_MODE=' "${dir}/config.cfg" && fail "unreadable: config.cfg changed"
[ ! -e "${dir}/.collection-mode-migrated" ] || fail "unreadable: marker created after a failure"
echo "✓ Unreadable config fails non-zero and creates no marker"
