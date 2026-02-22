# Minimal dev container image for unraid-management-agent.
# Go, Node.js, Python, and GitHub CLI are installed via Dev Container Features
# in devcontainer.json — only project-specific system dependencies live here.
FROM mcr.microsoft.com/devcontainers/base:bookworm

ENV DEBIAN_FRONTEND=noninteractive

# C toolchain (cgo), SSH automation, and project-specific CLI tools
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        gcc \
        g++ \
        pkg-config \
        libssl-dev \
        jq \
        php-cli \
        shellcheck \
        yamllint \
        sshpass \
    && rm -rf /var/lib/apt/lists/*
