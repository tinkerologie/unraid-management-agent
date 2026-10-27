#!/bin/bash
# Fixture-based release-note extraction regressions (#289).
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
EXTRACT="$REPO_ROOT/scripts/extract-release-notes.sh"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT HUP INT TERM

fail() { echo "✗ $*" >&2; exit 1; }

cat > "$work/changelog.md" <<'EOF'
# Changelog

## [Unreleased]

- Future work

## [2026.10.02] - 2026-10-20

### Fixed

- Exact notes with & < > ]]>

  Preserved indentation.

## [2026.10.0] - 2026-10-19

- Similar version

## [2026x10x02] - 2026-10-18

- Dots are literal

## [2026.09.01]

- Final notes

---

## Links

- Legacy links
EOF
cat > "$work/expected" <<'EOF'
### Fixed

- Exact notes with & < > ]]>

  Preserved indentation.
EOF
bash "$EXTRACT" 2026.10.02 "$work/changelog.md" > "$work/actual"
cmp -s "$work/expected" "$work/actual" || fail "Exact matching or internal Markdown spacing changed"
[ "$(bash "$EXTRACT" 2026.10.0 "$work/changelog.md")" = '- Similar version' ] || fail "Version prefix matched"
[ "$(bash "$EXTRACT" 2026.09.01 "$work/changelog.md")" = '- Final notes' ] || fail "Legacy tail captured"
[ "$(bash "$EXTRACT" Unreleased "$work/changelog.md")" = '- Future work' ] || fail "Explicit Unreleased extraction failed"
printf '## [1.0.0+rc-1]\n\n- EOF notes' > "$work/eof.md"
[ "$(bash "$EXTRACT" 1.0.0+rc-1 "$work/eof.md")" = '- EOF notes' ] || fail "EOF section lost"
echo "✓ Exact versions, adjacent sections, EOF, legacy tail and Markdown spacing"

expect_failure() {
    local name="$1"
    shift
    if bash "$EXTRACT" "$@" > "$work/output" 2> "$work/error"; then
        fail "$name: extraction succeeded unexpectedly"
    fi
    [ ! -s "$work/output" ] || fail "$name: emitted partial notes on failure"
    [ -s "$work/error" ] || fail "$name: no error message"
}
expect_failure missing 1999.01.01 "$work/changelog.md"
expect_failure literal-dots 2026.10.03 "$work/changelog.md"
printf '## [2026.10.02] - date\n \t\n---\n\n## [Other]\n- Other\n' > "$work/empty.md"
expect_failure empty 2026.10.02 "$work/empty.md"
printf '## [2026.10.02] - date\n\n### Fixed\n\n### Added\n\n---\n' > "$work/headings.md"
expect_failure headings-only 2026.10.02 "$work/headings.md"
cat "$work/changelog.md" "$work/changelog.md" > "$work/duplicate.md"
expect_failure duplicate 2026.10.02 "$work/duplicate.md"
printf '## [Unreleased]\n- Future only\n## [2026.10.020] - date\n- Prefix only\n' > "$work/prefix.md"
expect_failure unreleased-or-prefix 2026.10.02 "$work/prefix.md"
# shellcheck disable=SC2016 # This is literal invalid-version input, not a command.
for version in '' '1[2]' '1/2' '1 2' '$(id)'; do
    expect_failure invalid-version "$version" "$work/changelog.md"
done
expect_failure missing-file 2026.10.02 "$work/missing.md"
expect_failure no-arguments
expect_failure excess-arguments 2026.10.02 "$work/changelog.md" extra
echo "✓ Missing, empty, duplicate and invalid inputs fail without partial output"

(cd "$REPO_ROOT" && bash "$EXTRACT" "$(cat VERSION)") > "$work/current.md"
[ -s "$work/current.md" ] || fail "Current shipped changelog has no release notes"
if command -v xmllint >/dev/null 2>&1; then
    xmllint --nonet --noout "$REPO_ROOT/unraid-management-agent.plg"
else
    echo "SKIP: xmllint not installed; XML validation skipped"
fi
echo "✓ Current VERSION has non-empty matching changelog notes"
