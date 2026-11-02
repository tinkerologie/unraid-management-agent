#!/bin/bash
# Non-destructive manifest release-note regressions (#289).
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
UPDATE="$REPO_ROOT/scripts/update-plg-changes.sh"
PLG="$REPO_ROOT/unraid-management-agent.plg"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT HUP INT TERM
fail() { echo "✗ $*" >&2; exit 1; }
version=$(cat "$REPO_ROOT/VERSION")

# Comparing everything outside CHANGES also checks every script, entity and pragma.
outside_changes() {
    sed '/^<CHANGES>$/,/^<\/CHANGES>$/d' "$1"
}
cat > "$work/notes" <<'EOF'

### Fixed

- Markdown & <tag> > ]]> &name; &amp; "quotes" and 'apostrophes'.

  Indented details.

EOF
cp "$PLG" "$work/updated.plg"
bash "$UPDATE" "$version" "$work/notes" "$work/updated.plg" > /dev/null
outside_changes "$PLG" > "$work/original-outside"
outside_changes "$work/updated.plg" > "$work/updated-outside"
cmp -s "$work/original-outside" "$work/updated-outside" || fail "Content outside CHANGES changed"
grep -qF "### $version" "$work/updated.plg" || fail "Version heading missing"
grep -qF 'Markdown &amp; &lt;tag&gt; &gt; ]]&gt; &amp;name; &amp;amp;' "$work/updated.plg" || fail "XML escaping incorrect"
if command -v xmllint >/dev/null 2>&1; then
    xmllint --nonet --noout "$work/updated.plg"
    xmllint --nonet --xpath 'string(/PLUGIN/CHANGES)' "$work/updated.plg" > "$work/decoded"
    grep -qF 'Markdown & <tag> > ]]> &name; &amp;' "$work/decoded" || fail "XML parser did not recover literal notes"
else
    echo "SKIP: xmllint not installed; XML validation skipped"
fi
# The Plugins page dialog needs unwrapped lines and tight lists (#312).
normalized() {
    cp "$PLG" "$work/normalized.plg"
    bash "$UPDATE" "$version" "$1" "$work/normalized.plg" > /dev/null
    sed -n '/^<CHANGES>$/,/^<\/CHANGES>$/p' "$work/normalized.plg" | sed '1,3d;$d' \
        | sed -e 's/&gt;/>/g' -e 's/&lt;/</g' -e 's/&amp;/\&/g'
}
# shellcheck disable=SC2016 # literal Markdown backticks, not shell expansions
{
    printf 'Intro wrapped\nacross two lines.\n\n### Fixed\n\n'
    printf -- '- First bullet with `code`\n  wrapped `more` here.\n\n'
    printf -- '- Second bullet in a loose list\n  - Nested item\n    wrapped too\n\n'
    printf '  Indented paragraph.\n- Third with a hard break  \n  kept line\\\n  and another\n\n'
    printf 'Closing paragraph.\n\n| A | B |\n| - | - |\n\n> Quote\n> stays\n\nTitle\n=====\n\n---\n\n'
    printf '```text\nkeep\n  this  exactly\n\n```\n\n~~~sh\nfirst command\nsecond command\n~~~\n\n'
    printf '    indented code\n    stays\n\n1. Numbered\n   item\n2) Other\n'
} > "$work/wrapped"
# shellcheck disable=SC2016 # literal Markdown backticks, not shell expansions
{
    printf 'Intro wrapped across two lines.\n\n### Fixed\n\n'
    printf -- '- First bullet with `code` wrapped `more` here.\n'
    printf -- '- Second bullet in a loose list\n  - Nested item wrapped too\n\n'
    printf '  Indented paragraph.\n- Third with a hard break  \n  kept line\\\n  and another\n\n'
    printf 'Closing paragraph.\n\n| A | B |\n| - | - |\n\n> Quote\n> stays\n\nTitle\n=====\n\n---\n\n'
    printf '```text\nkeep\n  this  exactly\n\n```\n\n~~~sh\nfirst command\nsecond command\n~~~\n\n'
    printf '    indented code\n    stays\n\n1. Numbered item\n2) Other\n'
} > "$work/want"
normalized "$work/wrapped" > "$work/got"
diff "$work/want" "$work/got" >&2 || fail "Notes were not normalized for the Plugins page dialog"
normalized "$work/got" > "$work/again"
cmp -s "$work/got" "$work/again" || fail "Normalized notes are not a fixed point"

printf -- '- CRLF bullet\r\n  wrapped line\r\n' > "$work/crlf"
[ "$(normalized "$work/crlf")" = "- CRLF bullet wrapped line" ] || fail "CRLF notes were not joined cleanly"

