#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if ! command -v go >/dev/null 2>&1; then
  echo "Go is required. Install it first (e.g. brew install go)."
  exit 1
fi

echo "Installing ask..."
go install "$ROOT_DIR/cmd/ask"

if [[ ":$PATH:" != *":$HOME/go/bin:"* ]]; then
  echo "Note: add Go bin to PATH if needed:"
  echo "  export PATH=\"\$HOME/go/bin:\$PATH\""
fi

echo "Installing OpenClaw skill..."
"$HOME/go/bin/ask" install-openclaw-skill

echo
echo "Done."
echo "CLI check:"
echo "  ask --help"
echo "Skill check:"
echo "  openclaw skills info ask"
