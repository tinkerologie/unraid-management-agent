<?PHP
/* Diagnostics proxy.
 *
 * The webGUI is served on port 80 while the agent's API listens on its own
 * port (default 8043). A browser request from this page to the agent is
 * therefore cross-origin: fetch() is blocked (no CORS headers) and Safari
 * blocks cross-origin downloads. This helper reaches the agent server-side
 * (same origin as the page) so both the self-test (AJAX) and the diagnostics
 * download work in every browser.
 *
 *   action=selftest  (POST)  -> returns the agent self-test JSON
 *   action=download  (GET)   -> streams the diagnostics ZIP as an attachment
 *
 * Authentication and CSRF are handled by Unraid's local_prepend.php auto-prepend.
 */

$plugin = "unraid-management-agent";
$config_file = "/boot/config/plugins/$plugin/config.cfg";

// Resolve the agent base URL from config, mirroring scripts/apply's PROBE_HOST
// logic: a wildcard/empty bind address is reachable via loopback, while a
// specific bind address must be used as-is (IPv6 literals need brackets).
$config = file_exists($config_file) ? parse_ini_file($config_file, false, INI_SCANNER_RAW) : [];
$port = preg_replace('/\D/', '', (string)($config['PORT'] ?? '8043'));
if ($port === '') {
    $port = '8043';
}
$bind = trim((string)($config['BIND_ADDRESS'] ?? ''));
if ($bind === '' || $bind === '0.0.0.0' || $bind === '::') {
    $host = '127.0.0.1';
} elseif (strpos($bind, ':') !== false) {
    $host = '[' . $bind . ']';
} else {
    $host = $bind;
}
$base = "http://$host:$port";

$token = trim((string)($config['API_TOKEN'] ?? ''));
$auth_header = $token !== '' ? '-H ' . escapeshellarg("Authorization: Bearer $token") : '';

// action may arrive via GET (download navigation) or POST (self-test AJAX, policy AJAX).
$action = $_REQUEST['action'] ?? '';

