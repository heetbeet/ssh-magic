# Third-party software

SSH Wormhole embeds sshd-lite 1.55.4 (Jay Pillora, MIT), wormhole-william 1.0.8 (Peter Sanford, MIT), pkg/sftp 1.13.10 (pkg/sftp contributors, BSD-2-Clause), gofrs/flock 0.13.0 (gofrs contributors, BSD-3-Clause), and Go x/crypto, x/sys, and x/term (The Go Authors, BSD-3-Clause). Exact direct and transitive module versions are in go.mod and go.sum.

The separately distributed Iroh SSH binaries are unmodified release 0.2.12 from [rustonbsd/iroh-ssh](https://github.com/rustonbsd/iroh-ssh/tree/0.2.12), MIT. Windows SHA256: `d50813aea4425c2113edcb889ffcc1a97f5a0d85517a35ef56114de45d8bda64`. Linux SHA256: `6e39a6b22f14d683598600350ca642d3b67fd0b0a2f2a7b29928ad6cdf032826`.

The release's `licenses.zip` includes the license files from the Go module dependencies and Iroh SSH's pinned source, including the bundled Go runtime license. The Iroh SSH Cargo.lock and complete source are available at the pinned source link above; Iroh itself uses MIT/Apache-2.0. Bootstrap downloads trust GitHub HTTPS and the hash values embedded in the pinned bootstrap, then fail closed on a hash mismatch.
