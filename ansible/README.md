# Ansible Deploy for Unraid Management Agent

Primary workflow for local build, deploy, uninstall, and live verification. Uses a **single persistent SSH connection** via ControlPersist, eliminating the "Too many authentication failures" problem.

## Prerequisites

```bash
# macOS
brew install ansible sshpass

# Linux (Debian/Ubuntu)
sudo apt install ansible sshpass

# pip
pip install ansible
```

## Setup

```bash
# 1. Create inventory from template
cp ansible/inventory.yml.example ansible/inventory.yml

# 2. Edit with your Unraid server details
#    - ansible_host: your server IP
#    - ansible_password: your root password
#    - api_token: a test token (openssl rand -hex 32)
```

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
