# Issue tracker: GitHub

Issues and specs live in this repository's GitHub Issues.
Use the `gh` CLI from this clone, or pass `--repo tinkerologie/unraid-management-agent`
with the repository named below.

Repository: tinkerologie/unraid-management-agent

## Conventions

Follow the issue and PR governance in `AGENTS.md`.

- Create issues using the official bug or enhancement template,
  retaining the required title prefix and template fields.
- Read: `gh issue view <number> --json number,title,body,labels,comments,blockedBy`.
- List: `gh issue list --state open`, with label filters as needed.
- For multiline issue bodies and comments, write the content to a
  temporary file and use `--body-file`.
- Apply or remove labels with `gh issue edit <number> --add-label "<label>"`
  or `gh issue edit <number> --remove-label "<label>"`, using the mapping in `triage-labels.md`.
- Close completed issues with `gh issue close <number>`.
- Use native GitHub sub-issues and blocking dependencies when available.
  Otherwise, record `Part of #<parent>` and `Blocked by: #<number>`
  in the issue body.

## Pull requests as a triage surface

**PRs as a request surface: no.**

## Skill operations

When a skill says "publish to the issue tracker", create a GitHub issue
using the conventions above.

When a skill says "fetch the relevant ticket", read the GitHub issue.
