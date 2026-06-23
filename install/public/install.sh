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

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "unsupported architecture: $ARCH"; exit 1 ;;
esac

case "$OS" in
  linux|darwin) ;;
  *) echo "unsupported OS: $OS"; exit 1 ;;
esac

ASSET="${BINARY}-${OS}-${ARCH}"

echo "detecting platform... ${OS}/${ARCH}"

LATEST=$(curl -sSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | cut -d'"' -f4)

if [ -z "$LATEST" ]; then
  echo "error: could not fetch latest release"
  exit 1
fi

echo "creating paystable directory..."
mkdir -p paystable
cd paystable

echo "downloading ${BINARY} ${LATEST}..."
URL="https://github.com/${REPO}/releases/download/${LATEST}/${ASSET}"
curl -sSL "$URL" -o "${BINARY}"
chmod +x "${BINARY}"

echo "fetching .env.example..."
curl -sSL "https://raw.githubusercontent.com/${REPO}/${LATEST}/.env.example" -o .env.example
cp .env.example .env

echo "generating instructions.md..."
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

echo "--------------------------------------------------------"
echo "Paystable ${LATEST} has been successfully installed!"
echo "A directory named 'paystable' was created in your current path."
echo "To get started:"
echo "  1. cd paystable"
echo "  2. Open instructions.md for quick setup instructions."
echo "--------------------------------------------------------"
