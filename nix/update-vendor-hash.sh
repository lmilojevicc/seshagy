#!/usr/bin/env bash
# Recompute vendorHash in nix/package.nix after go.mod/go.sum change.
#
# Nix verifies the vendored Go modules against a fixed hash, so a dependency
# bump fails the Nix build until the hash is updated. This builds with a fake
# hash, reads the real one from Nix's mismatch error, and writes it back.
# It changes nothing when the hash is already correct.
set -euo pipefail

cd "$(dirname "$0")/.."
file=nix/package.nix
fake="sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="

current=$(sed -nE 's/^[[:space:]]*vendorHash = "([^"]+)";.*/\1/p' "$file")
if [ -z "$current" ]; then
	echo "no vendorHash found in $file" >&2
	exit 1
fi

replace() {
	local tmp
	tmp=$(mktemp)
	sed "s|vendorHash = \"$1\";|vendorHash = \"$2\";|" "$file" >"$tmp"
	cat "$tmp" >"$file"
	rm -f "$tmp"
}

replace "$current" "$fake"
log=$(nix build --no-link --no-update-lock-file .#seshagy 2>&1) || true
got=$(printf '%s\n' "$log" | sed -nE 's/^[[:space:]]*got:[[:space:]]*(sha256-[A-Za-z0-9+/=]+).*/\1/p' | head -n 1)

if [ -z "$got" ]; then
	replace "$fake" "$current"
	echo "nix build did not report a vendorHash mismatch; output tail:" >&2
	printf '%s\n' "$log" | tail -n 30 >&2
	exit 1
fi

replace "$fake" "$got"
if [ "$got" = "$current" ]; then
	echo "vendorHash is current: $got"
else
	echo "vendorHash: $current -> $got"
fi
