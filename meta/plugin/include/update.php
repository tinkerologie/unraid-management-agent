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

// Preserve API token: the form leaves the input empty when unchanged.
// If the user explicitly cleared the token (API_TOKEN_CLEAR == "1"), set it to empty.
// Otherwise, when empty, restore the existing token from config.cfg.
if (isset($_POST['API_TOKEN'])) {
    if (!empty($_POST['API_TOKEN_CLEAR']) && $_POST['API_TOKEN_CLEAR'] === '1') {
        $_POST['API_TOKEN'] = '';
    } elseif ($_POST['API_TOKEN'] === '') {
        if (is_file($cfg_path)) {
            $existing = parse_ini_file($cfg_path, false, INI_SCANNER_RAW);
            if (!empty($existing['API_TOKEN'])) {
                $_POST['API_TOKEN'] = $existing['API_TOKEN'];
            }
        }
    }
}

// Save tool policy JSON if submitted
$policy_file = "/boot/config/plugins/$plugin/tool_policy.json";
if (isset($_POST['TOOL_POLICY_JSON'])) {
    $json_data = trim($_POST['TOOL_POLICY_JSON']);
    if ($json_data !== '') {
        $decoded = json_decode($json_data, true);
        if (is_array($decoded)) {
            $allowed_policies = ['default', 'hidden', 'read_only', 'allow', 'ask'];
            $clean_policies = [];
            foreach ($decoded as $tool => $pol) {
                if (is_string($tool) && preg_match('/^[a-zA-Z0-9_.-]+$/', $tool) &&
                    is_string($pol) && in_array($pol, $allowed_policies, true) &&
                    $pol !== 'default' && $pol !== '') {
                    $clean_policies[$tool] = $pol;
                }
            }
            @mkdir(dirname($policy_file), 0750, true);
            file_put_contents($policy_file, json_encode((object)$clean_policies, JSON_PRETTY_PRINT | JSON_UNESCAPED_SLASHES));
        }
    }
    unset($_POST['TOOL_POLICY_JSON']);
}

