#!/bin/bash
# Checks that the collector intervals added to the plugin (#276) use the
# daemon's defaults everywhere and reach the daemon: default.cfg, the
# first-install config in the .plg, the start script (fallback, validation
# and passthrough) and the settings page. Also checks the collection mode
# setting (#280) the same way. Run by `make test-packaging`.
#
# Usage: bash scripts/test-plugin-intervals.sh

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MAIN="${REPO_ROOT}/main.go"
DEFAULT_CFG="${REPO_ROOT}/meta/plugin/default.cfg"
PLG="${REPO_ROOT}/unraid-management-agent.plg"
START="${REPO_ROOT}/meta/plugin/scripts/start"
PAGE="${REPO_ROOT}/meta/plugin/unraid-management-agent.page"

INTERVALS=(
    INTERVAL_FAN_CONTROL
    INTERVAL_TUNING
    INTERVAL_DOCKER_UPDATE
    INTERVAL_DOCKER_NETWORKS
    INTERVAL_PLUGIN_UPDATE
    INTERVAL_OS_UPDATE
    INTERVAL_MOVER
)

# Prints an error and exits non-zero.
fail() {
    echo "✗ $*" >&2
    exit 1
}

# Prints the daemon default of env var $1 from its main.go struct tag.
daemon_default() {
    sed -n "s/.*default:\"\([0-9]*\)\" env:\"$1\".*/\1/p" "$MAIN"
}

# Prints the quoted value of $1="..." in file $2 (first match).
cfg_value() {
    sed -n "s/^$1=\"\([^\"]*\)\"$/\1/p" "$2" | head -n 1
}

# The block of variables validated by sanitize_int and the env passthrough.
sanitized=$(sed -n '/^for var in INTERVAL_SYSTEM/,/; do$/p' "$START")
passthrough=$(sed -n '/^nohup env/,/^$/p' "$START")

for var in "${INTERVALS[@]}"; do
    want=$(daemon_default "$var")
    [ -n "$want" ] || fail "$var: no default in main.go"

    got=$(cfg_value "$var" "$DEFAULT_CFG")
    [ "$got" = "$want" ] || fail "$var: default.cfg has '$got', daemon default is '$want'"

    got=$(cfg_value "$var" "$PLG")
    [ "$got" = "$want" ] || fail "$var: .plg first-install config has '$got', daemon default is '$want'"

    got=$(sed -n "s/^$var=\"\${$var:-\([^}]*\)}\"$/\1/p" "$START")
    [ "$got" = "$want" ] || fail "$var: start script fallback is '$got', daemon default is '$want'"

    grep -qw "$var" <<<"$sanitized" || fail "$var: not validated with sanitize_int in the start script"
    grep -q "^  $var=\"\$$var\" \\\\$" <<<"$passthrough" || fail "$var: not passed to the daemon by the start script"

    got=$(sed -n "s/.*\$config\['$var'\] ?? '\([^']*\)'.*/\1/p" "$PAGE")
    [ "$got" = "$want" ] || fail "$var: settings page fallback is '$got', daemon default is '$want'"
    grep -q "<select name=\"$var\"" "$PAGE" || fail "$var: no select on the settings page"

    echo "✓ $var defaults to $want everywhere and reaches the daemon"
done

# ── Collection mode (#280) ────────────────────────────────────
mode_field=$(grep 'env:"COLLECTION_MODE"' "$MAIN" || true)
[ -n "$mode_field" ] || fail "COLLECTION_MODE: main.go has no COLLECTION_MODE field"
grep -q 'default:"manual"' <<<"$mode_field" || fail "COLLECTION_MODE: main.go default is not manual"
grep -q 'enum:"dynamic,manual"' <<<"$mode_field" || fail "COLLECTION_MODE: main.go enum is not dynamic,manual"
echo "✓ main.go reads COLLECTION_MODE (dynamic|manual, default manual)"
[ "$(cfg_value COLLECTION_MODE "$DEFAULT_CFG")" = "dynamic" ] || fail "COLLECTION_MODE: default.cfg is not dynamic"
[ "$(cfg_value COLLECTION_MODE "$PLG")" = "dynamic" ] || fail "COLLECTION_MODE: .plg first-install config is not dynamic"
# shellcheck disable=SC2016 # literal start script lines, not expansions
grep -qx 'COLLECTION_MODE="${COLLECTION_MODE:-manual}"' "$START" || fail "COLLECTION_MODE: start script fallback is not manual"
# shellcheck disable=SC2016 # literal start script lines, not expansions
grep -q '^  COLLECTION_MODE="$COLLECTION_MODE" \\$' <<<"$passthrough" || fail "COLLECTION_MODE: not passed to the daemon by the start script"
echo "✓ COLLECTION_MODE is dynamic for new installs, falls back to manual in the start script and reaches the daemon"

