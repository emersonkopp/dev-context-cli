#!/usr/bin/env bash
# install.sh — installs the latest dctx binary for the current OS and architecture.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/emersonkopp/dev-context-cli/main/install.sh | bash
#
# Or, to install a specific version:
#   DCTX_VERSION=v1.2.0 bash install.sh

set -euo pipefail

REPO="emersonkopp/dev-context-cli"
BINARY="dctx"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

# ── colour helpers ────────────────────────────────────────────────────────────
red()    { printf '\033[0;31m%s\033[0m\n' "$*"; }
green()  { printf '\033[0;32m%s\033[0m\n' "$*"; }
yellow() { printf '\033[0;33m%s\033[0m\n' "$*"; }
cyan()   { printf '\033[0;36m%s\033[0m\n' "$*"; }

# ── detect OS ────────────────────────────────────────────────────────────────
detect_os() {
  case "$(uname -s)" in
    Linux*)  echo "linux"  ;;
    Darwin*) echo "macos"  ;;
    *)
      red "Unsupported OS: $(uname -s)"
      exit 1
      ;;
  esac
}

# ── detect architecture ───────────────────────────────────────────────────────
detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *)
      red "Unsupported architecture: $(uname -m)"
      exit 1
      ;;
  esac
}

# ── fetch latest version from GitHub API ─────────────────────────────────────
latest_version() {
  if command -v curl &>/dev/null; then
    curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
      | grep '"tag_name"' \
      | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/'
  elif command -v wget &>/dev/null; then
    wget -qO- "https://api.github.com/repos/${REPO}/releases/latest" \
      | grep '"tag_name"' \
      | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/'
  else
    red "curl or wget is required but neither was found."
    exit 1
  fi
}

# ── main ──────────────────────────────────────────────────────────────────────
main() {
  OS=$(detect_os)
  ARCH=$(detect_arch)

  VERSION="${DCTX_VERSION:-}"
  if [[ -z "$VERSION" ]]; then
    cyan "Fetching latest release version…"
    VERSION=$(latest_version)
    if [[ -z "$VERSION" ]]; then
      red "Could not determine the latest version. Set DCTX_VERSION and retry."
      exit 1
    fi
  fi

  # Strip leading 'v' for the filename (goreleaser uses bare version in names).
  VER_BARE="${VERSION#v}"

  ARCHIVE_NAME="${BINARY}_${VERSION}_${OS}_${ARCH}.tar.gz"
  DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${ARCHIVE_NAME}"

  cyan "Installing dctx ${VERSION} (${OS}/${ARCH})…"
  cyan "Downloading ${DOWNLOAD_URL}"

  TMP_DIR=$(mktemp -d)
  trap 'rm -rf "$TMP_DIR"' EXIT

  if command -v curl &>/dev/null; then
    curl -fsSL "$DOWNLOAD_URL" -o "${TMP_DIR}/${ARCHIVE_NAME}"
  else
    wget -qO "${TMP_DIR}/${ARCHIVE_NAME}" "$DOWNLOAD_URL"
  fi

  tar -xzf "${TMP_DIR}/${ARCHIVE_NAME}" -C "$TMP_DIR"

  # Make sure the install directory exists and is writable.
  if [[ ! -d "$INSTALL_DIR" ]]; then
    mkdir -p "$INSTALL_DIR"
  fi

  if [[ -w "$INSTALL_DIR" ]]; then
    mv "${TMP_DIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
  else
    yellow "INSTALL_DIR (${INSTALL_DIR}) is not writable — trying with sudo…"
    sudo mv "${TMP_DIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
  fi

  chmod +x "${INSTALL_DIR}/${BINARY}"

  # Verify.
  if command -v "${BINARY}" &>/dev/null; then
    INSTALLED_VER=$("${BINARY}" --version 2>&1 || true)
    green "✓  dctx installed successfully!"
    green "   Version: ${INSTALLED_VER}"
    green "   Path:    $(command -v "${BINARY}")"
  else
    yellow "Binary installed to ${INSTALL_DIR}/${BINARY}."
    yellow "Make sure ${INSTALL_DIR} is in your \$PATH."
  fi

  echo ""
  cyan "Next steps:"
  echo "  1. dctx config init      # point dctx at your monorepo"
  echo "  2. dctx install          # install all AI artifacts"
  echo "  3. dctx status           # verify everything is up-to-date"
}

main "$@"
