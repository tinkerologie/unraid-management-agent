#!/bin/bash
# Regression tests for plugin bundle ownership (#242).
#
# Usage:
#   sudo bash scripts/test-bundle-extraction.sh
#       Runs the .plg extraction command against an archive owned by uid 1001
#       and checks that pre-existing directories keep their owner and mode and
#       that extracted files are owned by root. Requires root and GNU tar.
#       Then runs the .plg ownership-repair loop and checks it chowns only
#       non-root directories, without changing their mode.
#   bash scripts/test-bundle-extraction.sh check-owners <bundle.tgz>
#       Fails if any archive entry is not owned by 0/0.
#   bash scripts/test-bundle-extraction.sh check-tar-flags
#       Checks scripts/tar-owner-flags.sh against fake GNU tar, bsdtar and
#       unsupported tar implementations.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PLG="${REPO_ROOT}/unraid-management-agent.plg"
PLUGIN_REL="usr/local/emhttp/plugins/unraid-management-agent"

# Prints an error and exits non-zero.
fail() {
    echo "✗ $*" >&2
    exit 1
}

# Fails unless every entry in the given bundle is owned by numeric 0/0.
check_owners() {
    bundle="$1"
    [ -f "$bundle" ] || fail "Bundle not found: $bundle"
    # GNU tar lists "uid/gid" in field 2; bsdtar lists uid and gid in fields 3 and 4
    bad=$(tar --numeric-owner -tvzf "$bundle" |
        awk '{ o = ($2 ~ /\//) ? $2 : $3 "/" $4; if (o != "0/0") print }')
    [ -z "$bad" ] || fail "Entries not owned by 0/0 in ${bundle}:
${bad}"
    echo "✓ All entries in ${bundle} are owned by 0/0"
}

# Fails unless the path's "uid:gid mode" matches the expected value.
assert_stat() {
    path="$1"
    want="$2"
    got=$(stat -c '%u:%g %a' "$path")
    [ "$got" = "$want" ] || fail "${path}: expected ${want}, got ${got}"
}

# Prints the flags tar-owner-flags.sh selects when tar --version prints $1.
tar_flags_for() {
    fakebin=$(mktemp -d)
    printf '#!/bin/sh\necho "%s"\n' "$1" >"${fakebin}/tar"
    chmod +x "${fakebin}/tar"
    status=0
    PATH="${fakebin}:${PATH}" sh "${REPO_ROOT}/scripts/tar-owner-flags.sh" 2>/dev/null || status=$?
    rm -rf "$fakebin"
    return "$status"
}

# Checks tar flag selection for GNU tar, bsdtar and an unsupported tar.
check_tar_flags() {
    got=$(tar_flags_for "tar (GNU tar) 1.35")
    [ "$got" = "--owner=0 --group=0 --numeric-owner" ] || fail "GNU tar flags: got '${got}'"
    got=$(tar_flags_for "bsdtar 3.5.3 - libarchive 3.7.4")
    [ "$got" = "--uid 0 --gid 0 --numeric-owner" ] || fail "bsdtar flags: got '${got}'"
    if tar_flags_for "busybox tar" >/dev/null; then
        fail "Unsupported tar was accepted"
    fi
    echo "✓ tar-owner-flags.sh selects root-owner flags for GNU tar and bsdtar and rejects others"
}