# sanitize_int keeps digits only, so a bad value cannot inject shell code.
eval "$(grep '^sanitize_int()' "$START")"
[ "$(sanitize_int '60')" = "60" ] || fail "sanitize_int changed a valid value"
[ "$(sanitize_int '5; rm -rf /')" = "5" ] || fail "sanitize_int kept non-digits"
echo "✓ sanitize_int strips everything but digits"

# ── Run the start script against a fake daemon ────────────────
# The script is copied with its config, binary and plugin paths pointed into a
# temp directory; killall is stubbed and the fake daemon writes its environment.
SANDBOX=$(mktemp -d)
trap 'rm -rf "$SANDBOX"' EXIT
mkdir -p "$SANDBOX/scripts" "$SANDBOX/bin" "$SANDBOX/config"
sed -e "s|^CONFIG_FILE=.*|CONFIG_FILE=\"$SANDBOX/config/config.cfg\"|" \
    -e "s|^PROG=.*|PROG=\"$SANDBOX/bin/daemon\"|" \
    -e "s|^mkdir -p \"/boot/config/plugins/\$PLUGIN\"|mkdir -p \"$SANDBOX/config\"|" \
    "$START" >"$SANDBOX/scripts/start"
cp "${REPO_ROOT}/meta/plugin/scripts/auto-intervals" "$SANDBOX/scripts/"
grep -q "^CONFIG_FILE=\"$SANDBOX" "$SANDBOX/scripts/start" || fail "could not redirect CONFIG_FILE in the start script copy"
grep -q "^PROG=\"$SANDBOX" "$SANDBOX/scripts/start" || fail "could not redirect PROG in the start script copy"
printf '#!/bin/sh\nexit 0\n' >"$SANDBOX/bin/killall"
printf '#!/bin/sh\nenv >"%s/env.out"\n' "$SANDBOX" >"$SANDBOX/bin/daemon"
chmod +x "$SANDBOX/bin/killall" "$SANDBOX/bin/daemon"

# Runs the start script with config.cfg content $1 and prints the daemon's
# value of env var $2 for each remaining argument.
run_start() {
    local cfg="$1" i
    shift
    printf '%s\n' "$cfg" >"$SANDBOX/config/config.cfg"
    rm -f "$SANDBOX/env.out"
    # Clear the intervals so values exported by the caller cannot leak in.
    env -u COLLECTION_MODE $(printf -- '-u %s ' "${INTERVALS[@]}") PATH="$SANDBOX/bin:$PATH" \
        bash "$SANDBOX/scripts/start" >/dev/null
    for i in $(seq 1 50); do
        [ -s "$SANDBOX/env.out" ] && break
        sleep 0.1
    done
    [ -s "$SANDBOX/env.out" ] || fail "the start script did not launch the daemon"
    for var in "$@"; do
        sed -n "s/^$var=//p" "$SANDBOX/env.out"
    done
}

# Unset: every interval reaches the daemon with its default.
for var in "${INTERVALS[@]}"; do
    got=$(run_start 'PORT="8043"' "$var")
    [ "$got" = "$(daemon_default "$var")" ] || fail "$var unset: daemon got '$got', want '$(daemon_default "$var")'"
done
echo "✓ unset intervals reach the daemon with their defaults"

# Configured: custom values, including Disabled (0), reach the daemon unchanged.
cfg='PORT="8043"'
i=0
for var in "${INTERVALS[@]}"; do
    cfg+=$'\n'"$var=\"$((i * 7))\""
    i=$((i + 1))
done
i=0
for var in "${INTERVALS[@]}"; do
    got=$(run_start "$cfg" "$var")
    [ "$got" = "$((i * 7))" ] || fail "$var configured as $((i * 7)): daemon got '$got'"
    i=$((i + 1))
done
echo "✓ configured intervals, including 0 (Disabled), reach the daemon unchanged"

# Invalid: non-digits are stripped before the value reaches the daemon.
got=$(run_start $'PORT="8043"\nINTERVAL_FAN_CONTROL="15s"\nINTERVAL_MOVER="30;reboot"' INTERVAL_FAN_CONTROL INTERVAL_MOVER | paste -sd, -)
[ "$got" = "15,30" ] || fail "invalid intervals: daemon got '$got', want '15,30'"
echo "✓ non-digit characters are stripped from configured intervals"

# Collection mode: unset falls back to manual; anything but dynamic|manual is manual.
for pair in '<unset>:manual' 'dynamic:dynamic' 'manual:manual' 'Dynamic:manual' 'auto:manual' 'dynamic;reboot:manual' ':manual'; do
    value="${pair%:*}"
    want="${pair##*:}"
    cfg='PORT="8043"'
    [ "$value" = '<unset>' ] || cfg+=$'\n'"COLLECTION_MODE=\"$value\""
    got=$(run_start "$cfg" COLLECTION_MODE)
    [ "$got" = "$want" ] || fail "COLLECTION_MODE='$value': daemon got '$got', want '$want'"
done
echo "✓ COLLECTION_MODE reaches the daemon as dynamic or manual only; unset and invalid values become manual"
