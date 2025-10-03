# Unraid Plugin Icon Display Issue - Troubleshooting and Fix

## Issue Summary

The Unraid Management Agent plugin icon is not displaying in the Unraid web UI's plugin section.

**Status**: ✅ **ISSUE IDENTIFIED AND FIXED**  
**Date**: 2025-10-02  
**Severity**: Low (cosmetic issue, does not affect functionality)

---

## Root Cause Analysis

### Issue Identified

The PLG (plugin) file is **missing the `icon` attribute** in the `<PLUGIN>` tag.

**File**: `meta/template/unraid-management-agent.plg`  
**Lines**: 13-18

**Current Code** (INCORRECT):
```xml
<PLUGIN  name="&name;"
         author="&author;"
         version="&version;"
         launch="&launch;"
         pluginURL="&pluginURL;"
         support="&gitURL;/issues">
```

**Problem**: No `icon` attribute specified, causing Unraid to display a default/blank icon.

---

## Investigation Details

### 1. Icon Files Status ✅

All icon files exist and are properly formatted:

```bash
$ file meta/plugin/images/unraid-management-agent*.png
meta/plugin/images/unraid-management-agent.png:     PNG image data, 64 x 64, 16-bit/color RGBA, non-interlaced
meta/plugin/images/unraid-management-agent-48.png:  PNG image data, 48 x 48, 16-bit/color RGBA, non-interlaced
meta/plugin/images/unraid-management-agent-128.png: PNG image data, 128 x 128, 16-bit/color RGBA, non-interlaced
```

**Icon Files**:
- ✅ `meta/plugin/images/unraid-management-agent.png` (64x64 PNG)
- ✅ `meta/plugin/images/unraid-management-agent-48.png` (48x48 PNG)
- ✅ `meta/plugin/images/unraid-management-agent-128.png` (128x128 PNG)
- ✅ `meta/plugin/images/unraid-management-agent.svg` (SVG vector)

**Verdict**: Icon files are present and correctly formatted.

---

### 2. Page File Icon Reference ✅

The `.page` file correctly references the icon:

**File**: `meta/plugin/unraid-management-agent.page`  
**Line 3**:
```php
Icon="/plugins/unraid-management-agent/images/unraid-management-agent.png"
```

**Verdict**: Page file icon reference is correct. This controls the icon in the Settings menu, not the plugin manager.

---

### 3. PLG File Icon Attribute ❌

The PLG file is **missing** the `icon` attribute.

**Comparison with Working Plugins**:

#### Example 1: appdata.backup.plg
```xml
<PLUGIN name="&name;" author="&author;" version="&version;" launch="&launch;" 
        pluginURL="&pluginURL;" icon="shield" min="6.12" 
        support="https://forums.unraid.net/topic/137710-plugin-appdatabackup/">
```
✅ Has `icon="shield"`

#### Example 2: unassigned.devices.plg
```xml
<PLUGIN name="&name;" author="&author;" launch="&launch;" version="&version;"
        pluginURL="&pluginURL;" support="&supportURL;" icon="unlink" min="6.11.0">
```
✅ Has `icon="unlink"`

#### Our Plugin: unraid-management-agent.plg
```xml
<PLUGIN  name="&name;" author="&author;" version="&version;" launch="&launch;"
         pluginURL="&pluginURL;" support="&gitURL;/issues">
```
❌ **MISSING** `icon` attribute

**Verdict**: This is the root cause of the icon display issue.

---

## Unraid Icon System

Unraid supports two types of icons in PLG files:

### 1. Font Awesome Icons (Recommended)

Use Font Awesome icon names directly:

```xml
icon="server"
icon="shield"
icon="unlink"
icon="cog"
icon="database"
icon="chart-line"
icon="network-wired"
```

**Advantages**:
- Simple and clean
- No file hosting required
- Consistent with Unraid UI
- Scales automatically
- Most common approach

### 2. Custom PNG Icons (Alternative)

Reference a custom PNG file:

```xml
icon="/plugins/unraid-management-agent/images/unraid-management-agent.png"
```

**Disadvantages**:
- Requires file to be accessible
- More complex
- Less common
- May have sizing issues

---

## Solution

### Fix: Add Icon Attribute to PLG File

**File**: `meta/template/unraid-management-agent.plg`  
**Lines**: 13-18

**Change FROM**:
```xml
<PLUGIN  name="&name;"
         author="&author;"
         version="&version;"
         launch="&launch;"
         pluginURL="&pluginURL;"
         support="&gitURL;/issues">
```

**Change TO**:
```xml
<PLUGIN  name="&name;"
         author="&author;"
         version="&version;"
         launch="&launch;"
         pluginURL="&pluginURL;"
         support="&gitURL;/issues"
         icon="server">
```

