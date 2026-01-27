# Devcontainer image for unraid-management-agent
ARG BASE_IMAGE=mcr.microsoft.com/devcontainers/base:bookworm
FROM ${BASE_IMAGE}

ARG GO_VERSION=1.25.0
ARG NODE_VERSION=20

ENV DEBIAN_FRONTEND=noninteractive

# Base tooling and build dependencies
# Added development tools: php-cli (plugin page validation), shellcheck (bash linting), yamllint (YAML validation)
# Added sshpass (SSH passwordless authentication), pre-commit (git hooks framework), mosquitto-clients (MQTT testing)
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        wget \
        git \
        make \
        bash-completion \
        gcc \
        g++ \
        pkg-config \
        unzip \
        tar \
        gnupg \
        lsb-release \
        software-properties-common \
        libssl-dev \
        openssh-client \
        jq \
        php-cli \
        shellcheck \
        yamllint \
        sshpass \
        python3-pip \
        python3-venv \
        python3-dev \
        mosquitto-clients \
    && rm -rf /var/lib/apt/lists/*

# Install pre-commit framework using pip (must be after python3-pip is installed)
RUN pip3 install --no-cache-dir --break-system-packages pre-commit 2>/dev/null || pip3 install --no-cache-dir pre-commit

# Install Go (matches go.mod requirement) for linux/amd64 (deployment target)
RUN curl -fsSL https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz -o /tmp/go.tgz \
    && rm -rf /usr/local/go \
    && tar -C /usr/local -xzf /tmp/go.tgz \
    && rm /tmp/go.tgz

ENV GOOS=linux
ENV GOARCH=amd64
ENV GOPATH=/go
ENV GOBIN=/go/bin
ENV PATH=/usr/local/go/bin:${GOBIN}:${PATH}
RUN echo 'export PATH=/usr/local/go/bin:/go/bin:$PATH' | tee /etc/profile.d/000-go-path.sh >/dev/null
RUN mkdir -p /go /workspaces && chown -R vscode:vscode /go /workspaces

# Install Node.js (needed for Copilot CLI)
RUN curl -fsSL https://deb.nodesource.com/setup_${NODE_VERSION}.x | bash - \
    && apt-get update \
    && apt-get install -y --no-install-recommends nodejs \
    && rm -rf /var/lib/apt/lists/*

# Install GitHub CLI
RUN curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg \
    && chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | tee /etc/apt/sources.list.d/github-cli.list > /dev/null \
    && apt-get update \
    && apt-get install -y --no-install-recommends gh \
    && rm -rf /var/lib/apt/lists/*

# Install Copilot CLI (global)
RUN npm install -g @githubnext/github-copilot-cli

# Default to vscode user (created in base image)
USER vscode
WORKDIR /workspaces/unraid-management-agent
