---
applyTo: "**/*.{yaml,yml,md}"
---

# YAML and Markdown Instructions

Reference: [`AGENTS.md`](../../AGENTS.md) for full project context.

## YAML

- Use 2-space indentation
- Quote strings that contain special characters
- Use `---` document separators in multi-document files
- Keep GitHub Actions workflows readable with comments

## Markdown

- Follow markdownlint rules (see `.markdownlint.json`)
- Use ATX-style headers (`#`, `##`, etc.)
- One blank line before and after headings
- Use fenced code blocks with language identifiers
- Keep lines under 120 characters where practical

## CHANGELOG.md

**Must be updated with every change.** Format:

```markdown
## [YYYY.MM.DD]

### Added

- New feature description (#issue)

### Fixed

- Bug fix description (#issue)

### Changed

- Change description (#issue)
```

Group changes by: Added, Fixed, Changed, Security, Performance, Removed.
