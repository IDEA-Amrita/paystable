#!/bin/sh
  set -e
  # this script installs paystable on your machine
  #
  # usage:
  #   curl -sSL https://paystable.vercel.app | sh
  #
  # it creates a directory 'paystable' in the current working directory,
  # downloads the latest compiled binary, runs `paystable init` to write
  # a local .env with generated secrets, and writes instructions.md.
  #
  # supported platforms:
  #   - linux/amd64
  #   - linux/arm64
  #   - darwin/amd64 (macOS intel)
  #   - darwin/arm64 (macOS apple silicon)
  #
  # source: https://github.com/IDEA-Amrita/paystable
  #

  REPO="IDEA-Amrita/paystable"
  BINARY="paystable"

  info() {
    echo "[INFO] $*"
  }

  error() {
    echo "[ERROR] $*" >&2
  }

  OS=$(uname -s | tr '[:upper:]' '[:lower:]')
  ARCH=$(uname -m)

  case "$ARCH" in
    x86_64|amd64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) error "unsupported architecture: $ARCH"; exit 1 ;;
  esac
  
  case "$OS" in
    linux|darwin) ;;
    *) error "unsupported OS: $OS"; exit 1 ;;
  esac
  
  ASSET="${BINARY}-${OS}-${ARCH}"
  
  info "starting paystable installation"
  info "detected platform: ${OS}/${ARCH}"
  
  info "fetching latest release metadata"
  LATEST=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" |
  grep '"tag_name"' | cut -d'"' -f4)
  if [ -z "$LATEST" ]; then
    error "could not fetch latest release"
    exit 1
  fi
  
  info "latest release: ${LATEST}"
  info "creating paystable directory"
  mkdir -p paystable
  cd paystable
  
  info "downloading ${ASSET}"
  URL="https://github.com/${REPO}/releases/download/${LATEST}/${ASSET}"
  curl -fsSL "$URL" -o "${BINARY}"
  
  info "downloading checksums"
  curl -fsSL "https://github.com/${REPO}/releases/download/${LATEST}/checksums.txt" -o checksums.txt
  
  EXPECTED=$(grep " ${ASSET}$" checksums.txt | awk '{print $1}')
  if [ -z "$EXPECTED" ]; then
    error "checksum for ${ASSET} was not found"
    exit 1
  fi
  
  info "verifying checksum"
  if command -v sha256sum >/dev/null 2>&1; then
    echo "${EXPECTED}  ${BINARY}" | sha256sum -c - >/dev/null
  elif command -v shasum >/dev/null 2>&1; then
    echo "${EXPECTED}  ${BINARY}" | shasum -a 256 -c - >/dev/null
  else
    error "sha256sum or shasum is required to verify the download"
    exit 1
  fi
  
  info "marking binary executable"
  chmod +x "${BINARY}"

  info "generating local .env"
  ./"${BINARY}" init


  info "writing instructions.md"
  cat << 'EOF' > instructions.md
  # Paystable Quick Start Guide
  
  Welcome to Paystable! You have successfully installed the binary.
  A local `.env` was created with generated secrets via `./paystable init`.
  ## Quick Start Steps
  
  1. **Set up Postgres**:
     Create a local user and database (password must match `DATABASE_URL` in
  `.env`):
     ```sql
     CREATE USER paystable WITH PASSWORD 'CHANGE_ME';
     CREATE DATABASE paystable OWNER paystable;
     ```
     Then edit `.env` if your password, host, or database name differ:
     ```bash
     nano .env
     ```
    If doctor reports peer/ident or other Postgres errors, run `./paystable
    doctor` and follow the exact next command it prints.
  
  2. **Set PayU gateway credentials**:
     In `.env`, fill in the TODO fields from your PayU dashboard:
     - `GATEWAY_API_KEY`
     - `PAYU_STATUS_URL`
     Local secrets (`WEBHOOK_SECRET`, `MERCHANT_CALLBACK_SECRET`,
  `ADMIN_API_KEY`,
     `SECRET_ENCRYPTION_KEY`) were generated for you. Replace `WEBHOOK_SECRET`
     with your real PayU salt when you integrate webhooks.
  
  3. **Check the setup**:
     ```bash
     ./paystable doctor
     ```
  
  4. **Run Paystable**:
     ```bash
     ./paystable
     ```
     *Note: Paystable will automatically run database migrations on startup.*

  5. **Access the Ops Dashboard**:
     Once started, open your browser and navigate to:
     `http://localhost:8080/dashboard`

  ## Deployment & Production

  - To install the binary globally (so you can run `paystable` from anywhere):
    ```bash
    sudo mv paystable /usr/local/bin/
    ```
  - For details on setting up systemd services, Prometheus metrics, and
  production deployment, refer to the official documentation.
  ## Documentation
  For in-depth integration workflows, callback contracts, and configuration
  options, visit:
  https://github.com/IDEA-Amrita/paystable
EOF

info "paystable ${LATEST} installed successfully"
info "next step: cd paystable && configure Postgres + PayU fields in .env"
info "then run: ./paystable doctor"
info "then start: ./paystable"
