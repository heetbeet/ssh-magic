#!/usr/bin/env bash
# Iroh SSH 0.2.12 publishes only an x86_64 macOS binary. This builds the arm64
# binary from the same pinned tag and Cargo.lock. Path remapping, and linking
# without debug info so the Mach-O UUID does not hash temporary object paths,
# let anyone with the same toolchain reproduce the hash pinned in runtime.go.
# --publish uploads it to the iroh-ssh-0.2.12 release that release.ps1 downloads.
set -euo pipefail
cd -- "$(dirname -- "$0")/.."
tag=0.2.12 commit=c799cfd1f9ef7ba2b199c46f311206415b681f38 toolchain=1.99.0 target=aarch64-apple-darwin
src=${IROH_SOURCE:-docs/temp/iroh-source}
[[ -d $src ]] || git clone --depth 1 --branch "$tag" https://github.com/rustonbsd/iroh-ssh.git "$src"
[[ $(git -C "$src" rev-parse HEAD) == "$commit" ]] || { echo "Unexpected Iroh SSH revision in $src" >&2; exit 1; }
[[ -z $(git -C "$src" status --porcelain) ]] || { echo "Iroh SSH source in $src is modified" >&2; exit 1; }
src=$(cd -- "$src" && pwd)
rustup toolchain install "$toolchain" --profile minimal --target "$target"
export CARGO_HOME=${CARGO_HOME:-$HOME/.cargo}
# Flags are separated by 0x1f rather than spaces, so paths may contain spaces.
flags=("--remap-path-prefix=$src=/iroh-ssh" "--remap-path-prefix=$CARGO_HOME=/cargo" "--remap-path-prefix=$(rustc +"$toolchain" --print sysroot)=/rustc" -Clink-arg=-Wl,-S -Clink-arg=-Wl,-reproducible)
CARGO_ENCODED_RUSTFLAGS=$(IFS=$'\x1f'; echo "${flags[*]}")
export CARGO_ENCODED_RUSTFLAGS
unset RUSTFLAGS
cargo +"$toolchain" build --locked --release --target "$target" --manifest-path "$src/Cargo.toml" --target-dir "$src/target"
mkdir -p dist
cp "$src/target/$target/release/iroh-ssh" dist/iroh-ssh-darwin-arm64
hash=$(shasum -a 256 dist/iroh-ssh-darwin-arm64 | cut -d' ' -f1)
echo "dist/iroh-ssh-darwin-arm64 $hash"
pinned=$(sed -n 's/^const irohDarwinARM64 = "\([0-9a-f]*\)"$/\1/p' runtime.go)
[[ $hash == "$pinned" ]] || { echo "Does not match irohDarwinARM64 in runtime.go ($pinned)" >&2; exit 1; }
if [[ ${1:-} == --publish ]]; then
 notes="Unmodified Iroh SSH $tag ($commit) for Apple Silicon, built with Rust $toolchain by scripts/build-iroh-macos.sh, because upstream publishes only an x86_64 macOS binary. MIT. SHA256: $hash"
 gh release create "iroh-ssh-$tag" dist/iroh-ssh-darwin-arm64 --title "Iroh SSH $tag for Apple Silicon" --notes "$notes" --prerelease --latest=false
fi
