# Ship a static Go artifact for baseline x86-64 Unraid hosts

Existing decision, recorded 2026-11-07.

Release builds target Linux/amd64 with cgo disabled and the x86-64-v1 instruction baseline, rather than
depending on the build host's CPU features or shared C libraries. This preserves compatibility with older
x86-64 Unraid hardware and avoids runtime linker dependencies in the installed plugin.
Packaging verifies the shipped executable's build settings and static ELF linkage, and CI smoke-tests the
packaged artifact on an emulated Nehalem CPU.

## Consequences

Dependencies must remain compatible with a pure Go release build. Local build success on a newer computer
does not establish compatibility of the packaged executable with the supported hardware baseline.

## Evidence

- [Release build settings](../../Makefile)
- [Artifact validation and CPU smoke test](../../scripts/check-package-artifact.sh)
- [Packaging CI](../../.github/workflows/ci.yml)
- [Compatibility fixes and their rationale](../../CHANGELOG.md)
