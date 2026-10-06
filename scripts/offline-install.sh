#!/usr/bin/env bash
set -euo pipefail
dir=$(cd -- "$(dirname -- "$0")" && pwd)
chmod 700 "$dir/ssh-magic" "$dir/iroh-ssh"
# Browsers and Archive Utility quarantine extracted files; Gatekeeper would refuse the unsigned binaries.
if [[ $(uname -s) == Darwin ]]; then xattr -d com.apple.quarantine "$dir/ssh-magic" "$dir/iroh-ssh" 2>/dev/null || true; fi
exec "$dir/ssh-magic" install
