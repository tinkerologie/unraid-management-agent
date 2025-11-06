# Community Applications (CA) Preparation Guide

## ✅ Completed Steps

### 1. Created Production-Ready .plg File ✓
- **Location**: `unraid-management-agent.plg` (repository root)
- **Features**:
  - XML structure with proper entities for maintainability
  - Comprehensive installation script with error handling
  - Graceful uninstallation script
  - Detailed changelog from CHANGELOG.md
  - Icon URL reference
  - Support URL pointing to GitHub Issues
  - Download URL pointing to GitHub releases

### 2. Created Git Tag and GitHub Release ✓
- **Tag**: `v2025.11.0`
- **Release**: Created with comprehensive release notes
- **Release URL**: https://github.com/ruaan-deysel/unraid-management-agent/releases/tag/v2025.11.0

### 3. Committed and Pushed Changes ✓
- Committed .plg file to repository
- Pushed to main branch
- Tag pushed to GitHub

---

## 🔴 Required Manual Steps

### Step 1: Upload Package to GitHub Release

**IMPORTANT**: You need to manually upload the package file to the GitHub release.

1. **Navigate to the release page**:
   ```
   https://github.com/ruaan-deysel/unraid-management-agent/releases/tag/v2025.11.0
   ```

2. **Click "Edit release"** (pencil icon on the right)

3. **Upload the package file**:
   - Drag and drop or click to upload: `build/unraid-management-agent-2025.11.0.tgz`
   - The file is located at: `/Users/ruaandeysel/Github/Unraid/unraid-management-agent/build/unraid-management-agent-2025.11.0.tgz`
   - File size: ~6.6 MB

4. **Save the release**

5. **Verify the download URL**:
   - After upload, verify the file is accessible at:
     ```
     https://github.com/ruaan-deysel/unraid-management-agent/releases/download/v2025.11.0/unraid-management-agent-2025.11.0.tgz
     ```

### Step 2: Add Plugin Icon

**IMPORTANT**: You mentioned you have an icon.png file. You need to add it to the repository root.

1. **Copy your icon file** to the repository root:
   ```bash
   cp /path/to/your/icon.png /Users/ruaandeysel/Github/Unraid/unraid-management-agent/icon.png
   ```

2. **Icon requirements**:
   - Format: PNG
   - Recommended size: 256x256 pixels
   - File name: `icon.png`

3. **Commit and push the icon**:
   ```bash
   cd /Users/ruaandeysel/Github/Unraid/unraid-management-agent
   git add icon.png
   git commit -m "Add plugin icon for Community Applications"
   git push origin main
   ```

4. **Verify the icon URL**:
   - After pushing, verify the icon is accessible at:
     ```
     https://raw.githubusercontent.com/ruaan-deysel/unraid-management-agent/main/icon.png
     ```

---

## 🧪 Testing the .plg File

### Test 1: Manual Installation via URL

1. **Open your Unraid Web UI**

2. **Navigate to**: Plugins → Install Plugin

3. **Enter the plugin URL**:
   ```
   https://raw.githubusercontent.com/ruaan-deysel/unraid-management-agent/main/unraid-management-agent.plg
   ```

4. **Click "Install"**

5. **Verify installation**:
   - Check that the plugin appears in the Plugins list
   - Verify the icon displays correctly
   - Check that the service starts automatically
   - Verify the API is accessible: `http://YOUR_UNRAID_IP:8043/api/v1/health`

### Test 2: Verify Installation Script

After installation, verify:

```bash
# SSH into your Unraid server
ssh root@YOUR_UNRAID_IP

# Check service is running
pidof unraid-management-agent

# Check files were installed
ls -la /usr/local/emhttp/plugins/unraid-management-agent/

# Check configuration was created
cat /boot/config/plugins/unraid-management-agent/config.cfg

# Check logs
tail -f /var/log/unraid-management-agent.log
```

### Test 3: Verify Uninstallation Script

1. **Uninstall the plugin** via Unraid Web UI:
   - Go to Plugins
   - Find "unraid-management-agent"
   - Click "Remove"

2. **Verify cleanup**:
   ```bash
   # SSH into your Unraid server
   ssh root@YOUR_UNRAID_IP

   # Verify service stopped
   pidof unraid-management-agent  # Should return nothing

   # Verify plugin files removed
   ls /usr/local/emhttp/plugins/unraid-management-agent/  # Should not exist

   # Verify log file removed
   ls /var/log/unraid-management-agent.log  # Should not exist

   # Configuration should still exist (user data)
   ls /boot/config/plugins/unraid-management-agent/  # Should exist
   ```