# The real notes keep every word, and no blank line sits between two bullets.
bash "$REPO_ROOT/scripts/extract-release-notes.sh" "$version" "$REPO_ROOT/CHANGELOG.md" > "$work/real"
normalized "$work/real" > "$work/real-got"
[ "$(tr -s '[:space:]' '\n' < "$work/real")" = "$(tr -s '[:space:]' '\n' < "$work/real-got")" ] \
    || fail "Normalizing changed the words in the real notes"
if awk '/^- / && gap && item { found = 1 } /^$/ { gap = 1; next } { gap = 0; item = /^- / } END { exit !found }' "$work/real-got"; then
    fail "A blank line remains between bullets in the real notes"
fi
if grep -qE '^  [^ -]' "$work/real-got"; then
    fail "A wrapped bullet line remains in the real notes"
fi
cp "$work/updated.plg" "$work/first.plg"
bash "$UPDATE" "$version" "$work/notes" "$work/updated.plg" > /dev/null
cmp -s "$work/first.plg" "$work/updated.plg" || fail "Update is not idempotent"

# Simulate the next release and preserve its extracted base notes through publication.
next_version=2099.01.01
sed "s/<!ENTITY version     \"$version\">/<!ENTITY version     \"$next_version\">/" "$work/updated.plg" > "$work/next.plg"
printf '## [%s] - date\n\n- Next release only\n' "$next_version" > "$work/changelog"
bash "$REPO_ROOT/scripts/extract-release-notes.sh" "$next_version" "$work/changelog" > "$work/base"
cp "$work/base" "$work/github-body"
printf '\nDownload and checksum details\n' >> "$work/github-body"
printf '## [%s] - date\n\n- Main changed after release\n' "$next_version" > "$work/changelog"
bash "$UPDATE" "$next_version" "$work/base" "$work/next.plg" > /dev/null
grep -qF "### $next_version" "$work/next.plg" || fail "New heading missing"
grep -qF 'Next release only' "$work/next.plg" || fail "New notes missing"
if grep -qE 'Markdown|Main changed after release|Download and checksum details' "$work/next.plg"; then
    fail "Previous, main-branch or GitHub-only notes leaked into CHANGES"
fi
echo "✓ CHANGES-only replacement, literal XML round-trip, idempotence and subsequent releases"

expect_failure() {
    local name="$1" source="$2" release_version="$3" notes="$4"
    cp "$source" "$work/failure.plg"
    if bash "$UPDATE" "$release_version" "$notes" "$work/failure.plg" > /dev/null 2>&1; then
        fail "$name: update succeeded unexpectedly"
    fi
    cmp -s "$source" "$work/failure.plg" || fail "$name: manifest changed on failure"
}
expect_failure wrong-version "$PLG" 1999.01.01 "$work/notes"
expect_failure invalid-version "$PLG" '<bad>' "$work/notes"
expect_failure missing-notes "$PLG" "$version" "$work/missing"
printf ' \t\n\r\n' > "$work/empty"
expect_failure empty-notes "$PLG" "$version" "$work/empty"
for control in 000 001 010 013 014 016 037; do
    printf 'Illegal control: %b\n' "\\$control" > "$work/control"
    expect_failure "control-$control" "$PLG" "$version" "$work/control"
done
printf 'Allowed tab:\t carriage return:\r\n' > "$work/allowed"
cp "$PLG" "$work/allowed.plg"
bash "$UPDATE" "$version" "$work/allowed" "$work/allowed.plg" > /dev/null
for delimiter in '<CHANGES>' '</CHANGES>'; do
    sed "\|^$delimiter$|d" "$PLG" > "$work/malformed"
    expect_failure missing-delimiter "$work/malformed" "$version" "$work/notes"
    { cat "$PLG"; printf '%s\n' "$delimiter"; } > "$work/malformed"
    expect_failure duplicate-delimiter "$work/malformed" "$version" "$work/notes"
done
awk '{ if ($0 == "<CHANGES>") print "</CHANGES>"; else if ($0 == "</CHANGES>") print "<CHANGES>"; else print }' "$PLG" > "$work/malformed"
expect_failure reversed-delimiters "$work/malformed" "$version" "$work/notes"
if command -v xmllint >/dev/null 2>&1; then
    sed 's|</PLUGIN>|</BROKEN>|' "$PLG" > "$work/malformed"
    expect_failure malformed-xml "$work/malformed" "$version" "$work/notes"
fi
if bash "$UPDATE" "$version" "$work/notes" "$work/absent.plg" > /dev/null 2>&1; then
    fail "Missing manifest accepted"
fi
expect_usage_failure() {
    if bash "$UPDATE" "$@" > /dev/null 2>&1; then fail "Bad argument count accepted"; fi
}
expect_usage_failure
expect_usage_failure unused
expect_usage_failure unused unused unused unused
echo "✓ Invalid versions, notes, delimiters, XML and control bytes leave the manifest untouched"
