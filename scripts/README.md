# Deployment Scripts

This directory contains scripts for deploying and managing the Unraid Management Agent plugin.

## Security Setup

**IMPORTANT**: Before using these scripts, you must configure your server credentials.

### Initial Setup

1. **Create your configuration file**:

   ```bash
   cp scripts/config.sh.example scripts/config.sh
   ```

2. **Edit `config.sh` with your server details**:

   ```bash
   nano scripts/config.sh
   ```

3. **Update the following values**:

   - `UNRAID_IP`: Your Unraid server IP address
   - `UNRAID_PASSWORD`: Your Unraid root password

4. **Verify the file is ignored by git**:

   ```bash
   git status
   # config.sh should NOT appear in the output
   ```

### Security Notes

- `config.sh` is automatically excluded from git via `.gitignore`
- Never commit `config.sh` to version control
- The `config.sh.example` template is safe to commit (contains no real credentials)
- All deployment scripts load credentials from `config.sh`

---

## Available Scripts

### `deploy-plugin.sh`

Build and deploy the complete plugin package to an Unraid server.

**Usage**:

```bash
./scripts/deploy-plugin.sh [unraid_ip] [password] [backup]
```

**Examples**:

```bash
# Use credentials from config.sh
./scripts/deploy-plugin.sh

# Override IP and password
./scripts/deploy-plugin.sh 192.168.1.100 mypassword

# Create backup before deployment
./scripts/deploy-plugin.sh 192.168.1.100 mypassword yes
```

**Features**:

- Builds the plugin package
- Deploys to Unraid server via SSH
- Optionally creates backup before deployment
- Restarts the plugin service

---

### `validate-live.sh`

Comprehensive validation script that tests all API endpoints against a running Unraid server.

**Usage**:

```bash
./scripts/validate-live.sh
```

**Features**:

- Tests all REST API endpoints
- Compares API responses with actual system state
- Validates data accuracy
- Provides detailed test results with pass/fail counts

**Requirements**:

- `config.sh` must be configured
- `curl` and `sshpass` must be installed

---

### `setup-pre-commit.sh`

Automated setup for pre-commit hooks and development tools.

**Usage**:

```bash
./scripts/setup-pre-commit.sh
```

**Features**:

- Installs pre-commit framework
- Configures git hooks for code quality checks
- Installs required linting tools (golangci-lint, shellcheck, etc.)

---

## Prerequisites

### Required Tools

1. **sshpass** - For automated SSH authentication

   ```bash
   # macOS
   brew install hudochenkov/sshpass/sshpass

   # Ubuntu/Debian
   sudo apt-get install sshpass
   ```

2. **curl** - For API testing (usually pre-installed)

3. **jq** - For JSON parsing (optional, but recommended)

   ```bash
   # macOS
   brew install jq

   # Ubuntu/Debian
   sudo apt-get install jq
   ```

---

## Troubleshooting

### "Configuration file not found" Error

```bash
cp scripts/config.sh.example scripts/config.sh
# Edit config.sh with your server details
```

### SSH Connection Issues

1. Verify server is reachable: `ping <your-unraid-ip>`
2. Test SSH manually: `ssh root@<your-unraid-ip>`
3. Check sshpass is installed: `which sshpass`

### Permission Denied

```bash
chmod +x scripts/*.sh
```

---

## Security Checklist

Before pushing to GitHub:

- [ ] `config.sh` is in `.gitignore`
- [ ] `config.sh` does not appear in `git status`
- [ ] No hardcoded passwords in committed scripts
- [ ] Template files (`.example`) contain placeholder values only
- [ ] Documentation uses example IPs, not your real IP
