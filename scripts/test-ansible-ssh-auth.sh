#!/usr/bin/env bash
# Exercise the controller gate without connecting to a server.
set -euo pipefail
ROOT=$(cd "$(dirname "$0")/.." && pwd)
for tool in ansible-playbook ssh-keygen ssh scp python3; do
  command -v "$tool" >/dev/null || { echo "Required tool missing: $tool" >&2; exit 1; }
done
SANDBOX=$(mktemp -d)
trap 'rm -rf "$SANDBOX"' EXIT
export ANSIBLE_CONFIG="$ROOT/ansible/ansible.cfg"
export ANSIBLE_STDOUT_CALLBACK=ansible.builtin.default
export ANSIBLE_NOCOLOR=1
export UMA_SSH_TEST_TRACE="$SANDBOX/connections"

python3 - "$ROOT" <<'PY'
import pathlib, re, sys
root = pathlib.Path(sys.argv[1])
for path in (root / 'ansible').rglob('*'):
    if not path.is_file() or path.name == 'inventory.yml':
        continue
    if path.suffix not in ('.yml', '.yaml', '.cfg', '.example'):
        continue
    text = path.read_text()
    for pattern in (r'\bsshpass\b', r'^\s*ansible_(?:password|ssh_pass|ssh_password)\s*:',
                    r'PreferredAuthentications=password', r'PubkeyAuthentication=no',
                    r'StrictHostKeyChecking=no', r'UserKnownHostsFile=/dev/null'):
        assert not re.search(pattern, text, re.M), f'Unsafe SSH configuration in {path}'
deploy = (root / 'ansible/roles/deploy/tasks/main.yml').read_text()
policy = (root / 'ansible/group_vars/all.yml').read_text()
assert 'uma_scp_argv' in deploy
assert all(value in policy for value in ('ansible_ssh_private_key_file', "'-P'", 'uma_ssh_option_argv'))
PY

# Ansible selects its YAML inventory parser by extension; .example is not YAML.
cp "$ROOT/ansible/inventory.yml.example" "$SANDBOX/example.yml"
ansible-playbook -i "$SANDBOX/example.yml" "$ROOT/ansible/deploy.yml" --syntax-check
ansible-playbook -i "$SANDBOX/example.yml" "$ROOT/ansible/ssh-check.yml" --syntax-check
ssh-keygen -q -t ed25519 -N '' -f "$SANDBOX/key"
printf '[192.0.2.15]:2222 %s\n' "$(cat "$SANDBOX/key.pub")" > "$SANDBOX/known_hosts"
mkdir "$SANDBOX/bin"
cat > "$SANDBOX/bin/ssh" <<'SH'
#!/bin/sh
echo unexpected-connection >> "$UMA_SSH_TEST_TRACE"
exit 99
SH
cp "$SANDBOX/bin/ssh" "$SANDBOX/bin/scp"
chmod +x "$SANDBOX/bin/ssh" "$SANDBOX/bin/scp"
export PATH="$SANDBOX/bin:$PATH"
SENTINEL=$(python3 -c 'import secrets; print(secrets.token_hex(20))')

run_case() {
  local name=$1 expected=$2 message=$3 changes=$4 tag=${5:-preflight}
  python3 - "$SANDBOX" "$changes" <<'PY'
import json, pathlib, sys
base = pathlib.Path(sys.argv[1])
host = {'ansible_host': '192.0.2.15', 'ansible_user': 'root', 'ansible_port': 2222,
        'ansible_ssh_private_key_file': str(base / 'key'),
        'uma_known_hosts_file': str(base / 'known_hosts'),
        'uma_ssh_control_dir': str(base / 'sockets')}
for name, value in json.loads(sys.argv[2]).items():
    if value is None:
        host.pop(name, None)
    else:
        host[name] = value
(base / 'inventory.json').write_text(json.dumps({'all': {'hosts': {'unraid': host}}}))
PY
  local rc=0
  ansible-playbook -i "$SANDBOX/inventory.json" "$ROOT/ansible/ssh-check.yml" \
    --tags "$tag" > "$SANDBOX/output" 2>&1 || rc=$?
  if { [ "$expected" = pass ] && [ "$rc" -ne 0 ]; } ||
    { [ "$expected" = fail ] && [ "$rc" -eq 0 ]; }; then
    cat "$SANDBOX/output" >&2
    echo "FAIL: $name (rc=$rc)" >&2
    exit 1
  fi
  if ! grep -Fq "$message" "$SANDBOX/output" || grep -Fq "$SENTINEL" "$SANDBOX/output"; then
    echo "FAIL: $name did not report its problem safely" >&2
    exit 1
  fi
  [ ! -e "$UMA_SSH_TEST_TRACE" ] || { echo "FAIL: preflight connected to SSH" >&2; exit 1; }
  echo "PASS: $name"
}

