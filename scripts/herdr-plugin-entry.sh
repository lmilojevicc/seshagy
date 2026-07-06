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

# Under herdr, the overlay auto-restores focus when the command exits,
# so seshagy-focus-kill is unnecessary here — seshagy exits after session
# switch and the overlay closes cleanly.
exec seshagy