switch ($action) {
    case 'selftest':
        header('Content-Type: application/json');
        $url = "$base/api/v1/diagnostics/self-test";
        // Use the curl binary (always present on Unraid) rather than the PHP
        // curl extension, consistent with scripts/apply.
        exec("curl -fsS -m 10 $auth_header " . escapeshellarg($url) . ' 2>/dev/null', $out, $rc);
        if ($rc !== 0) {
            http_response_code(502);
            echo json_encode([
                'error' => 'Could not reach the agent self-test endpoint — is the service running?',
                'rc'    => $rc,
            ]);
            exit;
        }
        // Pass the agent's JSON through verbatim.
        echo implode("\n", $out);
        break;

    case 'download':
        // Fetch the agent's diagnostics ZIP and relay it from this same-origin
        // endpoint. Same-origin + a top-level navigation is the one download path
        // that works in every browser (cross-origin downloads are blocked in
        // Safari and are mixed-content on HTTPS webGUIs). shell_exec captures the
        // body binary-safely; the bundle is small (logs are capped), so buffering
        // it lets us send an accurate Content-Length for a well-formed response.
        $url = "$base/api/v1/diagnostics/bundle";
        $data = shell_exec("curl -fsS -m 60 $auth_header " . escapeshellarg($url));
        if ($data === null || $data === '') {
            header('Content-Type: application/json');
            http_response_code(502);
            echo json_encode(['error' => 'Could not fetch the diagnostics bundle — is the service running?']);
            exit;
        }
        $fname = 'unraid-management-agent-diagnostics-' . preg_replace('/[^A-Za-z0-9._-]+/', '_', gethostname() ?: 'unraid') . '-' . gmdate('Ymd-His') . '.zip';
        header('Content-Type: application/zip');
        header('Content-Disposition: attachment; filename="' . $fname . '"');
        header('X-Content-Type-Options: nosniff');
        header('Content-Length: ' . strlen($data));
        echo $data;
        break;

    case 'get_tool_policy':
        header('Content-Type: application/json');
        $url = "$base/api/v1/mcp/tool-policy";
        exec("curl -fsS -m 10 $auth_header " . escapeshellarg($url) . ' 2>/dev/null', $out, $rc);
        if ($rc === 0) {
            echo implode("\n", $out);
            break;
        }
        // Fallback when daemon is stopped or unreachable: read saved file
        $policy_file = "/boot/config/plugins/$plugin/tool_policy.json";
        $saved = [];
        if (file_exists($policy_file)) {
            $saved = json_decode(file_get_contents($policy_file), true) ?: [];
        }
        echo json_encode([
            'global_read_only' => (($config['READ_ONLY'] ?? 'false') === 'true'),
            'policies'         => $saved,
            'tools'            => [],
        ]);
        break;

    case 'save_tool_policy':
        header('Content-Type: application/json');
        $raw = file_get_contents('php://input');
        if (empty($raw) && isset($_POST['policies'])) {
            $raw = json_encode(['policies' => $_POST['policies']]);
        }
        if (empty($raw)) {
            http_response_code(400);
            echo json_encode(['error' => 'No policy payload received']);
            exit;
        }

        $decoded = json_decode($raw, true);
        if (!is_array($decoded)) {
            http_response_code(400);
            echo json_encode(['error' => 'Invalid JSON payload']);
            exit;
        }

        $input_map = isset($decoded['policies']) && is_array($decoded['policies']) ? $decoded['policies'] : $decoded;
        $allowed_policies = ['default', 'hidden', 'read_only', 'allow', 'ask'];
        $clean_policies = [];

        foreach ($input_map as $tool => $pol) {
            if (!is_string($tool) || !preg_match('/^[a-zA-Z0-9_.-]+$/', $tool)) {
                http_response_code(400);
                echo json_encode(['error' => "Invalid tool name: $tool"]);
                exit;
            }
            if (!is_string($pol) || !in_array($pol, $allowed_policies, true)) {
                http_response_code(400);
                echo json_encode(['error' => "Invalid policy value '$pol' for tool '$tool'"]);
                exit;
            }
            if ($pol !== 'default' && $pol !== '') {
                $clean_policies[$tool] = $pol;
            }
        }

        // Update live agent first if running
        $url = "$base/api/v1/mcp/tool-policy";
        $payload_for_daemon = json_encode(['policies' => (object)$clean_policies]);
        $cmd = "curl -fsS -m 10 -X PUT -H 'Content-Type: application/json' $auth_header -d " . escapeshellarg($payload_for_daemon) . ' ' . escapeshellarg($url) . ' 2>&1';
        $out = [];
        $rc = 0;
        exec($cmd, $out, $rc);
        if ($rc === 22) {
            http_response_code(400);
            echo json_encode(['error' => 'Live agent rejected policy update', 'details' => implode("\n", $out)]);
            exit;
        }

        // Save to file on disk
        $policy_file = "/boot/config/plugins/$plugin/tool_policy.json";
        @mkdir(dirname($policy_file), 0750, true);
        $json_to_write = json_encode((object)$clean_policies, JSON_PRETTY_PRINT | JSON_UNESCAPED_SLASHES);
        if (file_put_contents($policy_file, $json_to_write) === false) {
            http_response_code(500);
            echo json_encode(['error' => 'Failed to write tool policy file to disk']);
            exit;
        }

        // Keep TOOL_POLICY in config.cfg synchronized
        if (file_exists($config_file)) {
            $pairs = [];
            foreach ($clean_policies as $k => $v) {
                $pairs[] = "$k=$v";
            }
            $tool_policy_str = implode(',', $pairs);
            $cfg_content = file_get_contents($config_file);
            if (preg_match('/^TOOL_POLICY=.*$/m', $cfg_content)) {
                $cfg_content = preg_replace('/^TOOL_POLICY=.*$/m', 'TOOL_POLICY="' . addcslashes($tool_policy_str, '"\\$') . '"', $cfg_content);
            } else {
                $cfg_content .= "\nTOOL_POLICY=\"" . addcslashes($tool_policy_str, '"\\$') . "\"\n";
            }
            @file_put_contents($config_file, $cfg_content);
        }

        echo json_encode([
            'success'     => true,
            'live_update' => ($rc === 0),
            'message'     => ($rc === 0) ? 'Policies applied instantly to running agent and saved to disk.' : 'Policies saved to disk (agent will apply them on next start).',
        ]);
        break;

    default:
        header('Content-Type: application/json');
        http_response_code(400);
        echo json_encode(['error' => 'Unknown action']);
}
