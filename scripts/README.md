# Deployment Scripts

This directory contains scripts for deploying and managing the Unraid Management Agent plugin.

## 🔒 Security Setup

**IMPORTANT**: Before using these scripts, you must configure your server credentials.

### Initial Setup

1. **Create your configuration file**:
   ```bash
   cp scripts/config.sh.example scripts/config.sh
   ```

2. **Edit `config.sh` with your server details**:
   ```bash
   # Edit the file with your favorite editor
   nano scripts/config.sh
   # or
   vim scripts/config.sh
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

- ✅ `config.sh` is automatically excluded from git via `.gitignore`
- ✅ Never commit `config.sh` to version control
- ✅ The `config.sh.example` template is safe to commit (contains no real credentials)
- ✅ All deployment scripts now load credentials from `config.sh`

---

## 📜 Available Scripts

### `deploy-to-unraid.sh`

Deploy the agent binary to your Unraid server.

**Usage**:
```bash
./scripts/deploy-to-unraid.sh <unraid_ip> [--test]
```

**Examples**:
```bash
# Standard deployment
./scripts/deploy-to-unraid.sh 192.168.1.100

# Deployment with debug logging
./scripts/deploy-to-unraid.sh 192.168.1.100 --test
```

**Note**: This script requires the IP as a parameter and does not use `config.sh`.

---

### `deploy-plugin-with-icon-fix.sh`

Build and deploy the complete plugin package including icon fix.

**Usage**:
```bash
./scripts/deploy-plugin-with-icon-fix.sh [unraid_ip] [password] [backup]
```

**Examples**:
```bash
# Use credentials from config.sh
./scripts/deploy-plugin-with-icon-fix.sh

# Override IP and password
./scripts/deploy-plugin-with-icon-fix.sh 192.168.1.100 mypassword

# Create backup before deployment
./scripts/deploy-plugin-with-icon-fix.sh 192.168.1.100 mypassword yes
```

**Features**:
- Builds the plugin package
- Verifies icon fix
- Optionally creates backup
- Deploys to Unraid server
- Restarts the plugin service

---

### `validate-live.sh`

Comprehensive validation script that tests all API endpoints.

**Usage**:
```bash
./scripts/validate-live.sh
```

**Features**:
- Tests all API endpoints
- Compares API responses with actual system state
- Validates data accuracy
- Provides detailed test results

**Requirements**:
- `config.sh` must be configured
- `curl` must be installed
- `sshpass` must be installed for SSH automation

---

### `cleanup-backups.sh`

Remove old plugin backups from the Unraid server.

**Usage**:
```bash
./scripts/cleanup-backups.sh [unraid_ip] [password]
```

**Examples**:
```bash
# Use credentials from config.sh
./scripts/cleanup-backups.sh

# Override IP and password
./scripts/cleanup-backups.sh 192.168.1.100 mypassword
```

**Warning**: This will delete all backup directories. Use with caution!

---

### `generate-icon.sh`

Generate plugin icons in various sizes.

**Usage**:
```bash
./scripts/generate-icon.sh
```

**Features**:
- Generates icons in multiple sizes (48x48, 64x64, 96x96, 128x128)
- Creates both PNG and SVG formats
- No credentials required

---

## 🔧 Prerequisites

### Required Tools

1. **sshpass** - For automated SSH authentication
   ```bash
   # macOS
   brew install hudochenkov/sshpass/sshpass
   
   # Ubuntu/Debian
   sudo apt-get install sshpass
   
   # Fedora/RHEL
   sudo dnf install sshpass
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

## 🚨 Troubleshooting

### "Configuration file not found" Error

If you see this error:
```
ERROR: Configuration file not found!
Please create scripts/config.sh from scripts/config.sh.example
```

**Solution**:
```bash
cp scripts/config.sh.example scripts/config.sh
# Edit config.sh with your server details
```

### SSH Connection Issues

If SSH connections fail:

1. **Verify server is reachable**:
   ```bash
   ping 192.168.1.100
   ```

2. **Test SSH manually**:
   ```bash
   ssh root@192.168.1.100
   ```

3. **Check sshpass is installed**:
   ```bash
   which sshpass
   ```

### Permission Denied

If you get permission denied errors:

```bash
chmod +x scripts/*.sh
```

---

## 📝 Best Practices

1. **Never commit credentials**:
   - Always use `config.sh` for credentials
   - Never hardcode passwords in scripts
   - Verify `.gitignore` is working: `git status`

2. **Use version control for scripts**:
   - Commit script changes
   - Use `.example` files for templates
   - Document changes in commit messages

3. **Test before deploying**:
   - Use `--test` mode when available
   - Verify backups are created
   - Check logs after deployment

4. **Keep backups**:
   - Use `CREATE_BACKUP=yes` for important deployments
   - Regularly clean old backups to save space
   - Test restore procedures

---

## 🔐 Security Checklist

Before pushing to GitHub:

- [ ] `config.sh` is in `.gitignore`
- [ ] `config.sh` does not appear in `git status`
- [ ] No hardcoded passwords in committed scripts
- [ ] Template files (`.example`) contain placeholder values only
- [ ] Documentation uses example IPs (192.168.1.100, not your real IP)

---

## 📚 Additional Resources

- [Unraid Management Agent Documentation](../docs/)
- [API Reference](../docs/api/API_REFERENCE.md)
- [Changelog](../CHANGELOG.md)

