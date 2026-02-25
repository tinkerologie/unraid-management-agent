<?PHP
/* Service control endpoint for AJAX requests.
 * Returns JSON with service status after executing action.
 *
 * Authentication and CSRF are handled by Unraid's local_prepend.php auto-prepend.
 */

$plugin = "unraid-management-agent";
$scripts_dir = "/usr/local/emhttp/plugins/$plugin/scripts";

header('Content-Type: application/json');

// Only allow POST requests
if ($_SERVER['REQUEST_METHOD'] !== 'POST') {
    http_response_code(405);
    header('Allow: POST');
    echo json_encode(['error' => 'Method not allowed']);
    exit;
}

$action = $_POST['action'] ?? '';
$response = [];

switch ($action) {
    case 'start':
        exec("$scripts_dir/start 2>&1", $output, $rc);
        $response['output'] = implode("\n", $output);
        $response['rc'] = $rc;
        // Brief poll to confirm startup (3 x 300ms = 0.9s max)
        for ($i = 0; $i < 3; $i++) {
            usleep(300000);
            exec("pidof $plugin 2>/dev/null", $pids, $pid_rc);
            if ($pid_rc === 0 && !empty($pids)) break;
            $pids = [];
        }
        break;
    case 'stop':
        exec("$scripts_dir/stop 2>&1", $output, $rc);
        $response['output'] = implode("\n", $output);
        $response['rc'] = $rc;
        // Brief poll to confirm shutdown (3 x 300ms = 0.9s max)
        for ($i = 0; $i < 3; $i++) {
            usleep(300000);
            exec("pidof $plugin 2>/dev/null", $pids, $pid_rc);
            if ($pid_rc !== 0 || empty($pids)) break;
            $pids = [];
        }
        break;
    case 'restart':
        exec("$scripts_dir/stop 2>&1", $output_stop, $rc_stop);
        $response['stop_output'] = implode("\n", $output_stop);
        $response['stop_rc'] = $rc_stop;
        if ($rc_stop !== 0) {
            $response['error'] = 'Stop failed';
            break;
        }
        // Poll until stopped or timeout (3 x 300ms = 0.9s max)
        for ($i = 0; $i < 3; $i++) {
            usleep(300000);
            exec("pidof $plugin 2>/dev/null", $check_pids, $check_rc);
            if ($check_rc !== 0 || empty($check_pids)) break;
            $check_pids = [];
        }
        // Abort if process is still running after stop
        exec("pidof $plugin 2>/dev/null", $guard_pids, $guard_rc);
        if ($guard_rc === 0 && !empty($guard_pids)) {
            $response['error'] = 'Stop succeeded but process still running';
            break;
        }
        exec("$scripts_dir/start 2>&1", $output, $rc);
        $response['start_output'] = implode("\n", $output);
        $response['start_rc'] = $rc;
        // Brief poll to confirm startup (3 x 300ms = 0.9s max)
        for ($i = 0; $i < 3; $i++) {
            usleep(300000);
            exec("pidof $plugin 2>/dev/null", $pids, $pid_rc);
            if ($pid_rc === 0 && !empty($pids)) break;
            $pids = [];
        }
        break;
    case 'status':
        // Just check status, no action
        break;
    default:
        http_response_code(400);
        echo json_encode(['error' => 'Invalid action']);
        exit;
}

// Check if service is running
exec("pidof $plugin 2>/dev/null", $status_pids, $status_rc);
$running = ($status_rc === 0 && !empty($status_pids));

$response['running'] = $running;
$response['action']  = $action;

// Sanitize non-UTF-8 bytes from exec output before encoding
foreach (['output', 'stop_output', 'start_output'] as $key) {
    if (isset($response[$key]) && is_string($response[$key])) {
        $response[$key] = mb_convert_encoding($response[$key], 'UTF-8', 'UTF-8');
    }
}

$json = json_encode($response);
if ($json === false) {
    http_response_code(500);
    echo json_encode([
        'error'       => 'JSON encoding failed: ' . json_last_error_msg(),
        'running'     => $running,
        'action'      => $action,
    ]);
} else {
    echo $json;
}