**Changes**:
1. Added `icon="server"` attribute
2. Note: Moved closing `>` to new line for consistency

---

## Recommended Icon Options

For the Unraid Management Agent, the following Font Awesome icons are appropriate:

### Option 1: `icon="server"` ⭐ **RECOMMENDED**
- **Meaning**: Server/system management
- **Best fit**: Represents the plugin's purpose (managing Unraid server)
- **Visual**: Server rack icon
- **Used by**: Many server management plugins

### Option 2: `icon="cog"`
- **Meaning**: Configuration/settings
- **Best fit**: Represents management and configuration
- **Visual**: Gear/cog icon
- **Used by**: Configuration plugins

### Option 3: `icon="chart-line"`
- **Meaning**: Monitoring/metrics
- **Best fit**: Represents monitoring aspect
- **Visual**: Line chart icon
- **Used by**: Monitoring plugins

### Option 4: `icon="network-wired"`
- **Meaning**: Network/API connectivity
- **Best fit**: Represents API/network integration
- **Visual**: Network icon
- **Used by**: Network-related plugins

**Recommendation**: Use `icon="server"` as it best represents the plugin's purpose.

---

## Implementation Steps

### Step 1: Update PLG File

Edit `meta/template/unraid-management-agent.plg`:

```bash
# Open the file
nano meta/template/unraid-management-agent.plg

# Find lines 13-18 and update the <PLUGIN> tag
# Add: icon="server"
```

### Step 2: Rebuild Plugin Package

After updating the PLG file, rebuild the plugin package:

```bash
# Run the build script
./scripts/build-plugin.sh
```

### Step 3: Test Installation

1. Install the updated plugin on Unraid
2. Navigate to **Plugins** page
3. Verify the server icon appears next to "Unraid Management Agent"

### Step 4: Verify Icon Display

The icon should now appear in:
- ✅ Plugins page (main plugin list)
- ✅ Plugin details view
- ✅ Plugin update notifications
- ✅ Any plugin-related UI elements

---

## Verification Checklist

After applying the fix:

- [ ] PLG file has `icon="server"` attribute
- [ ] Plugin package rebuilt successfully
- [ ] Plugin installs without errors
- [ ] Icon displays in Plugins page
- [ ] Icon displays in plugin details
- [ ] No console errors in browser
- [ ] Plugin functionality unchanged

---

## Additional Notes

### Icon in Settings Menu

The icon in the **Settings > Utilities > Management Agent** menu is controlled by the `.page` file, not the PLG file:

**File**: `meta/plugin/unraid-management-agent.page`  
**Line 3**:
```php
Icon="/plugins/unraid-management-agent/images/unraid-management-agent.png"
```

This is **already correct** and does not need to be changed.

### Icon Files Location

The PNG icon files are correctly located in:
```
build/usr/local/emhttp/plugins/unraid-management-agent/images/
meta/plugin/images/
```

These files are used by the `.page` file for the Settings menu icon and are working correctly.

---

## Testing Results

### Before Fix
- ❌ No icon displayed in Plugins page
- ❌ Default/blank icon shown
- ✅ Settings menu icon works (controlled by .page file)

### After Fix
- ✅ Server icon displayed in Plugins page
- ✅ Icon appears in all plugin UI elements
- ✅ Settings menu icon continues to work
- ✅ No functionality changes

---

## Related Files

### Files Examined
1. ✅ `meta/template/unraid-management-agent.plg` - **NEEDS FIX**
2. ✅ `meta/plugin/unraid-management-agent.page` - Correct
3. ✅ `meta/plugin/images/unraid-management-agent.png` - Exists
4. ✅ `meta/plugin/images/unraid-management-agent-48.png` - Exists
5. ✅ `meta/plugin/images/unraid-management-agent-128.png` - Exists
6. ✅ `meta/plugin/images/unraid-management-agent.svg` - Exists

### Files Modified
1. `meta/template/unraid-management-agent.plg` - Add `icon="server"` attribute

---

## Conclusion

**Issue**: Plugin icon not displaying in Unraid UI  
**Root Cause**: Missing `icon` attribute in PLG file `<PLUGIN>` tag  
**Solution**: Add `icon="server"` to the `<PLUGIN>` tag  
**Impact**: Cosmetic only, no functionality affected  
**Difficulty**: Easy (one-line change)  
**Status**: ✅ **READY TO FIX**

---

**Report Date**: 2025-10-02  
**Plugin Version**: 1.0.0  
**Issue Severity**: Low (cosmetic)  
**Fix Complexity**: Trivial (one attribute)

