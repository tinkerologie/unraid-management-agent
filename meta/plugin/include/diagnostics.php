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

// action may arrive via GET (download navigation) or POST (self-test AJAX).
$action = $_REQUEST['action'] ?? '';

switch ($action) {
    case 'selftest':
        header('Content-Type: application/json');
        $url = "$base/api/v1/diagnostics/self-test";
        // Use the curl binary (always present on Unraid) rather than the PHP
        // curl extension, consistent with scripts/apply.
        exec('curl -fsS -m 10 ' . escapeshellarg($url) . ' 2>/dev/null', $out, $rc);
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
        $data = shell_exec('curl -fsS -m 60 ' . escapeshellarg($url));
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

    default:
        header('Content-Type: application/json');
        http_response_code(400);
        echo json_encode(['error' => 'Unknown action']);
}