---

## 📝 Submitting to Community Applications

### Prerequisites

Before submitting to CA, ensure:

- ✅ .plg file is in repository root
- ✅ GitHub release v2025.11.0 exists
- ✅ Package file is uploaded to the release
- ✅ Icon.png is in repository root
- ✅ Plugin installs successfully via URL
- ✅ Plugin uninstalls cleanly
- ✅ Service starts automatically after installation
- ✅ All features work as expected

### Submission Process

1. **Fork the Community Applications repository**:
   ```
   https://github.com/Squidly271/community.applications
   ```

2. **Add your plugin**:
   - Navigate to the appropriate category folder
   - Create a new XML file for your plugin
   - Follow the CA template format

3. **Create a Pull Request**:
   - Title: "Add Unraid Management Agent"
   - Description: Brief description of the plugin
   - Include link to your repository
   - Include link to support thread (create one on Unraid forums first)

4. **Wait for review**:
   - CA maintainers will review your submission
   - Address any feedback or requested changes
   - Once approved, your plugin will be available in CA

### Support Thread

Before submitting to CA, create a support thread on Unraid forums:

1. **Navigate to**: https://forums.unraid.net/forum/88-plugin-support/

2. **Create a new topic**:
   - Title: "[PLUGIN] Unraid Management Agent - REST API & WebSocket Server"
   - Include:
     - Plugin description
     - Features list
     - Installation instructions
     - Known issues
     - Changelog
     - Link to GitHub repository

3. **Update the .plg file** with the support thread URL:
   - Edit `unraid-management-agent.plg`
   - Update the `support` attribute to point to your forum thread
   - Commit and push the change

---

## 📋 Checklist

Use this checklist to track your progress:

### Pre-Submission
- [ ] Upload package to GitHub release v2025.11.0
- [ ] Add icon.png to repository root
- [ ] Test installation via .plg URL
- [ ] Test uninstallation
- [ ] Verify service auto-starts
- [ ] Verify all API endpoints work
- [ ] Create Unraid forums support thread
- [ ] Update .plg with support thread URL

### Submission
- [ ] Fork Community Applications repository
- [ ] Add plugin XML to CA repository
- [ ] Create Pull Request
- [ ] Address review feedback
- [ ] Plugin approved and merged

### Post-Submission
- [ ] Announce on Unraid forums
- [ ] Update README.md with CA installation instructions
- [ ] Monitor support thread for issues
- [ ] Respond to user feedback

---

## 🔗 Important URLs

- **Repository**: https://github.com/ruaan-deysel/unraid-management-agent
- **Release**: https://github.com/ruaan-deysel/unraid-management-agent/releases/tag/v2025.11.0
- **.plg File**: https://raw.githubusercontent.com/ruaan-deysel/unraid-management-agent/main/unraid-management-agent.plg
- **Icon URL**: https://raw.githubusercontent.com/ruaan-deysel/unraid-management-agent/main/icon.png (after upload)
- **Package URL**: https://github.com/ruaan-deysel/unraid-management-agent/releases/download/v2025.11.0/unraid-management-agent-2025.11.0.tgz (after upload)
- **Community Applications**: https://github.com/Squidly271/community.applications
- **Unraid Forums**: https://forums.unraid.net/

---

## 🆘 Troubleshooting

### Issue: .plg file fails to download package

**Solution**: Verify the package was uploaded to the GitHub release and the URL is correct.

### Issue: Icon doesn't display

**Solution**: 
1. Verify icon.png is in repository root
2. Verify icon URL is accessible
3. Check icon file size (should be < 1 MB)
4. Verify icon format is PNG

### Issue: Service doesn't start after installation

**Solution**:
1. Check logs: `/var/log/unraid-management-agent.log`
2. Verify binary has execute permissions
3. Check configuration file exists
4. Manually start: `/usr/local/emhttp/plugins/unraid-management-agent/scripts/start`

### Issue: Installation script fails

**Solution**:
1. Check Unraid system logs
2. Verify package file is not corrupted
3. Check disk space on /boot
4. Verify tar command is available

---

## 📞 Support

If you encounter issues:

1. **Check the logs**: `/var/log/unraid-management-agent.log`
2. **Review the installation output** in Unraid Web UI
3. **Create an issue**: https://github.com/ruaan-deysel/unraid-management-agent/issues
4. **Post on forums**: (after creating support thread)

---

**Last Updated**: 2025-11-06
**Version**: 2025.11.0

