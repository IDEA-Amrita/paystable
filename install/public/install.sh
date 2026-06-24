#!/bin/sh
set -e

# this script installs paystable on your machine
#
# usage:
#   curl -sSL https://paystable.vercel.app | sh
#
# it creates a directory 'paystable' in the current working directory,
# downloads the latest compiled binary, fetches .env.example, and
# generates a quickstart instructions.md file.
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
LATEST=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | cut -d'"' -f4)

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

info "fetching .env.example"
curl -fsSL "https://raw.githubusercontent.com/${REPO}/${LATEST}/.env.example" -o .env.example
cp .env.example .env

info "writing instructions.md"
cat << 'EOF' > instructions.md
# Paystable Quick Start Guide

Welcome to Paystable! You have successfully installed the binary.

## Quick Start Steps

1. **Configure Environment Variables**:
   Open the `.env` file and fill in the required variables (especially `DATABASE_URL` for PostgreSQL):
   ```bash
   nano .env
   ```

2. **Run Paystable**:
   ```bash
   ./paystable
   ```
   *Note: Paystable will automatically run database migrations on startup.*

3. **Access the Ops Dashboard**:
   Once started, open your browser and navigate to:
   `http://localhost:8080/dashboard`

## Deployment & Production

- To install the binary globally (so you can run `paystable` from anywhere):
  ```bash
  sudo mv paystable /usr/local/bin/
  ```
- For details on setting up systemd services, Prometheus metrics, and production deployment, refer to the official documentation.

## Documentation
For in-depth integration workflows, callback contracts, and configuration options, visit:
https://github.com/IDEA-Amrita/paystable
EOF

info "paystable ${LATEST} installed successfully"
info "next step: cd paystable && edit .env"
info "then run: ./paystable"
