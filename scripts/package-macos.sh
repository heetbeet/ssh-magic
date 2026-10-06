#!/usr/bin/env bash
# Builds dist/ssh-magic-macos.pkg, one installer for Apple Silicon and Intel Macs.
# Run on a Mac. It packages the binaries release.ps1 left in dist/, or with
# --download fetches the published release assets and verifies them against
# SHA256SUMS. --publish uploads the package and adds it to SHA256SUMS.
set -euo pipefail
cd -- "$(dirname -- "$0")/.."
version=$(sed -n 's/^const version = "\(.*\)"$/\1/p' main.go)
id=io.github.heetbeet.ssh-magic
assets=(ssh-magic-darwin-arm64 ssh-magic-darwin-amd64 iroh-ssh-darwin-arm64 iroh-ssh-darwin-amd64 licenses.zip)
download=0 publish=0
for arg in "$@"; do
 case $arg in --download) download=1 ;; --publish) publish=1 ;; *) echo "usage: $0 [--download] [--publish]" >&2; exit 2 ;; esac
done
mkdir -p dist
if (( download )); then
 gh release download "v$version" -D dist --clobber -p SHA256SUMS $(printf -- '-p %s ' "${assets[@]}")
fi
if [[ -f dist/SHA256SUMS ]]; then
 (cd dist && for a in "${assets[@]}"; do awk -v n="$a" '$2 == n { print; found = 1 } END { exit !found }' SHA256SUMS || { echo "$a missing from SHA256SUMS" >&2; exit 1; }; done | shasum -a 256 -c)
elif (( download )); then
 echo 'Release has no SHA256SUMS' >&2; exit 1
fi
# Refuse binaries the program would reject at runtime.
for arch in arm64 amd64; do
 [[ $(file -b "dist/ssh-magic-darwin-$arch") == *Mach-O* ]] || { echo "dist/ssh-magic-darwin-$arch is not a macOS binary" >&2; exit 1; }
done
stage=docs/temp/pkg
rm -rf "$stage"
root=$stage/root
for arch in arm64 amd64; do
 mkdir -p "$root/$arch"
 cp "dist/ssh-magic-darwin-$arch" "$root/$arch/ssh-magic"
 cp "dist/iroh-ssh-darwin-$arch" "$root/$arch/iroh-ssh"
 # ssh-magic install copies licenses.zip from beside the executable.
 cp dist/licenses.zip "$root/$arch/"
 chmod 755 "$root/$arch/ssh-magic" "$root/$arch/iroh-ssh"
done
tr -d '\r' < scripts/macos/launcher.sh > "$root/ssh-magic"
tr -d '\r' < scripts/macos/uninstall.sh > "$root/uninstall.sh"
cp LICENSE THIRD_PARTY.md "$root/"
chmod 755 "$root/ssh-magic" "$root/uninstall.sh"
xattr -cr "$root"
mkdir -p "$stage/scripts" "$stage/resources"
# A CRLF checkout would break the shebang; Installer would then fail the install.
tr -d '\r' < scripts/macos/postinstall > "$stage/scripts/postinstall"
chmod 755 "$stage/scripts/postinstall"
cp LICENSE "$stage/resources/LICENSE.txt"
for page in welcome conclusion; do sed "s/@VERSION@/$version/g" "scripts/macos/$page.html" > "$stage/resources/$page.html"; done
COPYFILE_DISABLE=1 pkgbuild --root "$root" --install-location /usr/local/lib/ssh-magic --scripts "$stage/scripts" --identifier "$id" --version "$version" --ownership recommended "$stage/ssh-magic.pkg"
cat > "$stage/distribution.xml" <<EOF
<?xml version="1.0" encoding="utf-8"?>
<installer-gui-script minSpecVersion="2">
 <title>SSH Magic $version</title>
 <options customize="never" require-scripts="false" hostArchitectures="arm64,x86_64"/>
 <domains enable_anywhere="false" enable_currentUserHome="false" enable_localSystem="true"/>
 <volume-check><allowed-os-versions><os-version min="12.0"/></allowed-os-versions></volume-check>
 <welcome file="welcome.html" mime-type="text/html"/>
 <license file="LICENSE.txt" mime-type="text/plain"/>
 <conclusion file="conclusion.html" mime-type="text/html"/>
 <choices-outline><line choice="default"/></choices-outline>
 <choice id="default" title="SSH Magic"><pkg-ref id="$id"/></choice>
 <pkg-ref id="$id" version="$version" onConclusion="none">ssh-magic.pkg</pkg-ref>
</installer-gui-script>
EOF
productbuild --distribution "$stage/distribution.xml" --resources "$stage/resources" --package-path "$stage" dist/ssh-magic-macos.pkg
line="$(shasum -a 256 dist/ssh-magic-macos.pkg | cut -d' ' -f1)  ssh-magic-macos.pkg"
echo "$line"
if (( publish )); then
 [[ -f dist/SHA256SUMS ]] || gh release download "v$version" -D dist -p SHA256SUMS
 grep -v '  ssh-magic-macos.pkg$' dist/SHA256SUMS > "$stage/SHA256SUMS" || true
 echo "$line" >> "$stage/SHA256SUMS"
 cp "$stage/SHA256SUMS" dist/SHA256SUMS
 gh release upload "v$version" dist/ssh-magic-macos.pkg dist/SHA256SUMS --clobber
fi
