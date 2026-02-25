<?PHP
/* Pre-save hook — called by Unraid's /update.php via #include.
 * Runs BEFORE the config file is written, so we can read the existing
 * config and preserve values that the form intentionally omits (e.g.
 * the MQTT password is never sent back to the browser for security).
 */

$plugin = "unraid-management-agent";
$cfg_path = "/boot/config/plugins/$plugin/config.cfg";

// Preserve MQTT password: the form never sends the stored password, so
// $_POST['MQTT_PASSWORD'] will be empty when the user didn't change it.
// Read the existing password from the current config and restore it.
if (isset($_POST['MQTT_PASSWORD']) && $_POST['MQTT_PASSWORD'] === '') {
    if (is_file($cfg_path)) {
        $existing = parse_ini_file($cfg_path, false, INI_SCANNER_RAW);
        if (!empty($existing['MQTT_PASSWORD'])) {
            $_POST['MQTT_PASSWORD'] = $existing['MQTT_PASSWORD'];
        }
    }
}
