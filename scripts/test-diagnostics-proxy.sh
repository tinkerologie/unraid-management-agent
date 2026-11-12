#!/bin/bash
# Tests the settings page's capabilities and tool-policy proxies with a fake
# curl on PATH and isolated config and policy files.
# Run by `make test-packaging`.
#
# Usage: bash scripts/test-diagnostics-proxy.sh

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PROXY="${REPO_ROOT}/meta/plugin/include/diagnostics.php"

# Prints an error and exits non-zero.
fail() {
    echo "✗ $*" >&2
    exit 1
}

if ! command -v php >/dev/null 2>&1; then
    echo "- php not installed; skipped the diagnostics proxy tests"
    exit 0
fi

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

CONFIG="$WORK/config.cfg"
POLICY="$WORK/tool_policy.json"
# Redirect the config and both policy path assignments. A source change that
# breaks the substitutions must fail before any test invokes the proxy.
php -- "$PROXY" "$WORK/proxy.php" "$CONFIG" "$POLICY" <<'PHP'
<?php
[$script, $source, $target, $config, $policy] = $argv;
$contents = file_get_contents($source);
$contents = str_replace('"/boot/config/plugins/$plugin/config.cfg"', var_export($config, true), $contents, $configCount);
$contents = str_replace('"/boot/config/plugins/$plugin/tool_policy.json"', var_export($policy, true), $contents, $policyCount);
if ($configCount !== 1 || $policyCount !== 2 || file_put_contents($target, $contents) === false) {
    exit(1);
}
PHP
PROXY="$WORK/proxy.php"

# The fake curl records its arguments and uses the requested response or error.
mkdir -p "$WORK/bin"
cat >"$WORK/bin/curl" <<'CURL'
#!/bin/bash
printf '%s\n' "$@" >"$FAKE_CURL_ARGS"
if [ -n "${FAKE_CURL_EXIT:-}" ]; then
    printf '%s' "${FAKE_CURL_ERROR:-}" >&2
    exit "$FAKE_CURL_EXIT"
fi
printf '%s' "$FAKE_CURL_BODY"
CURL
chmod +x "$WORK/bin/curl"

# Runs the proxy with request fields "key=value ..." and prints
# "<http status>|<body>".
# shellcheck disable=SC2016 # PHP code, not shell expansions
run_proxy() {
    PATH="$WORK/bin:$PATH" FAKE_CURL_ARGS="$WORK/args" php -r '
        $_SERVER["REQUEST_METHOD"] = $argv[2];
        foreach (array_slice($argv, 3) as $kv) {
            [$k, $v] = explode("=", $kv, 2);
            if ($k === "policies") {
                $_POST[$k] = $v;
            } else {
                $_REQUEST[$k] = $v;
            }
        }
        register_shutdown_function(function () {
            $body = ob_get_clean();
            echo (http_response_code() ?: 200) . "|" . $body;
        });
        ob_start();
        include $argv[1];
    ' -- "$PROXY" "${PROXY_METHOD:-GET}" "$@"
}

BODY='{"unraid_version":"7.3.3","items":[{"name":"hardware.nic_optics","state":"absent"}]}'

out=$(FAKE_CURL_BODY="$BODY" run_proxy action=capabilities)
[ "$out" = "200|$BODY" ] || fail "capabilities: got '$out'"
url=$(tail -n1 "$WORK/args")
[ "$url" = "http://127.0.0.1:8043/api/v1/capabilities" ] || fail "capabilities: curl URL '$url'"
grep -qx -- '-m' "$WORK/args" || fail "capabilities: curl has no timeout"
echo "✓ capabilities passes the agent's JSON through from /api/v1/capabilities"

for flag in 1 true; do
    FAKE_CURL_BODY="$BODY" run_proxy action=capabilities "refresh=$flag" >/dev/null
    url=$(tail -n1 "$WORK/args")
    [ "$url" = "http://127.0.0.1:8043/api/v1/capabilities?refresh=true" ] || fail "refresh=$flag: curl URL '$url'"
done
for flag in 0 false yes ""; do
    FAKE_CURL_BODY="$BODY" run_proxy action=capabilities "refresh=$flag" >/dev/null
    url=$(tail -n1 "$WORK/args")
    [ "$url" = "http://127.0.0.1:8043/api/v1/capabilities" ] || fail "refresh=$flag: curl URL '$url'"
done
echo "✓ refresh=1 or refresh=true adds ?refresh=true; anything else does not"

