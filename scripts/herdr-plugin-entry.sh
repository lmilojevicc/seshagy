#!/usr/bin/env bash
set -euo pipefail

# Gate: check if seshagy is installed before launching.
# herdr plugin install clones this repo but does NOT install the binary.
# The user must install seshagy separately via Homebrew or go install.
if ! command -v seshagy >/dev/null 2>&1; then
  echo "seshagy is not installed on your PATH."
  echo ""
  echo "Install with Homebrew:"
  echo "  brew tap lmilojevicc/tap && brew install seshagy"
  echo ""
  echo "Or with Go:"
  echo "  go install github.com/lmilojevicc/seshagy/cmd/seshagy@latest"
  echo ""
  echo "After installing, reopen this pane."
  # Keep the pane open so the user can read the message.
  read -rp "Press Enter to close..."
  exit 1
fi

# Launch seshagy with built-in --ephemeral focus-loss dismissal: the overlay
# closes on command exit, and --ephemeral additionally dismisses when focus
# leaves the pane/workspace mid-session.
exec seshagy --ephemeral
