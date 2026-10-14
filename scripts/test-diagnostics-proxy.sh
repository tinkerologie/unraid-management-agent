#!/bin/bash
# Tests the settings page's capabilities proxy (action=capabilities in
# meta/plugin/include/diagnostics.php, #261) with a fake curl on PATH.
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

# The fake curl records its arguments and prints $FAKE_CURL_BODY, or fails with
# exit code 7 (connection refused) when FAKE_CURL_FAIL is set.
mkdir -p "$WORK/bin"
cat >"$WORK/bin/curl" <<'CURL'
#!/bin/bash
printf '%s\n' "$@" >"$FAKE_CURL_ARGS"
if [ -n "${FAKE_CURL_FAIL:-}" ]; then
    exit 7
fi
printf '%s' "$FAKE_CURL_BODY"
CURL
chmod +x "$WORK/bin/curl"

# Runs the proxy with request fields "key=value ..." and prints
# "<http status>|<body>".
# shellcheck disable=SC2016 # PHP code, not shell expansions
run_proxy() {
    PATH="$WORK/bin:$PATH" FAKE_CURL_ARGS="$WORK/args" php -r '
        foreach (array_slice($argv, 2) as $kv) {
            [$k, $v] = explode("=", $kv, 2);
            $_REQUEST[$k] = $v;
        }
        register_shutdown_function(function () {
            $body = ob_get_clean();
            echo (http_response_code() ?: 200) . "|" . $body;
        });
        ob_start();
        include $argv[1];
    ' -- "$PROXY" "$@"
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

out=$(FAKE_CURL_FAIL=1 FAKE_CURL_BODY="" run_proxy action=capabilities)
status="${out%%|*}"
body="${out#*|}"
[ "$status" = "502" ] || fail "curl failure: status '$status', want 502"
# shellcheck disable=SC2016 # PHP code, not shell expansions
php -r 'exit(isset(json_decode($argv[1], true)["error"]) ? 0 : 1);' -- "$body" || fail "curl failure: body '$body' has no JSON error"
echo "✓ An unreachable agent returns HTTP 502 with a JSON error"
