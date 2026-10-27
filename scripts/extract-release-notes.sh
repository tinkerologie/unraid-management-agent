#!/bin/bash
# Emit one exact version's changelog body for both GitHub and Unraid (#289).
# Usage: scripts/extract-release-notes.sh <version> [changelog]
set -euo pipefail

if [ $# -lt 1 ] || [ $# -gt 2 ]; then
    echo "Usage: $0 <version> [changelog]" >&2
    exit 2
fi
version="$1"
changelog="${2:-CHANGELOG.md}"
[[ "$version" =~ ^[0-9A-Za-z.+-]+$ ]] || { echo "Invalid release version: '$version'" >&2; exit 2; }
[ -f "$changelog" ] || { echo "Changelog not found: $changelog" >&2; exit 1; }

awk -v version="$version" '
    /^## / {
        active = 0
        heading = "## [" version "]"
        if ($0 == heading || index($0, heading " - ") == 1) {
            matches++
            active = 1
        }
        next
    }
    active { lines[++count] = $0 }
    END {
        if (matches != 1) {
            print "Expected exactly one changelog section for " version "; found " (matches + 0) > "/dev/stderr"
            exit 1
        }
        first = 1
        while (first <= count && lines[first] ~ /^[[:space:]]*$/) first++
        while (count >= first && (lines[count] ~ /^[[:space:]]*$/ || lines[count] ~ /^---[[:space:]]*$/)) count--
        for (i = first; i <= count; i++) {
            if (lines[i] !~ /^[[:space:]]*$/ && lines[i] !~ /^#+[[:space:]]/ && lines[i] !~ /^---[[:space:]]*$/) content++
        }
        if (!content) {
            print "Release notes are empty for " version > "/dev/stderr"
            exit 1
        }
        for (i = first; i <= count; i++) print lines[i]
    }
' "$changelog"
