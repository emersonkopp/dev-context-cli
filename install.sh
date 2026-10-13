#!/usr/bin/env bash
# install.sh — installs dctx and bootstraps the dev-context monorepo.
#
# Usage (one-liner — does everything):
#   curl -fsSL https://raw.githubusercontent.com/emersonkopp/dev-context-cli/main/install.sh | bash
#
# Environment variables:
#   DCTX_VERSION   install a specific version (default: latest)
#   INSTALL_DIR    where to put the binary (default: ~/.local/bin — no sudo needed)
#   REPO_PATH      local path for the monorepo clone (default: ~/git/dev-context)
#   SKIP_BOOTSTRAP set to "1" to install the binary only, skip bootstrap

set -euo pipefail

REPO="emersonkopp/dev-context-cli"
BINARY="dctx"
# Default to a user-writable directory so install, use and self-update never
# require sudo. A root-owned location like /usr/local/bin forces 'sudo dctx
# update', which fails for a normal user and can leave a corrupted binary.
# Callers can still override INSTALL_DIR explicitly (e.g. a system-wide install).
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
SKIP_BOOTSTRAP="${SKIP_BOOTSTRAP:-0}"

# ── colour helpers ────────────────────────────────────────────────────────────
red()    { printf '\033[0;31m%s\033[0m\n' "$*"; }
green()  { printf '\033[0;32m%s\033[0m\n' "$*"; }
yellow() { printf '\033[0;33m%s\033[0m\n' "$*"; }
cyan()   { printf '\033[0;36m%s\033[0m\n' "$*"; }

# ── detect OS ────────────────────────────────────────────────────────────────
detect_os() {
  case "$(uname -s)" in
    Linux*)  echo "linux" ;;
    # Release assets name macOS "macos" (see .goreleaser.yaml) so that the
    # self-update matcher in dctx <= 1.3.0 can find them. Keep this in sync.
    Darwin*) echo "macos" ;;
    *)
      red "Unsupported OS: $(uname -s)"
      exit 1
      ;;
  esac
}

# ── detect architecture ───────────────────────────────────────────────────────
detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64)  echo "amd64" ;;
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

# ── download helper ───────────────────────────────────────────────────────────
download() {
  local url="$1" dest="$2"
  if command -v curl &>/dev/null; then
    curl -fsSL "$url" -o "$dest"
  else
    wget -qO "$dest" "$url"
  fi
}

# ── install the binary ────────────────────────────────────────────────────────
install_binary() {
  local os="$1" arch="$2" version="$3"

  # GoReleaser strips the leading 'v' from the archive filename.
  # e.g. tag v1.1.1 → dctx_1.1.1_linux_amd64.tar.gz
  local ver_bare="${version#v}"

  local archive="${BINARY}_${ver_bare}_${os}_${arch}.tar.gz"
  local url="https://github.com/${REPO}/releases/download/${version}/${archive}"

  cyan "Downloading ${url}"

  # Declare tmp before the trap so it is always bound under set -u.
  local tmp=""
  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' EXIT

  download "$url" "${tmp}/${archive}"
  tar -xzf "${tmp}/${archive}" -C "$tmp"

  if [[ ! -d "$INSTALL_DIR" ]]; then
    mkdir -p "$INSTALL_DIR"
  fi

  if [[ -w "$INSTALL_DIR" ]]; then
    mv "${tmp}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
    chmod +x "${INSTALL_DIR}/${BINARY}"
  else
    # INSTALL_DIR is not writable by the current user. Installing here with
    # sudo would leave a root-owned binary that 'dctx update' cannot replace
    # without sudo — exactly the problem we want to avoid. Warn and suggest a
    # user-writable location instead of silently escalating.
    yellow ""
    yellow "  ⚠  ${INSTALL_DIR} is not writable by your user."
    yellow "     Installing there needs sudo and would make 'dctx update' require"
    yellow "     sudo too (and risk a root-owned, hard-to-fix binary)."
    yellow ""
    yellow "     Recommended: install into a user-writable directory, e.g.:"
    yellow "       INSTALL_DIR=\"\$HOME/.local/bin\" curl -fsSL .../install.sh | bash"
    yellow ""
    yellow "     Proceeding with sudo for this location…"
    sudo mv "${tmp}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
    sudo chmod +x "${INSTALL_DIR}/${BINARY}"
  fi
}

# ── ensure INSTALL_DIR is on PATH ─────────────────────────────────────────────
check_path() {
  # Resolve the binary on PATH to its real location; if it is not the one we
  # just installed, INSTALL_DIR is probably missing from PATH.
  local on_path=""
  on_path="$(command -v "$BINARY" 2>/dev/null || true)"

  if [[ "$on_path" != "${INSTALL_DIR}/${BINARY}" ]]; then
    yellow ""
    yellow "  ${INSTALL_DIR} is not on your \$PATH (or another ${BINARY} shadows it)."
    yellow "  Add it to your shell profile (~/.zshrc, ~/.bashrc, etc.):"
    yellow "    export PATH=\"${INSTALL_DIR}:\$PATH\""
    yellow ""
    # Export for the current script session so bootstrap can run.
    export PATH="${INSTALL_DIR}:${PATH}"
  fi
}

# ── main ──────────────────────────────────────────────────────────────────────
main() {
  local os arch version

  os=$(detect_os)
  arch=$(detect_arch)

  version="${DCTX_VERSION:-}"
  if [[ -z "$version" ]]; then
    cyan "▸ Fetching latest release…"
    version=$(latest_version)
    if [[ -z "$version" ]]; then
      red "Could not determine the latest version. Set DCTX_VERSION and retry."
      exit 1
    fi
  fi

  cyan "▸ Installing dctx ${version} (${os}/${arch})…"
  install_binary "$os" "$arch" "$version"

  check_path

  local installed_ver
  installed_ver=$("${INSTALL_DIR}/${BINARY}" --version 2>&1 || echo "unknown")
  green "✓  dctx ${installed_ver} installed → ${INSTALL_DIR}/${BINARY}"

  # ── bootstrap ───────────────────────────────────────────────────────────────
  if [[ "$SKIP_BOOTSTRAP" == "1" ]]; then
    cyan ""
    cyan "Skipping bootstrap (SKIP_BOOTSTRAP=1)."
    cyan "Run 'dctx bootstrap' when ready."
    return 0
  fi

  echo ""
  cyan "▸ Running bootstrap…"

  # Build the bootstrap command. Pass --repo-path if the caller set REPO_PATH.
  local bootstrap_args=()
  if [[ -n "${REPO_PATH:-}" ]]; then
    bootstrap_args+=(--repo-path "$REPO_PATH")
  fi
  # When piping through bash (non-interactive), pass -y to skip prompts.
  # The user can always re-run 'dctx bootstrap' interactively afterward.
  if [[ ! -t 0 ]]; then
    bootstrap_args+=(--yes)
  fi

  "${INSTALL_DIR}/${BINARY}" bootstrap "${bootstrap_args[@]}"
}

main "$@"
