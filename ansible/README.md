# Ansible Deploy for Unraid Management Agent

An alternative to `scripts/deploy-plugin.sh` using Ansible. Uses a **single persistent SSH connection** via ControlPersist, eliminating the "Too many authentication failures" problem.

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
```

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

# Deploy with backup
ansible-playbook -i ansible/inventory.yml ansible/deploy.yml -e create_backup=yes

# Skip endpoint tests
ansible-playbook -i ansible/inventory.yml ansible/deploy.yml --skip-tags tests
```

## Why Ansible?

| Feature                  | deploy-plugin.sh | Ansible        |
| ------------------------ | ---------------- | -------------- |
| SSH connections          | 1 (ControlMaster)| 1 (built-in)  |
| Endpoint tests via       | Local curl       | `uri` module   |
| Idempotent               | No               | Yes            |
| Error handling           | `set -e`         | Per-task       |
| Selective execution      | No               | `--tags`       |
| Setup overhead           | None             | Install Ansible|

Both approaches are supported — use whichever fits your workflow.
