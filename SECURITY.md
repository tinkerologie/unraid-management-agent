# Security Policy

## Overview

The Unraid Management Agent is a **third-party community plugin** for Unraid, not an official Unraid product. This plugin provides REST API and WebSocket interfaces for system monitoring and control.

**Important**: This plugin is designed for **trusted local LAN deployment only** and should **never be exposed to the internet**. It is intended for use within your private network, typically for integration with home automation systems like Home Assistant.

## Supported Versions

We follow a date-based versioning scheme (YYYY.MM.DD format). Security updates are provided for the following versions:

| Version | Supported | Status | Notes |
| ------- | --------- | ------ | ----- |
| 2025.11.25 | ✅ Yes | **Current** | Latest release with security fixes |
| 2025.11.24 | ⚠️ Limited | Upgrade recommended | Contains known vulnerabilities (CWE-22) |
| < 2025.11.24 | ❌ No | **Unsupported** | Immediate upgrade required |

**Recommendation**: Always use the latest version to ensure you have the most recent security patches and bug fixes.

## Reporting a Vulnerability

We take security vulnerabilities seriously and appreciate responsible disclosure. If you discover a security issue, please report it using one of the following methods:

### Preferred Method: GitHub Security Advisories (Private Disclosure)

Report security vulnerabilities privately via GitHub Security Advisories:

**🔒 <https://github.com/ruaan-deysel/unraid-management-agent/security/advisories/new>**

This allows us to work on a fix before public disclosure, protecting users from potential exploitation.

### Alternative Method: GitHub Issues (Public Disclosure)

If you prefer public disclosure or the issue is low severity, you can report via GitHub Issues:

**📋 <https://github.com/ruaan-deysel/unraid-management-agent/issues>**

### Direct Contact

For sensitive security matters, you can contact the maintainer directly:

**GitHub**: [@ruaan-deysel](https://github.com/ruaan-deysel)

## What to Include in Your Report

To help us understand and address the vulnerability quickly, please include:

- **Description**: Clear description of the vulnerability
- **Impact**: Potential impact and severity assessment
- **Steps to Reproduce**: Detailed steps to reproduce the issue
- **Proof of Concept**: Code, screenshots, or examples demonstrating the vulnerability
- **Affected Versions**: Which versions are affected
- **Suggested Fix**: If you have ideas for how to fix it (optional)

## Response Timeline

When you report a vulnerability, you can expect:

- **Initial Response**: Within **48-72 hours** acknowledging receipt of your report
- **Status Updates**: Regular updates on our progress investigating and fixing the issue
- **Security Patch**: Released as soon as possible, typically within **1-2 weeks** depending on complexity
- **Public Disclosure**: Coordinated disclosure after a fix is available

## Vulnerability Handling Process

1. **Triage**: We assess the severity and impact of the reported vulnerability
2. **Investigation**: We investigate the issue and develop a fix
3. **Testing**: The fix is thoroughly tested to ensure it resolves the issue without introducing regressions
4. **Release**: A security patch release is published with updated version number
5. **Disclosure**: Security advisory is published with details and credit to the reporter
6. **Notification**: Users are notified via GitHub release notes and plugin changelog

## Recognition

We believe in giving credit where credit is due:

- **Accepted Vulnerabilities**: Reporters will be credited in the security advisory and release notes (unless they prefer to remain anonymous)
- **Hall of Fame**: Significant security contributions may be recognized in the project README

## Security Best Practices for Users

To maximize security when using this plugin:

1. **Never expose to the internet**: This plugin is designed for local LAN use only
2. **Use a firewall**: Ensure your Unraid server is behind a firewall
3. **Keep updated**: Always use the latest version of the plugin
4. **Monitor access**: Review who has access to your local network
5. **Use strong passwords**: Secure your Unraid server with strong credentials
6. **Network segmentation**: Consider isolating your Unraid server on a separate VLAN

## Recent Security Updates

### v2025.11.25 (2025-11-18) - CRITICAL SECURITY UPDATE

**Fixed 5 CWE-22 Path Traversal Vulnerabilities (High Severity)**

- Added comprehensive input validation for file paths in notification controller and config collector
- Implemented defense-in-depth validation strategy with multiple protection layers
- Added 48 security test cases to prevent path traversal attacks
- Blocks parent directory references (`..`), absolute paths, and path separators
- Prevents attackers from reading or writing arbitrary files on the system

**Impact**: All users should upgrade immediately to v2025.11.25

**Details**: See [SECURITY_FIX_PATH_TRAVERSAL.md](SECURITY_FIX_PATH_TRAVERSAL.md) for comprehensive information

## Scope and Limitations

### In Scope

- Security vulnerabilities in the plugin code
- Path traversal, injection, and authentication bypass issues
- Information disclosure vulnerabilities
- Denial of service vulnerabilities

### Out of Scope

- Issues in third-party dependencies (report to the respective projects)
- Issues in Unraid OS itself (report to Lime Technology)
- Social engineering attacks
- Physical access attacks
- Issues requiring internet exposure (plugin is not designed for this)

## Security Disclosure Policy

We follow **coordinated disclosure**:

1. Security issues are fixed privately before public disclosure
2. Fixes are released as patch versions
3. Security advisories are published after fixes are available
4. Users are given time to upgrade before full details are disclosed

## Contact

- **Security Issues**: Use GitHub Security Advisories (preferred) or GitHub Issues
- **General Questions**: GitHub Discussions or Issues
- **Maintainer**: [@ruaan-deysel](https://github.com/ruaan-deysel)

## Additional Resources

- **GitHub Repository**: <https://github.com/ruaan-deysel/unraid-management-agent>
- **Security Advisories**: <https://github.com/ruaan-deysel/unraid-management-agent/security/advisories>
- **Issue Tracker**: <https://github.com/ruaan-deysel/unraid-management-agent/issues>
- **Changelog**: [CHANGELOG.md](CHANGELOG.md)

---

**Last Updated**: 2025-11-18
