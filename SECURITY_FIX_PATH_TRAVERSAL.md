# Security Fix: Path Traversal Vulnerabilities (CWE-22)

## Summary

Fixed 5 CodeQL-identified path traversal vulnerabilities in the Unraid Management Agent by implementing comprehensive input validation for user-controlled file paths.

**Vulnerability Type:** CWE-22 - Improper Limitation of a Pathname to a Restricted Directory ('Path Traversal')

**Severity:** High - Could allow attackers to read or modify arbitrary files on the system

**Status:** ✅ FIXED

---

## Affected Files and Lines

### Before Fix:

1. **daemon/services/controllers/notification.go:43**
   - Function: `CreateNotification()` - Line 43 (WriteFile)
   - Function: `ArchiveNotification()` - Line 54 (filepath.Join with user input)
   - Function: `UnarchiveNotification()` - Line 79 (filepath.Join with user input)
   - Function: `DeleteNotification()` - Line 102 (filepath.Join with user input)

2. **daemon/services/collectors/config.go:379, 380, 386**
   - Function: `GetShareConfig()` - Line 29 (os.Open with user input)
   - Function: `UpdateShareConfig()` - Lines 379-386 (os.Rename, os.Create with user input)

---

## Vulnerability Details

### Attack Vectors

Without validation, attackers could:

1. **Read arbitrary files:**
   ```
   GET /api/v1/notifications/../../../etc/passwd.notify
   GET /api/v1/shares/../../etc/shadow/config
   ```

2. **Write to arbitrary locations:**
   ```
   POST /api/v1/shares/../../../tmp/malicious/config
   DELETE /api/v1/notifications/../../../important/file.notify
   ```

3. **Escape directory restrictions:**
   ```
   /api/v1/shares/..%2F..%2F..%2Fetc%2Fpasswd
   ```

---

## Security Fixes Implemented

### 1. Notification Controller (`daemon/services/controllers/notification.go`)

**Added:** `validateNotificationID()` function

**Validation Checks:**
- ✅ Rejects empty IDs
- ✅ Blocks parent directory references (`..`)
- ✅ Blocks absolute paths (`/`, `\`)
- ✅ Blocks path separators (`/`, `\`)
- ✅ Enforces `.notify` file extension
- ✅ Verifies resolved path stays within notifications directory

**Protected Functions:**
- `ArchiveNotification(id string)` - Line 55
- `UnarchiveNotification(id string)` - Line 85
- `DeleteNotification(id string, isArchived bool)` - Line 109

**Example Validation:**
```go
func validateNotificationID(id string) error {
    if id == "" {
        return fmt.Errorf("notification ID cannot be empty")
    }
    if strings.Contains(id, "..") {
        return fmt.Errorf("invalid notification ID: parent directory references not allowed")
    }
    if strings.HasPrefix(id, "/") || strings.HasPrefix(id, "\\") {
        return fmt.Errorf("invalid notification ID: absolute paths not allowed")
    }
    if strings.Contains(id, "/") || strings.Contains(id, "\\") {
        return fmt.Errorf("invalid notification ID: path separators not allowed")
    }
    if !strings.HasSuffix(id, ".notify") {
        return fmt.Errorf("invalid notification ID: must have .notify extension")
    }
    cleanPath := filepath.Clean(filepath.Join(notificationsDir, id))
    if !strings.HasPrefix(cleanPath, notificationsDir) {
        return fmt.Errorf("invalid notification ID: path escapes notifications directory")
    }
    return nil
}
```

### 2. Config Collector (`daemon/services/collectors/config.go`)

**Added:** `validateShareName()` function

**Validation Checks:**
- ✅ Rejects empty names
- ✅ Enforces max length (255 characters)
- ✅ Blocks parent directory references (`..`)
- ✅ Blocks absolute paths (`/`, `\`)
- ✅ Blocks path separators (`/`, `\`)
- ✅ Verifies resolved path stays within shares directory

**Protected Functions:**
- `GetShareConfig(shareName string)` - Line 26
- `UpdateShareConfig(config *dto.ShareConfig)` - Line 379

**Example Validation:**
```go
func validateShareName(name string) error {
    if name == "" {
        return fmt.Errorf("share name cannot be empty")
    }
    if len(name) > 255 {
        return fmt.Errorf("share name too long: maximum 255 characters, got %d", len(name))
    }
    if strings.Contains(name, "..") {
        return fmt.Errorf("invalid share name: parent directory references not allowed")
    }
    if strings.HasPrefix(name, "/") || strings.HasPrefix(name, "\\") {
        return fmt.Errorf("invalid share name: absolute paths not allowed")
    }
    if strings.Contains(name, "/") || strings.Contains(name, "\\") {
        return fmt.Errorf("invalid share name: path separators not allowed")
    }
    const sharesDir = "/boot/config/shares"
    cleanPath := filepath.Clean(filepath.Join(sharesDir, name+".cfg"))
    if !strings.HasPrefix(cleanPath, sharesDir) {
        return fmt.Errorf("invalid share name: path escapes shares directory")
    }
    return nil
}
```

---

## Testing

### Security Test Coverage

**Created comprehensive security tests:**

1. **daemon/services/controllers/notification_security_test.go**
   - `TestValidateNotificationID` - 14 test cases
   - `TestArchiveNotificationSecurity` - 5 malicious input tests
   - `TestUnarchiveNotificationSecurity` - 4 malicious input tests
   - `TestDeleteNotificationSecurity` - 4 malicious input tests

2. **daemon/services/collectors/config_security_test.go**
   - `TestValidateShareName` - 12 test cases
   - `TestGetShareConfigSecurity` - 5 malicious input tests
   - `TestUpdateShareConfigSecurity` - 4 malicious input tests

**All tests passing:** ✅

```bash
go test -v ./daemon/services/controllers/ -run Security
go test -v ./daemon/services/collectors/ -run Security
```

---

## Verification

### Linter Status

✅ **golangci-lint:** All issues resolved
✅ **gosec:** Path traversal warnings suppressed with proper justification

### Test Results

✅ **All security tests passing**
✅ **No regressions in existing functionality**

---

## Impact

**Before Fix:**
- Attackers could read arbitrary files (e.g., `/etc/passwd`, `/etc/shadow`)
- Attackers could write to arbitrary locations
- Attackers could delete critical system files

**After Fix:**
- All user-controlled paths are validated before use
- Path traversal attempts are rejected with clear error messages
- Paths are confined to their intended directories
- Defense-in-depth with multiple validation layers

---

## Recommendations

1. ✅ **Implemented:** Input validation at the function level (defense-in-depth)
2. ✅ **Implemented:** Comprehensive security test coverage
3. ✅ **Implemented:** Clear error messages for debugging
4. ⚠️ **Consider:** Regular security audits with CodeQL or similar tools
5. ⚠️ **Consider:** Penetration testing before major releases

---

## References

- **CWE-22:** https://cwe.mitre.org/data/definitions/22.html
- **OWASP Path Traversal:** https://owasp.org/www-community/attacks/Path_Traversal
- **GitHub Security Scanning:** https://github.com/ruaan-deysel/unraid-management-agent/security/code-scanning

