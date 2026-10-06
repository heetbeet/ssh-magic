#!/bin/sh
# Removes what ssh-magic-macos.pkg installed. Run 'ssh-magic remove' first to
# also erase your own cache and credentials.
set -eu
[ "$(id -u)" = 0 ] || exec sudo /bin/sh "$0" "$@"
if [ "$(readlink /usr/local/bin/ssh-magic 2>/dev/null)" = /usr/local/lib/ssh-magic/ssh-magic ]; then rm -f /usr/local/bin/ssh-magic; fi
rm -rf /usr/local/lib/ssh-magic
pkgutil --forget io.github.heetbeet.ssh-magic >/dev/null 2>&1 || true
echo 'SSH Magic removed.'
