#!/bin/bash
# Tests for meta/plugin/scripts/migrate-secure-defaults (#245).
#
# Usage: bash scripts/test-secure-defaults-migration.sh
# Must run as a non-root user so the unwritable-config case is meaningful.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MIGRATE="${REPO_ROOT}/meta/plugin/scripts/migrate-secure-defaults"
KEY='ALLOW_UNAUTHENTICATED_WRITES="true"'

# Prints an error and exits non-zero.
fail() {
    echo "✗ $*" >&2
    exit 1
}

[ "$(id -u)" -ne 0 ] || fail "Run as a non-root user (root ignores file permissions)"

work=$(mktemp -d)
trap 'chmod -R u+w "$work" 2>/dev/null; rm -rf "$work"' EXIT HUP INT TERM

# Creates a config directory for one case; $2 is the config.cfg content.
# Set INSTALLED=1 or MIGRATED=1 to create the matching marker.
new_case() {
    dir="${work}/$1"
    mkdir -p "$dir"
    printf '%s' "$2" >"${dir}/config.cfg"
    chmod 0600 "${dir}/config.cfg"
    if [ "${INSTALLED:-0}" = 1 ]; then touch "${dir}/.installed"; fi
    if [ "${MIGRATED:-0}" = 1 ]; then touch "${dir}/.secure-defaults-migrated"; fi
    echo "$dir"
}

# Runs the migration against a config directory and returns its exit status.
migrate() {
    CONFIG_DIR="$1" bash "$MIGRATE" >/dev/null 2>&1
}

# Fails unless the config in $1 contains the opt-out exactly $2 times.
assert_key_count() {
    got=$(grep -cxF "$KEY" "$1/config.cfg" || true)
    [ "$got" -eq "$2" ] || fail "$(basename "$1"): expected opt-out ${2} time(s), found ${got}"
}

base='PORT="8043"
READ_ONLY="false"
'

dir=$(new_case fresh "${base}API_TOKEN=\"\"
")
migrate "$dir" || fail "fresh: migration failed"
assert_key_count "$dir" 0
echo "✓ Fresh install (no .installed) gets no opt-out"

dir=$(INSTALLED=1 new_case empty-token "${base}API_TOKEN=\"\"
")
migrate "$dir" || fail "empty-token: migration failed"
assert_key_count "$dir" 1
grep -qxF 'READ_ONLY="false"' "${dir}/config.cfg" || fail "empty-token: other settings were not preserved"
[ "$(stat -c '%a' "${dir}/config.cfg" 2>/dev/null || stat -f '%Lp' "${dir}/config.cfg")" = "600" ] ||
    fail "empty-token: config.cfg mode changed"
migrate "$dir" || fail "empty-token: second run failed"
assert_key_count "$dir" 1
echo "✓ Upgrade without a token keeps write access, preserves settings and mode, and is idempotent"

# The settings page and update.php read config.cfg with PHP's parse_ini_file,
# which rejects "#" comments containing "(" or quotes; the migration must keep
# the file parseable.
if command -v php >/dev/null 2>&1; then
    # shellcheck disable=SC2016 # PHP code, not shell expansions
    php -r 'exit(parse_ini_file($argv[1], false, INI_SCANNER_RAW)["ALLOW_UNAUTHENTICATED_WRITES"] === "true" ? 0 : 1);' \
        "${dir}/config.cfg" 2>/dev/null || fail "empty-token: migrated config.cfg is not readable by parse_ini_file"
    echo "✓ Migrated config.cfg parses with PHP parse_ini_file"
else
    echo "- php not installed; skipped the parse_ini_file check"
fi

dir=$(INSTALLED=1 new_case missing-token "$base")
migrate "$dir" || fail "missing-token: migration failed"
assert_key_count "$dir" 1
echo "✓ Upgrade with no API_TOKEN line keeps write access"

i=0
for quoted in "'my-token'" '"my-token"' 'my-token'; do
    i=$((i + 1))
    dir=$(INSTALLED=1 new_case "token-${i}" "${base}API_TOKEN=${quoted}
")
    migrate "$dir" || fail "token ${quoted}: migration failed"
    assert_key_count "$dir" 0
done
echo "✓ Upgrade with a token set gets no opt-out"

dir=$(INSTALLED=1 new_case explicit "${base}ALLOW_UNAUTHENTICATED_WRITES=\"false\"
")
before=$(cat "${dir}/config.cfg")
migrate "$dir" || fail "explicit: migration failed"
[ "$(cat "${dir}/config.cfg")" = "$before" ] || fail "explicit: config.cfg changed"
echo "✓ An existing ALLOW_UNAUTHENTICATED_WRITES setting is left unchanged"

dir=$(INSTALLED=1 MIGRATED=1 new_case marker "${base}API_TOKEN=\"\"
")
migrate "$dir" || fail "marker: migration failed"
assert_key_count "$dir" 0
echo "✓ Migration does nothing once the marker exists"

dir=$(INSTALLED=1 new_case readonly "${base}API_TOKEN=\"\"
")
chmod 0400 "${dir}/config.cfg"
if migrate "$dir"; then
    fail "readonly: migration succeeded on an unwritable config"
fi
assert_key_count "$dir" 0
echo "✓ Unwritable config fails non-zero and stays on the secure default"
