#!/bin/sh
# Installed by ssh-magic-macos.pkg; /usr/local/bin/ssh-magic links here.
dir=/usr/local/lib/ssh-magic
arch=amd64
if [ "$(uname -m)" = arm64 ] || [ "$(sysctl -n sysctl.proc_translated 2>/dev/null)" = 1 ]; then arch=arm64; fi
exec "$dir/$arch/ssh-magic" "$@"