run_case valid pass 'failed=0' '{}'
for alias in ansible_password ansible_ssh_pass ansible_ssh_password; do
  run_case "$alias" fail 'Remove SSH password variables' "{\"$alias\":\"$SENTINEL\"}"
done
run_case missing-user fail 'Set ansible_user' '{"ansible_user":null}'
run_case user-alias fail 'remove ansible_ssh_user' '{"ansible_ssh_user":"root"}'
run_case port-alias fail 'ansible_ssh_port aliases' '{"ansible_ssh_port":2223}'
run_case unset-key fail 'Set ansible_ssh_private_key_file' '{"ansible_ssh_private_key_file":""}'
run_case missing-key fail 'readable regular file' '{"ansible_ssh_private_key_file":"/missing/uma-test-key"}'
chmod 0644 "$SANDBOX/key"
run_case unsafe-key-mode fail 'owner-only permissions' '{}'
chmod 0700 "$SANDBOX/key"
run_case executable-key-mode fail 'mode 0400 or 0600' '{}'
chmod 0600 "$SANDBOX/key"
run_case ssh-override fail 'Remove inventory SSH argument overrides' '{"ansible_ssh_common_args":"-o BatchMode=no"}'
ANSIBLE_HOST_KEY_CHECKING=False run_case host-checking-disabled fail 'HOST_KEY_CHECKING' '{}'
run_case unknown-host fail 'verified host and port entry' '{"ansible_port":2223}'
for tag in build uninstall deploy verify redeploy; do
  # The lifecycle playbook must reject a password before any tagged remote task.
  python3 - "$ROOT" "$SANDBOX" "$tag" "$SENTINEL" <<'PY'
import json, pathlib, subprocess, sys
root, base = map(pathlib.Path, sys.argv[1:3])
inventory = json.loads((base / 'inventory.json').read_text())
inventory['all']['hosts']['unraid']['ansible_password'] = sys.argv[4]
(base / 'inventory.json').write_text(json.dumps(inventory))
result = subprocess.run(['ansible-playbook', '-i', str(base / 'inventory.json'),
                         str(root / 'ansible/deploy.yml'), '--tags', sys.argv[3]],
                        capture_output=True, text=True)
output = result.stdout + result.stderr
assert result.returncode != 0 and 'Remove SSH password variables' in output
assert sys.argv[4] not in output
assert not (base / 'connections').exists()
print('PASS: lifecycle gate for ' + sys.argv[3])
PY
done

# A valid first host must never hide an invalid later host in an inventory group.
python3 - "$ROOT" "$SANDBOX" "$SENTINEL" <<'PY'
import copy, json, pathlib, subprocess, sys
root, base = map(pathlib.Path, sys.argv[1:3])
first = {'ansible_host': '192.0.2.15', 'ansible_user': 'root', 'ansible_port': 2222,
         'ansible_ssh_private_key_file': str(base / 'key'),
         'uma_known_hosts_file': str(base / 'known_hosts'),
         'uma_ssh_control_dir': str(base / 'sockets')}
for problem in ('password', 'missing-key'):
    second = copy.deepcopy(first)
    if problem == 'password':
        second['ansible_password'] = sys.argv[3]
    else:
        second['ansible_ssh_private_key_file'] = str(base / 'missing-key')
    inventory = {'all': {'children': {'unraid': {'hosts': {'first': first, 'second': second}}}}}
    (base / 'group.json').write_text(json.dumps(inventory))
    result = subprocess.run(['ansible-playbook', '-i', str(base / 'group.json'),
                             str(root / 'ansible/deploy.yml'), '--tags', 'deploy'],
                            capture_output=True, text=True)
    output = result.stdout + result.stderr
    assert result.returncode != 0, 'Invalid second host bypassed preflight: ' + problem
    assert sys.argv[3] not in output
    assert not (base / 'connections').exists(), 'A host connected before every preflight passed'
    print('PASS: second host rejected for ' + problem)
PY

# Exercise real SSH-check tasks and the actual deployment upload task. These
# recording transports never execute remote commands or contact the TEST-NET host.
export UMA_SSH_TEST_SANDBOX="$SANDBOX"
mkdir -p "$SANDBOX/ssh files" "$SANDBOX/controller temp"
cp "$SANDBOX/key" "$SANDBOX/ssh files/private key"
cp "$SANDBOX/known_hosts" "$SANDBOX/ssh files/known hosts"
cat > "$SANDBOX/bin/ssh" <<'PY'
#!/usr/bin/env python3
import os, sys
command = sys.argv[-1]
if 'mktemp -d /tmp/uma-ssh-check.' in command:
    print('/tmp/uma-ssh-check.fixture')
