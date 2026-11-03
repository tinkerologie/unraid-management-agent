#!/usr/bin/env bash
# Inspect the shipped payload; smoke mode never starts the daemon.
set -euo pipefail
export LC_ALL=C
fail() { echo "Artifact check failed: $*" >&2; exit 1; }
[ "$#" -eq 2 ] || fail 'Usage: check-package-artifact.sh {check|smoke} bundle.tgz'
MODE=$1
case "$MODE" in check|smoke) ;; *) fail "Unknown mode: $MODE" ;; esac
BUNDLE=$2
READELF=${READELF:-readelf}
command -v go >/dev/null || fail 'Install Go and binutils (macOS: READELF=greadelf).'
command -v "$READELF" >/dev/null || fail 'Install binutils; on macOS use READELF=greadelf.'
SCRATCH=$(mktemp -d)
trap 'rm -rf "$SCRATCH"' EXIT
MEMBER=usr/local/emhttp/plugins/unraid-management-agent/unraid-management-agent
tar -tzf "$BUNDLE" > "$SCRATCH/members" 2>/dev/null || fail 'Cannot list the bundle.'
[ "$(awk -v member="$MEMBER" '$0 == member {n++} END {print n+0}' "$SCRATCH/members")" -eq 1 ] ||
  fail 'The bundle must contain exactly one daemon payload.'
LISTING=$(tar -tvzf "$BUNDLE" "$MEMBER" 2>/dev/null) || fail 'Cannot inspect the daemon member.'
[[ "$LISTING" == -* ]] || fail 'The daemon payload must be a regular file.'
tar -xOzf "$BUNDLE" "$MEMBER" > "$SCRATCH/binary" 2>/dev/null || fail 'Cannot extract the daemon payload.'
go version -m "$SCRATCH/binary" > "$SCRATCH/buildinfo" 2>/dev/null || fail 'Cannot read Go build metadata.'
for setting in GOOS=linux GOARCH=amd64 GOAMD64=v1 CGO_ENABLED=0; do
  key=${setting%%=*}
  awk -v key="$key" -v expected="$setting" '
    $1 == "build" && index($2, key "=") == 1 {count++; if ($2 == expected) good++}
    END {exit !(count == 1 && good == 1)}
  ' "$SCRATCH/buildinfo" || fail "Expected exactly one build setting: $setting"
done
"$READELF" -hW "$SCRATCH/binary" > "$SCRATCH/header" 2>/dev/null || fail 'Cannot read ELF header.'
grep -Eq 'Class:[[:space:]]+ELF64' "$SCRATCH/header" || fail 'Expected ELF64.'
grep -Eq 'Data:.*little endian' "$SCRATCH/header" || fail 'Expected little-endian ELF.'
grep -Eq 'Machine:.*X86-64' "$SCRATCH/header" || fail 'Expected an X86-64 ELF machine.'
"$READELF" -lW "$SCRATCH/binary" > "$SCRATCH/program" 2>/dev/null || fail 'Cannot read ELF program headers.'
grep -q 'Program Headers:' "$SCRATCH/program" || fail 'Missing ELF program headers.'
if grep -Eq '^[[:space:]]*INTERP[[:space:]]' "$SCRATCH/program"; then
  fail 'The payload has an ELF interpreter (dynamic linking).'
fi
"$READELF" -dW "$SCRATCH/binary" > "$SCRATCH/dynamic" 2>/dev/null || fail 'Cannot read ELF dynamic entries.'
grep -Eq 'There is no dynamic section|Dynamic section' "$SCRATCH/dynamic" || fail 'Unparseable ELF dynamic entries.'
if grep -q '(NEEDED)' "$SCRATCH/dynamic"; then
  fail 'The payload needs shared libraries.'
fi
if [ "$MODE" = smoke ]; then
  [ "$(uname -s)" = Linux ] || fail 'CPU smoke testing requires Linux and qemu-user.'
  QEMU_PATH=$(command -v "${QEMU:-qemu-x86_64}") || fail 'Install qemu-user for the CPU smoke test.'
  command -v timeout >/dev/null || fail 'CPU smoke testing requires timeout.'
  CPU=${QEMU_CPU:-Nehalem}
  chmod 700 "$SCRATCH/binary"
  rc=0
  (
    ulimit -c 0
    cd "$SCRATCH"
    timeout 15 env -i PATH=/usr/bin:/bin HOME="$SCRATCH" \
      "$QEMU_PATH" -cpu "$CPU" "$SCRATCH/binary" --help
  ) > "$SCRATCH/stdout" 2> "$SCRATCH/stderr" || rc=$?
  if [ "$rc" -ne 0 ]; then
    head -n 20 "$SCRATCH/stderr" >&2 || true
  fi
  case "$rc" in
    0) ;;
    132) fail "SIGILL: illegal instruction on CPU $CPU." ;;
    124) fail 'CPU smoke test timed out.' ;;
    *) fail "CPU smoke test exited with status $rc." ;;
  esac
  grep -q '^Usage:' "$SCRATCH/stdout" || fail 'CPU smoke test returned no CLI usage output.'
  echo "Verified packaged Linux/amd64 ELF64, CGO_ENABLED=0, GOAMD64=v1, static linking; $CPU --help passed."
else
  echo 'Verified packaged Linux/amd64 ELF64, CGO_ENABLED=0, GOAMD64=v1, static linking.'
fi
