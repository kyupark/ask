#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SKILL_SRC="$ROOT_DIR/skills/webai-cli"
SKILL_DST="$HOME/.openclaw/workspace/skills/webai-cli"

if ! command -v go >/dev/null 2>&1; then
  echo "Go is required. Install it first (e.g. brew install go)."
  exit 1
fi

echo "Installing webai-cli..."
go install "$ROOT_DIR/cmd/webai-cli"

if [[ ":$PATH:" != *":$HOME/go/bin:"* ]]; then
  echo "Note: add Go bin to PATH if needed:"
  echo "  export PATH=\"\$HOME/go/bin:\$PATH\""
fi

echo "Installing OpenClaw skill..."
mkdir -p "$HOME/.openclaw/workspace/skills"
rm -rf "$SKILL_DST"
cp -R "$SKILL_SRC" "$SKILL_DST"

echo
echo "Done."
echo "CLI check:"
echo "  webai-cli --help"
echo "Skill check:"
echo "  openclaw skills info webai-cli"