out=$(FAKE_CURL_EXIT=7 FAKE_CURL_BODY="" run_proxy action=capabilities)
status="${out%%|*}"
body="${out#*|}"
[ "$status" = "502" ] || fail "curl failure: status '$status', want 502"
# shellcheck disable=SC2016 # PHP code, not shell expansions
php -r 'exit(isset(json_decode($argv[1], true)["error"]) ? 0 : 1);' -- "$body" || fail "curl failure: body '$body' has no JSON error"
echo "✓ An unreachable agent returns HTTP 502 with a JSON error"

printf 'API_TOKEN="fixture-token"\nTOOL_POLICY="old=hidden"\n' > "$CONFIG"
printf '{"old":"hidden"}' > "$POLICY"

save_policy() {
    PROXY_METHOD=POST FAKE_CURL_BODY='{}' run_proxy action=save_tool_policy "$@"
}

assert_response() {
    local response=$1 expected_status=$2 expected_field=$3 expected_value=$4
    [ "${response%%|*}" = "$expected_status" ] || fail "expected HTTP $expected_status, got '$response'"
    php -r '
        $response = json_decode($argv[1], true, flags: JSON_THROW_ON_ERROR);
        exit(json_encode($response[$argv[2]] ?? null) === $argv[3] ? 0 : 1);
    ' -- "${response#*|}" "$expected_field" "$expected_value" || fail "unexpected $expected_field in '$response'"
}

out=$(save_policy 'policies={"get_system_info":"hidden","remove_me":"default"}')
assert_response "$out" 200 live_update true
grep -qxF 'PUT' "$WORK/args" || fail "instant save did not send PUT"
grep -qxF 'Authorization: Bearer fixture-token' "$WORK/args" || fail "instant save lost authentication"
grep -qxF '{"policies":{"get_system_info":"hidden"}}' "$WORK/args" || fail "wrong PUT payload"
grep -qxF 'http://127.0.0.1:8043/api/v1/mcp/tool-policy' "$WORK/args" || fail "wrong policy endpoint"
grep -qxF 'TOOL_POLICY="get_system_info=hidden"' "$CONFIG" || fail "config was not synchronized"
grep -qF '"get_system_info": "hidden"' "$POLICY" || fail "policy file was not updated"
! grep -qE 'remove_me|"old"' "$POLICY" || fail "policy file did not replace the old map"
echo "✓ Instant save sends an authenticated live replacement and synchronizes both files"

cp "$POLICY" "$WORK/policy-before"
cp "$CONFIG" "$WORK/config-before"
rm "$WORK/args"
out=$(run_proxy action=save_tool_policy 'policies={}')
assert_response "$out" 405 error '"Method not allowed"'
out=$(save_policy)
assert_response "$out" 400 error '"No policy payload received"'
out=$(save_policy 'policies=not-json')
assert_response "$out" 400 error '"Invalid JSON payload"'
out=$(save_policy 'policies={"invalid name":"hidden"}')
assert_response "$out" 400 error '"Invalid tool name: invalid name"'
out=$(save_policy 'policies={"get_system_info":"invalid"}')
assert_response "$out" 400 error "\"Invalid policy value 'invalid' for tool 'get_system_info'\""
[ ! -e "$WORK/args" ] || fail "invalid request reached the daemon"
out=$(FAKE_CURL_EXIT=22 FAKE_CURL_ERROR='curl: (22) The requested URL returned error: 400' save_policy 'policies={}')
assert_response "$out" 400 error '"Live agent rejected policy update"'
cmp -s "$POLICY" "$WORK/policy-before" && cmp -s "$CONFIG" "$WORK/config-before" ||
    fail "invalid request or daemon rejection changed policy files"
echo "✓ Invalid methods, payloads and rejected live writes leave policy files intact"

out=$(save_policy 'policies={}')
assert_response "$out" 200 live_update true
grep -qxF '{"policies":{}}' "$WORK/args" || fail "reset PUT was not an empty object"
[ "$(cat "$POLICY")" = '{}' ] || fail "reset policy file was not an empty object"
grep -qxF 'TOOL_POLICY=""' "$CONFIG" || fail "reset did not clear the configured policies"
echo "✓ Reset All persists an explicit empty policy object"

out=$(FAKE_CURL_EXIT=22 FAKE_CURL_ERROR='curl: (22) The requested URL returned error: 403' save_policy 'policies={"get_system_info":"hidden"}')
assert_response "$out" 200 live_update false
grep -qF '"get_system_info": "hidden"' "$POLICY" || fail "403 fallback did not save the policy"
out=$(FAKE_CURL_EXIT=7 save_policy 'policies={}')
assert_response "$out" 200 live_update false
[ "$(cat "$POLICY")" = '{}' ] || fail "connection failure fallback did not save the reset"
grep -qxF 'TOOL_POLICY=""' "$CONFIG" || fail "connection failure fallback did not clear the config"
echo "✓ A forbidden or unreachable live write still saves the requested map to disk"
