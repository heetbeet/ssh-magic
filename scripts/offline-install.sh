#!/usr/bin/env bash
set -euo pipefail
dir=$(cd -- "$(dirname -- "$0")" && pwd)
chmod 700 "$dir/ssh-magic" "$dir/iroh-ssh"
exec "$dir/ssh-magic" install
