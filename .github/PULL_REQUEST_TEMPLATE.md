## Description

<!-- Provide a clear and concise description of what this PR does -->

## Type of Change

<!-- Check all that apply -->

- [ ] Bug fix (non-breaking change that fixes an issue)
- [ ] New feature (non-breaking change that adds functionality)
- [ ] Breaking change (fix or feature that would cause existing functionality to not work as expected)
- [ ] Documentation update
- [ ] Hardware compatibility fix
- [ ] Performance improvement
- [ ] Code refactoring (no functional changes)
- [ ] Test additions/improvements

## Pre-Submission Governance

<!--
  REQUIRED for everyone, including AI agents and automation.
  Every box below is mandatory. If any box is left unchecked, the automated
  "PR Governance" check fails and the PR will not be reviewed or merged.
  Do not open multiple overlapping or back-to-back PRs for the same work —
  batch related changes together to avoid wasting CI runner capacity.
-->

- [ ] I built and ran the project locally and verified this change actually works (not just that it compiles)
- [ ] I ran `make test` locally and all tests pass
- [ ] I ran `make pre-commit-run` (lint + security checks) locally and it passes
- [ ] I pasted real local verification output under **Testing Performed** below (no placeholder text)
- [ ] This PR is self-contained and is not a duplicate; I have not opened other overlapping or back-to-back PRs for the same change
- [ ] If an AI agent created or assisted with this PR, a human reviewed and verified the changes before submission

## Related Issues

<!-- Link to related issues, or specify "None" for self-contained changes -->
<!-- Examples: Fixes #123, Closes https://github.com/..., Related to #456, or None -->

Fixes #

## Hardware Configuration (if applicable)

<!-- If this PR fixes hardware-specific issues, provide your hardware details -->

- **CPU:**
- **Disk Controller:**
- **GPU:**
- **UPS:**
- **Network:**
- **Other:**
- **Unraid Version:**

## Changes Made

<!-- List the main changes in this PR -->

-
-
-

## Testing Performed

<!-- Check all that apply and describe what you tested -->

- [ ] Unit tests pass (`make test`)
- [ ] Added new tests for new functionality
- [ ] Tested on actual Unraid system
- [ ] Verified affected API endpoints work correctly
- [ ] Tested WebSocket events (if applicable)
- [ ] Tested with debug logging enabled
- [ ] Regression testing (ensured existing functionality still works)
- [ ] Not applicable (documentation-only or metadata-only change)

### Test Results

<!-- Describe what you tested and the results -->

## **API Endpoints Tested:**

- **Test Output/Results:**

```
[Paste relevant test output or API responses]
```

**Log Output (if relevant):**

```
[Paste relevant log entries]
```

## Documentation

<!-- Check all that apply -->

- [ ] Code comments added/updated
- [ ] README.md updated (if needed)
- [ ] AGENTS.md / developer documentation updated (if architecture or guidelines changed)
- [ ] CONTRIBUTING.md updated (if contribution process changed)
- [ ] API documentation updated (if API changed)
- [ ] No documentation needed

## Breaking Changes

<!-- If this PR introduces breaking changes, describe them and the migration path -->

## **Breaking Changes:**

## **Migration Guide:**

## Screenshots/Logs

<!-- If applicable, add screenshots or relevant log excerpts -->

## Checklist

<!-- Ensure you've completed all items before submitting -->

- [ ] I have updated CHANGELOG.md under [Unreleased] with details of this change
- [ ] I linked related issues or noted "None" in **Related Issues**
- [ ] I completed all required sections in this template and removed placeholder-only content
- [ ] My code follows the project's coding standards
- [ ] I have performed a self-review of my own code
- [ ] I have commented my code, particularly in hard-to-understand areas (if applicable)
- [ ] My changes generate no new warnings
- [ ] I have added tests that prove my fix is effective or that my feature works (if applicable)
- [ ] New and existing unit tests pass locally with my changes
- [ ] Any dependent changes have been merged and published (if applicable)
- [ ] I have read and followed the [CONTRIBUTING.md](../CONTRIBUTING.md) guidelines
- [ ] No sensitive information (API keys, passwords, personal data) is included

## Hardware Compatibility Notes

<!-- If this is a hardware compatibility fix, provide details -->

**Issue on Original Hardware:**

**Fix Implemented:**

## **Tested On:**

## **Should Also Work On:**

**Potential Impact on Other Hardware:**

## Additional Notes

<!-- Any additional information that reviewers should know -->

---

<!-- Thank you for contributing to Unraid Management Agent! -->
