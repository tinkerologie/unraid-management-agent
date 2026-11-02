#!/bin/bash
# Replace only CHANGES, preserving entities, comments and installation scripts (#289).
# Usage: scripts/update-plg-changes.sh <version> <notes-file> [plg]
set -euo pipefail

if [ $# -lt 2 ] || [ $# -gt 3 ]; then
    echo "Usage: $0 <version> <notes-file> [plg]" >&2
    exit 2
fi
version="$1"
notes="$2"
plg="${3:-unraid-management-agent.plg}"

fail() {
    echo "✗ $*" >&2
    exit 1
}

entity() {
    sed -n -E "s/^[[:space:]]*<!ENTITY[[:space:]]+$1[[:space:]]+\"([^\"]*)\">.*/\1/p" "$2"
}

# Require one ordered pair of delimiter lines before taking any slices.
delimiters() {
    awk '
        /^[[:space:]]*<CHANGES>[[:space:]]*$/ { opens++; start = NR }
        /^[[:space:]]*<\/CHANGES>[[:space:]]*$/ { closes++; stop = NR }
        END {
            if (opens != 1 || closes != 1 || start >= stop) exit 1
            print start, stop
        }
    ' "$1"
}

# Fit the notes to the Plugins page dialog (#312). It renders them inside a
# <pre>, which keeps every line break, and its <p> is display:flex, which puts
# each inline code span on its own line. So join wrapped lines and keep lists
# tight, because a blank line around a bullet makes Markdown wrap it in <p>.
# Only plain paragraph lines join; every other line type is kept as it is.
normalize_for_dialog() {
    awk '
        function emit() { if (text != "") print text; text = "" }
        function block_start() { emit(); if (blanks) print ""; blanks = 0; listing = 0 }

        fence != "" {
            print
            if ($0 ~ ("^ {0,3}" fence fence fence)) fence = ""
            next
        }
        /^ {0,3}(```|~~~)/ {
            block_start()
            print
            line = $0
            sub(/^ +/, "", line)
            fence = substr(line, 1, 1)
            next
        }
        /^[[:space:]]*$/ { emit(); blanks = 1; next }
        /^ {0,3}#/ || /^ {0,3}>/ || /^[[:space:]]*\|/ { block_start(); print; next }
        # Thematic breaks and setext underlines.
        /^ {0,3}(-[ -]*-[ -]*-[ -]*|_[ _]*_[ _]*_[ _]*|\*[ *]*\*[ *]*\*[ *]*|=+|-+)[[:space:]]*$/ {
            block_start()
            print
            next
        }
        /^[[:space:]]*([-*+]|[0-9]+[.)])[[:space:]]/ {
            emit()
            if (blanks && !listing) print ""
            blanks = 0
            listing = 1
            text = $0
            next
        }
        # Only a plain continuation joins, and never across a hard break.
        !blanks && text != "" && text !~ /(  |\\)$/ {
            line = $0
            sub(/^[[:space:]]+/, "", line)
            text = text " " line
            next
        }
        {
            emit()
            if (blanks) print ""
            blanks = 0
            if ($0 !~ /^[[:space:]]/) listing = 0
            if (!listing && $0 ~ /^(    |\t)/) { print; next } # an indented code block
            text = $0
        }
        END { emit() }
    ' "$1"
}

[[ "$version" =~ ^[0-9A-Za-z.+-]+$ ]] || fail "Invalid release version: '$version'"
[ -f "$plg" ] || fail "PLG not found: $plg"
[ -f "$notes" ] || fail "Notes file not found: $notes"
[ "$(entity version "$plg")" = "$version" ] || fail "PLG version does not match release version '$version'"
positions=$(delimiters "$plg") || fail "Expected exactly one ordered CHANGES block in $plg"
read -r start stop <<< "$positions"

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT HUP INT TERM

# A shell variable cannot retain NUL bytes; compare files to catch every illegal C0 byte.
LC_ALL=C tr -d '\000-\010\013\014\016-\037' < "$notes" > "$work/clean"
cmp -s "$notes" "$work/clean" || fail "Notes contain disallowed XML control characters"
awk '
    { lines[NR] = $0 }
    END {
        first = 1; last = NR
        while (first <= last && lines[first] ~ /^[[:space:]]*$/) first++
        while (last >= first && lines[last] ~ /^[[:space:]]*$/) last--
        for (i = first; i <= last; i++) print lines[i]
    }
' "$notes" > "$work/notes"
[ -s "$work/notes" ] || fail "Release notes are empty"
normalize_for_dialog "$work/notes" > "$work/normalized"
{
    printf '### %s\n\n' "$version"
    cat "$work/normalized"
} > "$work/expected"
sed -e 's/&/\&amp;/g' -e 's/</\&lt;/g' -e 's/>/\&gt;/g' "$work/expected" > "$work/escaped"
{
    head -n "$start" "$plg"
    cat "$work/escaped"
    tail -n "+$stop" "$plg"
} > "$work/updated.plg"

new_positions=$(delimiters "$work/updated.plg") || fail "Invalid generated CHANGES block"
read -r new_start new_stop <<< "$new_positions"
sed -n "$((new_start + 1)),$((new_stop - 1))p" "$work/updated.plg" > "$work/actual"
if grep -q '<' "$work/actual" || sed -e 's/&amp;//g' -e 's/&lt;//g' -e 's/&gt;//g' "$work/actual" | grep -q '&'; then
    fail "Generated CHANGES contains unescaped XML"
fi
sed -e 's/&gt;/>/g' -e 's/&lt;/</g' -e 's/&amp;/\&/g' "$work/actual" > "$work/decoded"
cmp -s "$work/expected" "$work/decoded" || fail "Generated notes failed round-trip verification"
for name in version md5 sha256; do
    [ "$(entity "$name" "$plg")" = "$(entity "$name" "$work/updated.plg")" ] || fail "$name entity changed"
done
if command -v xmllint >/dev/null 2>&1; then
    xmllint --nonet --noout "$work/updated.plg" || fail "Generated manifest is not valid XML"
fi
cat "$work/updated.plg" > "$plg"
echo "✓ $plg contains release notes for v${version}"
