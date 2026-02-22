# Unraid Management Agent - Development Container

Fully configured development environment for the unraid-management-agent project.
Works with **VS Code Dev Containers** and **GitHub Codespaces**.

## Quick Start

**VS Code**: Open the repo and run `Dev Containers: Reopen in Container` from the Command Palette.

**GitHub Codespaces**: Click "Code" → "Codespaces" → "Create codespace on main".

The container automatically installs all tools and dependencies on first creation (~3 minutes).

## What's Included

### Via Dev Container Features

| Tool | Version | Purpose |
| ----------- | ------- | -------------------------------- |
| Go | 1.26 | Matches deployment target |
| Node.js | 22 LTS | Prettier, npm tooling |
| Python | 3.12 | pre-commit, Ansible, pip |
| GitHub CLI | latest | `gh` commands, `gh copilot` |

### Via Dockerfile (apt)

| Package | Purpose |
| ----------- | ----------------------------------------- |
| gcc, g++ | C toolchain for cgo |
| pkg-config | Build dependency resolution |
| libssl-dev | TLS/crypto libraries |
| jq | JSON processing |
| php-cli | Plugin page validation |
| shellcheck | Shell script linting |
| yamllint | YAML file linting |
| sshpass | Non-interactive SSH (deployment scripts) |

### Via on-create.sh (first launch)

| Tool | Purpose |
| --------------- | -------------------------------------- |
| golangci-lint | Go linting (v2, zero tolerance) |
| gosec | Go security scanner |
| govulncheck | Go vulnerability checker |
| swag | Swagger doc generator |
| goimports | Go import organizer |
| pre-commit | Git hooks framework |
| ansible | Deployment automation (`ansible/` dir) |
| ansible-lint | Ansible playbook linter |

### VS Code Extensions

- **Go** — Language support, debugging, testing
- **Python** — Required by Ansible extension
- **Ansible** — Playbook editing, linting, syntax highlighting
- **Makefile Tools** — Makefile support
- **Prettier** — Markdown, YAML, JSON formatting
- **GitHub Copilot + Chat** — AI-powered coding
- **GitHub Pull Requests** — PR review in editor
- **GitHub Actions** — Workflow editing
- **GitHub Codespaces** — Codespace management

## Container Lifecycle

The dev container uses the optimized lifecycle for fast startup and Codespaces caching:

```
onCreateCommand        →  Install Go tools, Python tools, Ansible (one-time)
updateContentCommand   →  go mod download (runs on code changes in Codespaces)
postCreateCommand      →  pre-commit install (quick, sets up git hooks)
```

## Common Tasks

```bash
# Build for Unraid (Linux/amd64)
make release

# Run all tests
make test

# Run pre-commit checks
pre-commit run --all-files

# Deploy to Unraid via Ansible
cd ansible && ansible-playbook -i inventory.yml deploy.yml

# Deploy to Unraid via script
cp scripts/config.sh.example scripts/config.sh  # configure first
./scripts/deploy-plugin.sh

# Generate Swagger docs
make swagger
```

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│ Development Container                                       │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│ Features: Go 1.26, Node.js 22, Python 3.12, GitHub CLI     │
│ Go Tools: golangci-lint, gosec, govulncheck, swag          │
│ Ansible:  ansible, ansible-lint                             │
│ Linters:  shellcheck, yamllint, pre-commit                 │
│ SSH:      sshpass for remote Unraid deployment              │
│                                                              │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
              ┌──────────────────────┐
              │ Unraid Server        │
              │ (your-server:8043)   │
              │                      │
              │ Running agent        │
              └──────────────────────┘
```

## Environment Variables

- `GOOS=linux` — Target operating system (always Linux for Unraid)
- `GOARCH=amd64` — Target architecture (always amd64 for Unraid)

## Codespaces Notes

- **Machine size**: Requests 4 CPUs, 8 GB RAM, 32 GB storage via `hostRequirements`
- **Port 8043**: Labeled "Agent API", auto-forwarded with notification
- **Caching**: `onCreateCommand` runs once; `updateContentCommand` refreshes deps on branch changes
- **Open files**: `README.md` opens automatically on attach

## Troubleshooting

### Tool not found after container creation

Re-run the setup script:

```bash
.devcontainer/on-create.sh
```

### Pre-commit hooks not installed

```bash
pre-commit install --install-hooks
```

### Ansible not found

```bash
pip install ansible ansible-lint
```

## References

- [Dev Container Features](https://containers.dev/features)
- [Dev Container Spec](https://containers.dev/implementors/spec/)
- [GitHub Codespaces](https://docs.github.com/en/codespaces)
- [MCP Integration](../docs/integrations/mcp.md)
