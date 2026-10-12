#!/bin/sh
# Prints the tar flags that write every archive entry as numeric owner 0/0 (#242).
# GNU tar (Linux CI) and bsdtar (macOS) spell these differently; anything else fails.

version=$(tar --version 2>/dev/null)
case "$version" in
*"GNU tar"*) echo "--owner=0 --group=0 --numeric-owner" ;;
*bsdtar*) echo "--uid 0 --gid 0 --numeric-owner" ;;
*)
    echo "Unsupported tar: need GNU tar or bsdtar" >&2
    exit 1
    ;;
esac