elif 'SSH-key connection OK' in command:
    print('SSH-key connection OK')
elif 'rm -f' in command and 'rmdir' in command:
    pass
else:
    raise SystemExit('Unexpected SSH command in transport fixture')
PY
cat > "$SANDBOX/bin/scp" <<'PY'
#!/usr/bin/env python3
import json, os, pathlib, shutil, sys
base = pathlib.Path(os.environ['UMA_SSH_TEST_SANDBOX'])
args = sys.argv[1:]
with (base / 'scp-argv.jsonl').open('a') as output:
    output.write(json.dumps(args) + '\n')
source, destination = args[-2:]
if source.startswith('root@192.0.2.15:'):
    shutil.copyfile(base / 'transport-payload', destination)
elif destination.startswith('root@192.0.2.15:'):
    shutil.copyfile(source, base / 'transport-payload')
else:
    raise SystemExit('Unexpected SCP endpoints in transport fixture')
PY
chmod +x "$SANDBOX/bin/ssh" "$SANDBOX/bin/scp"
python3 - "$ROOT" "$SANDBOX" <<'PY'
import json, pathlib, subprocess, sys, os
root, base = map(pathlib.Path, sys.argv[1:3])
host = {'ansible_host': '192.0.2.15', 'ansible_user': 'root', 'ansible_port': 2222,
        'ansible_ssh_private_key_file': str(base / 'ssh files/private key'),
        'uma_known_hosts_file': str(base / 'ssh files/known hosts'),
        'uma_ssh_control_dir': str(base / 'sockets'),
        'plugin_bundle': str(base / 'bundle with spaces.tgz')}
(base / 'inventory.json').write_text(json.dumps({'all': {'hosts': {'unraid': host}}}))
(base / 'bundle with spaces.tgz').write_text('transport fixture')
(base / 'group_vars').symlink_to(root / 'ansible/group_vars', target_is_directory=True)
extract = "{{ ((lookup('file', '" + str(root / 'ansible/roles/deploy/tasks/main.yml') + "') | from_yaml)[0].block | selectattr('name', 'equalto', 'Upload plugin bundle via SCP') | list) | to_nice_yaml }}"
play = [{'hosts': 'unraid', 'gather_facts': False,
         'tasks': [{'name': 'Extract the real deployment upload task', 'delegate_to': 'localhost',
                    'ansible.builtin.copy': {'dest': str(base / 'upload.yml'), 'content': extract, 'mode': '0600'}},
                   {'name': 'Exercise deployment upload only',
                    'ansible.builtin.include_tasks': str(base / 'upload.yml')}]}]
(base / 'upload-play.json').write_text(json.dumps(play))
environment = dict(os.environ, TMPDIR=str(base / 'controller temp'))
for playbook in (root / 'ansible/ssh-check.yml', base / 'upload-play.json'):
    result = subprocess.run(['ansible-playbook', '-i', str(base / 'inventory.json'), str(playbook)],
                            capture_output=True, text=True, env=environment)
    assert result.returncode == 0, result.stdout + result.stderr
records = [json.loads(line) for line in (base / 'scp-argv.jsonl').read_text().splitlines()]
assert len(records) == 3, records
for args in records:
    assert args[args.index('-i') + 1] == host['ansible_ssh_private_key_file'], args
    assert args[args.index('-P') + 1] == '2222', args
    options = [args[n + 1] for n, value in enumerate(args[:-1]) if value == '-o']
    assert 'UserKnownHostsFile=' + host['uma_known_hosts_file'] in options, options
    for option in ('BatchMode=yes', 'IdentitiesOnly=yes', 'PreferredAuthentications=publickey',
                   'PasswordAuthentication=no', 'KbdInteractiveAuthentication=no', 'StrictHostKeyChecking=yes'):
        assert options.count(option) == 1, options
remote = 'root@192.0.2.15:/tmp/uma-ssh-check.fixture/probe'
assert records[0][-1] == remote and ' ' in records[0][-2], records[0]
assert records[1][-2] == remote and ' ' in records[1][-1], records[1]
assert records[2][-2:] == [host['plugin_bundle'], 'root@192.0.2.15:/tmp/unraid-management-agent.tgz'], records[2]
print('PASS: rendered SCP upload, download and deployment argv preserve key, port and spaced paths')
PY
echo 'All Ansible SSH authentication checks passed.'
