#!/bin/sh
# dbman installer — downloads the latest release binary and puts it on PATH.
#
# One-line install:
#   curl -fsSL https://raw.githubusercontent.com/muizzyranking/dbman/main/install.sh | sh
#
# Env overrides:
#   DBMAN_VERSION=v0.1.0   install a specific version (default: latest)
#   DBMAN_DIR=$HOME/bin    install directory (default: $HOME/.local/bin)
set -eu

REPO="muizzyranking/dbman"
VERSION="${DBMAN_VERSION:-latest}"
DIR="${DBMAN_DIR:-$HOME/.local/bin}"

os() {
  case "$(uname -s)" in
    Linux)  echo "Linux" ;;
    Darwin) echo "Darwin" ;;
    *) echo "unsupported OS: $(uname -s) — grab a zip from https://github.com/$REPO/releases" >&2; exit 1 ;;
  esac
}

arch() {
  case "$(uname -m)" in
    x86_64|amd64)   echo "x86_64" ;;
    arm64|aarch64)  echo "arm64" ;;
    *) echo "unsupported arch: $(uname -m)" >&2; exit 1 ;;
  esac
}

latest_tag() {
  # no jq dependency: scrape tag_name from the GitHub API
  curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" |
    grep '"tag_name":' | head -1 | cut -d'"' -f4
}

main() {
  if [ "$VERSION" = "latest" ]; then
    VERSION="$(latest_tag)"
    [ -n "$VERSION" ] || { echo "could not determine latest release" >&2; exit 1; }
  fi
  OS="$(os)"
  ARCH="$(arch)"
  FILE="dbman_${OS}_${ARCH}.tar.gz"
  URL="https://github.com/$REPO/releases/download/$VERSION/$FILE"

  echo "installing dbman $VERSION ($OS/$ARCH) to $DIR"
  mkdir -p "$DIR"
  TMP="$(mktemp -d)"
  trap 'rm -rf "$TMP"' EXIT INT TERM
  curl -fsSL "$URL" -o "$TMP/$FILE"
  tar -xzf "$TMP/$FILE" -C "$TMP"
  mv "$TMP/dbman" "$DIR/dbman"
  chmod +x "$DIR/dbman"

  case ":$PATH:" in
    *":$DIR:"*) ;;
    *) echo "NOTE: $DIR is not on PATH. Add it, e.g.:"; echo "  export PATH=\"$DIR:\$PATH\"" ;;
  esac
  echo "done. Run: dbman --version"
}

main
