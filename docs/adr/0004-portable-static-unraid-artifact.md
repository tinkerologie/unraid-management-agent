# Ship a static Go artifact for baseline x86-64 Unraid hosts

Existing decision, recorded 2026-11-07.

Release builds target Linux/amd64 and use the x86-64-v1 instruction baseline. The builds disable cgo.
The executable runs on older x86-64 Unraid hardware without shared C libraries at runtime.
The packaging check examines the executable's build settings and static ELF linkage.
CI runs a smoke test on an emulated Nehalem CPU.

## Consequences

Dependencies must remain compatible with a pure Go release build. Local build success on a newer computer
does not establish compatibility of the packaged executable with the supported hardware baseline.

## Evidence

- [Release build settings](../../Makefile)
- [Artifact validation and CPU smoke test](../../scripts/check-package-artifact.sh)
- [Packaging CI](../../.github/workflows/ci.yml)
- [Compatibility fixes and their rationale](../../CHANGELOG.md)