# Runs the .plg extraction line and ownership-repair loop against a fixture root.
test_extraction() {
    [ "$(id -u)" -eq 0 ] || fail "Extraction test must run as root (use sudo)"

    work=$(mktemp -d)
    trap 'rm -rf "$work"' EXIT HUP INT TERM

    # Build an archive with the release layout, owned by the CI runner uid
    src="${work}/src"
    mkdir -p "${src}/${PLUGIN_REL}"
    echo "payload" >"${src}/${PLUGIN_REL}/VERSION"
    find "${src}/usr" -type d -exec chmod 0755 {} +
    archive="${work}/bundle.tgz"
    tar --owner=1001 --group=1001 --numeric-owner -czf "$archive" -C "$src" usr/

    # Pre-existing system directories with owners and modes the archive must not touch
    root="${work}/root"
    mkdir -p "${root}/${PLUGIN_REL}"
    chmod 0751 "${root}/usr" "${root}/usr/local" "${root}/usr/local/emhttp"
    chown 2002:2002 "${root}/usr/local/emhttp/plugins"
    chmod 0750 "${root}/usr/local/emhttp/plugins"

    # Run the production extraction line from the .plg, retargeted at the fixture root
    cmd=$(grep -E '^[[:space:]]*tar .*-xzf .*BUNDLE.* -C / ' "$PLG" || true)
    [ "$(printf '%s\n' "$cmd" | grep -c .)" -eq 1 ] || fail "Expected exactly one extraction line in ${PLG}"
    # shellcheck disable=SC2016 # literal match against the .plg line
    cmd=$(printf '%s' "$cmd" | sed 's| -C / | -C "${FIXTURE_ROOT}" |')
    echo "Running: ${cmd}"
    BUNDLE="$archive" FIXTURE_ROOT="$root" bash -c "$cmd" || fail "Extraction failed"

    assert_stat "${root}/usr" "0:0 751"
    assert_stat "${root}/usr/local" "0:0 751"
    assert_stat "${root}/usr/local/emhttp" "0:0 751"
    assert_stat "${root}/usr/local/emhttp/plugins" "2002:2002 750"
    assert_stat "${root}/${PLUGIN_REL}" "0:0 755"
    [ "$(cat "${root}/${PLUGIN_REL}/VERSION")" = "payload" ] || fail "Extracted file content mismatch"
    assert_stat "${root}/${PLUGIN_REL}/VERSION" "0:0 644"

    echo "✓ Extraction preserved existing directories and installed files as root"

    echo "not a gzip archive" >"${work}/corrupt.tgz"
    if BUNDLE="${work}/corrupt.tgz" FIXTURE_ROOT="$root" bash -c "$cmd" >/dev/null 2>&1; then
        fail "Extraction of a corrupt bundle succeeded"
    fi
    echo "✓ Extraction of a corrupt bundle fails"

    # Run the production repair loop from the .plg against the fixture root
    loop=$(sed -n '/^# Restore root ownership/,/^done/p' "$PLG")
    [ -n "$loop" ] || fail "Ownership repair loop not found in ${PLG}"
    # shellcheck disable=SC2016 # literal rewrite of the .plg loop
    loop=$(printf '%s\n' "$loop" | sed -E '/^for DIR in/s# /usr# "${FIXTURE_ROOT}"/usr#g')
    chown 1001:1001 "${root}/${PLUGIN_REL}"
    out=$(FIXTURE_ROOT="$root" INSTALL_DIR="${root}/${PLUGIN_REL}" bash -c "$loop")
    echo "$out"

    assert_stat "${root}/usr" "0:0 751"
    assert_stat "${root}/usr/local/emhttp/plugins" "0:0 750"
    assert_stat "${root}/${PLUGIN_REL}" "0:0 755"
    [ "$(printf '%s\n' "$out" | grep -c 'Restored root ownership')" -eq 2 ] ||
        fail "Expected exactly two directories to be repaired"

    echo "✓ Repair loop restored root ownership only on non-root directories, modes unchanged"

    # A failing chown must warn and move on; a missing directory must be skipped
    fakebin="${work}/fakebin"
    mkdir -p "$fakebin"
    printf '#!/bin/sh\nexit 1\n' >"${fakebin}/chown"
    chmod +x "${fakebin}/chown"
    chown 1001:1001 "${root}/usr/local" "${root}/usr/local/emhttp/plugins"
    out=$(PATH="${fakebin}:${PATH}" FIXTURE_ROOT="$root" INSTALL_DIR="${work}/missing" bash -c "$loop") ||
        fail "Repair loop exited non-zero when chown failed"
    echo "$out"
    [ "$(printf '%s\n' "$out" | grep -c 'Could not restore root ownership')" -eq 2 ] ||
        fail "Expected a warning for each directory chown could not repair"
    if printf '%s\n' "$out" | grep -q "${work}/missing"; then
        fail "Repair loop acted on a missing directory"
    fi
    echo "✓ Repair loop warns on chown failure, continues, and skips missing directories"
}

case "${1:-}" in
check-tar-flags)
    check_tar_flags
    ;;
check-owners)
    [ $# -eq 2 ] || fail "Usage: $0 check-owners <bundle.tgz>"
    check_owners "$2"
    ;;
"")
    test_extraction
    ;;
*)
    fail "Unknown mode: $1"
    ;;
esac
