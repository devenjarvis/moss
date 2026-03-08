#!/bin/sh
set -e

REPO="devenjarvis/moss"
INSTALL_DIR="/usr/local/bin"
BINARY="moss"

# Detect OS
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
    linux) OS="linux" ;;
    darwin) OS="darwin" ;;
    *) echo "Unsupported OS: $OS" >&2; exit 1 ;;
esac

# Detect architecture
ARCH="$(uname -m)"
case "$ARCH" in
    x86_64|amd64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

# Get latest version if not specified
VERSION="${MOSS_VERSION:-}"
if [ -z "$VERSION" ]; then
    VERSION="$(curl -sSf https://api.github.com/repos/${REPO}/releases/latest | grep '"tag_name"' | sed -E 's/.*"v([^"]+)".*/\1/')"
    if [ -z "$VERSION" ]; then
        echo "Error: could not determine latest version." >&2
        echo "Set MOSS_VERSION to install a specific version." >&2
        exit 1
    fi
fi

# Construct download URL
ARCHIVE="moss_${VERSION}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/v${VERSION}/${ARCHIVE}"

# Fallback install dir if /usr/local/bin is not writable
if [ ! -w "$INSTALL_DIR" ]; then
    INSTALL_DIR="${HOME}/.local/bin"
    mkdir -p "$INSTALL_DIR"
fi

# Download and install
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

echo "Downloading moss v${VERSION} for ${OS}/${ARCH}..."
curl -sSfL "$URL" -o "${TMPDIR}/${ARCHIVE}"
tar -xzf "${TMPDIR}/${ARCHIVE}" -C "$TMPDIR"
install -m 755 "${TMPDIR}/moss" "${INSTALL_DIR}/moss"

echo "Installed moss to ${INSTALL_DIR}/moss"

# Check if install dir is in PATH
case ":$PATH:" in
    *":${INSTALL_DIR}:"*) ;;
    *)
        echo ""
        echo "NOTE: ${INSTALL_DIR} is not in your PATH. Add it with:"
        echo "  export PATH=\"${INSTALL_DIR}:\$PATH\""
        ;;
esac
