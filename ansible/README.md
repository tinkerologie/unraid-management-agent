# Ansible Deploy for Unraid Management Agent

Primary workflow for local build, deploy, uninstall, and live verification. Uses a **single persistent SSH connection** via ControlPersist, eliminating the "Too many authentication failures" problem.

## Prerequisites

```bash
# macOS
brew install ansible

# Linux (Debian/Ubuntu)
sudo apt install ansible

# pip
pip install ansible
```

## Setup

```bash
# 1. Create inventory from template
cp ansible/inventory.yml.example ansible/inventory.yml

# 2. Edit with your Unraid server details
#    - ansible_host: your server IP
#    - ansible_ssh_private_key_file: an existing authorized SSH private key
#    - ansible_port: SSH port, if different from 22
#    - api_token: a test token (openssl rand -hex 32)
```

Authorize the public key on Unraid under **Users → root → SSH authorized keys**.
Keep the private key on the controller with mode `0600`; never put key material
or SSH passwords in the repository. For a passphrase-protected key, unlock it
in your SSH agent before running Ansible.

Verify the server's SSH host-key fingerprint through a trusted channel, then
add its key to `~/.ssh/known_hosts`. For a nonstandard port, the entry must use
`[host]:port`. Set `uma_known_hosts_file` to use a different file. Ansible and
SCP both enforce this file and reject unknown or changed keys.

For an existing inventory, remove `ansible_password`, `ansible_ssh_pass`,
`ansible_ssh_password` and the old `ansible_ssh_common_args` block. Set the
private key path instead. Shared options in `group_vars/all.yml` apply even
when invoking Ansible from the repository root. Both transports use a separate
`~/.ansible/uma-key/` socket directory, so old password sessions cannot mask an
authentication problem.

Validate the connection before deploying:

```bash
ansible-playbook -i ansible/inventory.yml ansible/ssh-check.yml
```

This runs controller checks, a read-only SSH probe and a small temporary SCP
round trip with checksum verification and cleanup. It leaves the installed
plugin and its configuration intact. Run `make test-ansible` for local
regression checks using throwaway keys and a TEST-NET host, without connecting.

Set `api_token`: without a token the agent refuses network writes, so the
verify role's write tests (alert and health-check CRUD, invalid JSON, MCP calls)
fail. The deploy role writes the token to `config.cfg`, and the verify role
sends it on every request. A full run (no `--tags`) also runs the `uninstall`
role, which deletes the plugin config, so the server starts like a fresh install.
Use a test-only token: the verify role sends it over plain HTTP and passes it to
`curl` on the command line, so it can appear in verbose (`-v`) Ansible output.

## Usage

```bash
# Full deploy: build → deploy → verify
ansible-playbook -i ansible/inventory.yml ansible/deploy.yml

# Build only (no deploy)
ansible-playbook -i ansible/inventory.yml ansible/deploy.yml --tags build

# Deploy only (skip build, assumes package exists)
ansible-playbook -i ansible/inventory.yml ansible/deploy.yml --tags deploy

# Verify endpoints only (against already-running server)
ansible-playbook -i ansible/inventory.yml ansible/deploy.yml --tags verify

# Skip endpoint tests
ansible-playbook -i ansible/inventory.yml ansible/deploy.yml --skip-tags tests
```

## Notes

- `--tags verify` supersedes the old local validation scripts with endpoint, schema, MCP, Prometheus, Swagger, WebSocket, and error-handling checks.
