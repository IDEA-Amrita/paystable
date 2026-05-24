#!/bin/sh
set -e

REPO="IDEA-Amrita/paystable"
BINARY="paystable"
INSTALL_DIR="/usr/local/bin"

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

echo "downloading ${BINARY} ${LATEST}..."

URL="https://github.com/${REPO}/releases/download/${LATEST}/${ASSET}"

if [ -w "$INSTALL_DIR" ]; then
  curl -sSL "$URL" -o "${INSTALL_DIR}/${BINARY}"
  chmod +x "${INSTALL_DIR}/${BINARY}"
else
  sudo curl -sSL "$URL" -o "${INSTALL_DIR}/${BINARY}"
  sudo chmod +x "${INSTALL_DIR}/${BINARY}"
fi

echo "installed ${BINARY} ${LATEST} to ${INSTALL_DIR}/${BINARY}"
