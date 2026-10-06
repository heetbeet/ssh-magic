#!/usr/bin/env bash
# Define the whole bootstrap before running it, so a truncated download cannot install.
main() {
set -euo pipefail
[[ $(uname -s) == Linux && $(uname -m) == x86_64 ]] || { echo 'This release supports Linux x64.' >&2; exit 1; }
for tool in curl sha256sum flock; do command -v "$tool" >/dev/null || { echo "Install $tool first." >&2; exit 1; }; done
umask 077
version=0.1.3
base="https://github.com/heetbeet/ssh-wormhole/releases/download/v$version"
root="${WH_HOME:-${XDG_CACHE_HOME:-$HOME/.cache}/ssh-wormhole}"
dir="$root/bin/$version"
mkdir -p "$dir"
chmod 700 "$root" "$dir"
exec 9>"$root/install.lock"
flock -w 120 9 || { echo 'Another download is still running. Retry shortly.' >&2; exit 1; }
fetch() {
 local dest="$dir/$1" asset="$2" hash="$3" tmp
 if [[ -f "$dest" ]] && echo "$hash  $dest" | sha256sum -c --status; then return; fi
 tmp=$(mktemp "$dir/.download.XXXXXX")
 if ! curl --fail --location --retry 3 --connect-timeout 15 --max-time 180 "$base/$asset" -o "$tmp"; then rm -f "$tmp"; return 1; fi
 if ! echo "$hash  $tmp" | sha256sum -c --status; then rm -f "$tmp"; echo "Checksum mismatch: $asset" >&2; return 1; fi
 chmod 700 "$tmp"
 mv -f "$tmp" "$dest"
}
fetch wh wh-linux-amd64 '835a13fda8725c41e2f8a99132d90ddda7a2d5c5c953c3dd4d8fb61a7b2a47bb'
fetch iroh-ssh iroh-ssh-linux-amd64 '6e39a6b22f14d683598600350ca642d3b67fd0b0a2f2a7b29928ad6cdf032826'
fetch licenses.zip licenses.zip '519b63cb5c6dee55eea1d4b32359008daf6d4735d2bfa1f0e36dcae3dc3972fb'
flock -u 9
exec 9>&-
if [[ $# == 0 ]]; then set -- open; fi
if [[ ! -t 0 && -t 1 ]]; then exec "$dir/wh" "$@" < /dev/tty; fi
exec "$dir/wh" "$@"
}
main "$@"
