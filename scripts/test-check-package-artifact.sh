#!/usr/bin/env bash
set -euo pipefail
ROOT=$(cd "$(dirname "$0")/.." && pwd)
CHECKER="$ROOT/scripts/check-package-artifact.sh"
REAL_BUNDLE=${1:-"$ROOT/build/unraid-management-agent-$(cat "$ROOT/VERSION").tgz"}
# First check the real artifact with the real tools, before installing fixtures.
bash "$CHECKER" check "$REAL_BUNDLE"
SANDBOX=$(mktemp -d)
trap 'rm -rf "$SANDBOX"' EXIT
mkdir "$SANDBOX/bin"
python3 - "$SANDBOX" <<'PY'
import io, pathlib, sys, tarfile
base = pathlib.Path(sys.argv[1])
member = 'usr/local/emhttp/plugins/unraid-management-agent/unraid-management-agent'
for kind in ('valid', 'missing', 'duplicate', 'symlink'):
    with tarfile.open(base / (kind + '.tgz'), 'w:gz') as archive:
        if kind == 'missing':
            continue
        entry = tarfile.TarInfo(member)
        if kind == 'symlink':
            entry.type = tarfile.SYMTYPE
            entry.linkname = '/bin/true'
            archive.addfile(entry)
        else:
            entry.size = 4
            for _ in range(2 if kind == 'duplicate' else 1):
                archive.addfile(entry, io.BytesIO(b'test'))
PY
cat > "$SANDBOX/bin/go" <<'SH'
#!/usr/bin/env bash
set -eu
[[ "$1" == version && "$2" == -m ]] || exit 90
[[ "$ARTIFACT_TEST_CASE" != go-fail ]] || exit 2
for entry in GOOS=linux GOARCH=amd64 GOAMD64=v1 CGO_ENABLED=0; do
  case "$ARTIFACT_TEST_CASE:$entry" in
    missing-build:GOOS=linux) continue ;;
    wrong-os:GOOS=linux) entry=GOOS=darwin ;;
    wrong-arch:GOARCH=amd64) entry=GOARCH=arm64 ;;
    avx:GOAMD64=v1) entry=GOAMD64=v3 ;;
    cgo:CGO_ENABLED=0) entry=CGO_ENABLED=1 ;;
  esac
  printf '\tbuild\t%s\n' "$entry"
done
if [[ "$ARTIFACT_TEST_CASE" == duplicate-build ]]; then printf '\tbuild\tGOAMD64=v3\n'; fi
SH
cat > "$SANDBOX/bin/readelf" <<'SH'
#!/bin/sh
[ "$ARTIFACT_TEST_CASE" != readelf-fail ] || exit 2
case "$1" in
  -hW)
    echo 'Class: ELF64'
    echo 'Data: 2s complement, little endian'
    if [ "$ARTIFACT_TEST_CASE" = wrong-elf ]; then echo 'Machine: AArch64'; else echo 'Machine: Advanced Micro Devices X86-64'; fi
    ;;
  -lW)
    echo 'Program Headers:'
    if [ "$ARTIFACT_TEST_CASE" = interpreter ]; then echo ' INTERP 0x00000'; fi
    ;;
  -dW)
    if [ "$ARTIFACT_TEST_CASE" = needed ]; then
      echo 'Dynamic section contains entries:'
      echo ' 0x1 (NEEDED) Shared library: [libc.so.6]'
    else echo 'There is no dynamic section in this file.'; fi
    ;;
esac
SH
cat > "$SANDBOX/bin/uname" <<'SH'
#!/bin/sh
echo Linux
SH
cat > "$SANDBOX/bin/timeout" <<'SH'
#!/bin/sh
shift
exec "$@"
SH
chmod +x "$SANDBOX/bin/"*
export PATH="$SANDBOX/bin:$PATH"
export READELF="$SANDBOX/bin/readelf"
export ARTIFACT_TEST_CASE=valid
expect_fail() {
  local name=$1 mode=${2:-check} bundle=${3:-valid}
  export ARTIFACT_TEST_CASE=$name
  if bash "$CHECKER" "$mode" "$SANDBOX/$bundle.tgz" > "$SANDBOX/out" 2> "$SANDBOX/err"; then
    echo "FAIL: accepted $name" >&2; exit 1
  fi
  [ -s "$SANDBOX/err" ] || { echo "FAIL: $name lacked an error" >&2; exit 1; }
  if grep -q '^Verified packaged' "$SANDBOX/out"; then echo "FAIL: $name reported success" >&2; exit 1; fi
  echo "PASS: reject $name"
}
for kind in missing duplicate symlink; do expect_fail "$kind" check "$kind"; done
for kind in missing-build duplicate-build cgo avx wrong-os wrong-arch go-fail wrong-elf interpreter needed readelf-fail; do
  expect_fail "$kind"
done
READELF="$SANDBOX/missing-reader" expect_fail missing-reader
export ARTIFACT_TEST_CASE=valid
bash "$CHECKER" check "$SANDBOX/valid.tgz"
for kind in sigill timeout nonzero empty success; do
  cat > "$SANDBOX/bin/qemu-x86_64" <<'SH'
#!/bin/sh
[ "$#" = 4 ] && [ "$1" = -cpu ] && [ "$2" = Nehalem ] && [ "$4" = --help ] || exit 91
echo 'emulator diagnostic' >&2
SH
  case "$kind" in
    sigill) echo 'exit 132' >> "$SANDBOX/bin/qemu-x86_64" ;;
    timeout) echo 'exit 124' >> "$SANDBOX/bin/qemu-x86_64" ;;
    nonzero) echo 'exit 2' >> "$SANDBOX/bin/qemu-x86_64" ;;
    empty) echo 'exit 0' >> "$SANDBOX/bin/qemu-x86_64" ;;
    success) echo 'echo "Usage: binary <command> [flags]"' >> "$SANDBOX/bin/qemu-x86_64" ;;
  esac
  chmod +x "$SANDBOX/bin/qemu-x86_64"
  export ARTIFACT_TEST_CASE=valid
  if [ "$kind" = success ]; then
    bash "$CHECKER" smoke "$SANDBOX/valid.tgz"
  else
    expect_fail "$kind" smoke
    case "$kind" in
      sigill) grep -q SIGILL "$SANDBOX/err" ;;
      timeout) grep -q 'timed out' "$SANDBOX/err" ;;
    esac
    if [ "$kind" != empty ]; then grep -q 'emulator diagnostic' "$SANDBOX/err"; fi
  fi
done
QEMU="$SANDBOX/missing-emulator" expect_fail missing-emulator smoke
expect_fail unknown-mode unsupported
echo 'All package artifact checks passed.'
