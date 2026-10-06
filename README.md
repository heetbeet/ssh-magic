# SSH Magic

Temporary SSH access for helping someone with their computer. They run one command, read back a four-word code, and keep the terminal open. You or your agent redeem the code, run commands, transfer files, and close the session when finished.

Windows 10/11 x64 and Linux x64. No account registration or router configuration. Access runs in the foreground. Downloads are cached for repeat use; adding the command to PATH is optional.

## Open the computer being helped

Windows, from PowerShell, cmd, or the Run dialog:

```powershell
powershell -nop -c "irm https://heetbeet.github.io/ssh-magic/boot.ps1|iex"
```

Linux:

```bash
curl -fsSL https://heetbeet.github.io/ssh-magic/boot.sh | bash
```

Read back the line starting with `CODE:`. Keep that terminal open. Only share the code with the person or agent you want to give access to.

## Copy-and-paste commands

Type these directly in PowerShell. Replace `CODE` with the received code.

| What you want | PowerShell command |
| --- | --- |
| Open this computer, user access | `irm https://heetbeet.github.io/ssh-magic/boot.ps1\|iex` |
| Open this computer, admin access | `iex ('&{'+(irm https://heetbeet.github.io/ssh-magic/boot.ps1)+'} open --admin')` |
| Connect to their computer | `iex ('&{'+(irm https://heetbeet.github.io/ssh-magic/boot.ps1)+'} connect CODE')` |
| Install the command on your user PATH | `iex ('&{'+(irm https://heetbeet.github.io/ssh-magic/boot.ps1)+'} install')` |

From cmd or the Run dialog, wrap a command as `powershell -nop -c "COMMAND"`, as in the opening example. No temp-path expansion, separate .NET installation, or permanent execution-policy change is needed. Windows PowerShell already includes its .NET runtime.

| What you want | Linux command |
| --- | --- |
| Open this computer, user access | `curl -fsSL https://heetbeet.github.io/ssh-magic/boot.sh \| bash` |
| Open this computer, admin access | `curl -fsSL https://heetbeet.github.io/ssh-magic/boot.sh \| bash -s -- open --admin` |
| Connect to their computer | `curl -fsSL https://heetbeet.github.io/ssh-magic/boot.sh \| bash -s -- connect CODE` |
| Install the command on your user PATH | `curl -fsSL https://heetbeet.github.io/ssh-magic/boot.sh \| bash -s -- install` |

Repeat the same command to retry. Windows admin access asks for UAC approval and displays the code in the elevated terminal. Linux asks through sudo. An already elevated terminal must use `open --admin` explicitly. User access runs with the opening user's permissions and reaches their accessible files; it is not a filesystem sandbox.

## Use a connection

The connector prints the absolute path of `ssh-magic` and a session-specific SSH configuration. Give those paths to your agent. No installed SSH client is needed for the built-in commands. Run the Windows executable with PowerShell's `&` operator if its path is quoted.

```text
ssh-magic exec help -- "remote command"
ssh-magic put help local-file remote-file
ssh-magic get help remote-file local-file
ssh-magic close help
```

Without a PATH installation, substitute the absolute executable path printed by `connect`. In PowerShell, put `&` before a quoted executable path.

Windows hosts execute commands with Windows PowerShell, `-NoProfile -NonInteractive -Command`. Linux hosts use Bash, `-lc`. stdout, stderr, stdin, and the remote exit status pass through. Each exec starts a fresh shell in the host user's home directory. To keep a working directory, include `cd` in the command. Paths for file transfers may contain spaces, Unicode, Windows drive letters, or Unix absolute paths. Transfers write a temporary sibling file and rename only after completing; a remote server without atomic overwrite support may reject an existing destination instead of replacing it.

The built-in client checks SSH liveness every fifteen seconds, allowing ten seconds for a reply. A lost host therefore fails the current operation within about twenty-five seconds. It does not automatically retry remote commands, which might have already changed something.

For an existing SSH client:

```text
ssh -F /absolute/path/printed/ssh_config help "remote command"
sftp -F /absolute/path/printed/ssh_config help
scp -F /absolute/path/printed/ssh_config local-file help:remote-file
```

| Current position | What to run |
| --- | --- |
| Computer being helped, ordinary permissions | Bootstrap with `open` |
| Computer being helped, administrator permissions | Bootstrap with `open --admin` |
| Operator or agent computer | Bootstrap with `connect CODE` |
| Same computer retrying a download or waiting session | Repeat the same command |
| Operator after pairing succeeded but SSH failed | Repeat `connect CODE` on that same computer |
| Different operator computer after code was consumed | Close the host session, then open a new one |
| Assistance finished | Operator runs `ssh-magic close help`, or host presses Ctrl+C |

The bootstrap verifies cached files and repairs missing or corrupt downloads. It does not overwrite a healthy cached install. Repeating `open` shows the existing waiting code or session status and does not restart the host. A saved `help` connection cannot silently be replaced by a different code. `connect` without a code prompts without echo on a terminal, keeping the code out of command history.

Both bootstraps live at the repository root on `master`. GitHub Pages serves those same files at the short URLs above. If Pages is blocked, substitute `https://raw.githubusercontent.com/heetbeet/ssh-magic/master/boot.ps1` or `https://raw.githubusercontent.com/heetbeet/ssh-magic/master/boot.sh`. There is no separate installer service or URL shortener. Repeat the same command for a retry.

## Permanent command installation

Run the `install` row above, then close and reopen your terminal application. Windows adds `%LOCALAPPDATA%\ssh-magic\bin` to your user PATH, leaving the system PATH alone. Linux adds a marked PATH line to `.profile` and `.bashrc`, and to existing `.bash_profile`, `.bash_login`, or `.zshrc` files. Repeating installation updates the launcher without duplicating PATH entries.

| After installation | Command |
| --- | --- |
| Open user access | `ssh-magic open` |
| Open admin access | `ssh-magic open --admin` |
| Connect | `ssh-magic connect CODE` |
| Run a remote command | `ssh-magic exec help -- "remote command"` |
| Close access | `ssh-magic close help` |
| Show local state | `ssh-magic status` |
| Uninstall after closing access | `ssh-magic remove` |

Installation keeps the program available, but opens no connection and starts no service or background task. Install from an ordinary Windows terminal. Request elevation only when opening admin access. `remove` deletes the product cache and removes its own PATH entry or profile lines. Restart your terminal after removal.

## Offline installation

Download the appropriate bundle from [the release](https://github.com/heetbeet/ssh-magic/releases/latest) on an online computer, then transfer it to the target computer. The bundle includes both binaries and license notices.

| Platform | Bundle | Run after extracting, in its directory |
| --- | --- | --- |
| Windows x64 | `ssh-magic-windows-amd64.zip` | `powershell -nop -ep bypass -file install.ps1` |
| Linux x64 | `ssh-magic-linux-amd64.tar.gz` | `bash install.sh` |

These installers make no network requests. You can also run the extracted `ssh-magic.exe` or `./ssh-magic` directly without installing it. Opening and connecting still require internet access to the public pairing and transport services.

## Lifetime and cleanup

Pairing expires after ten minutes. SSH must authenticate within two minutes of redemption. Access ends two hours after opening, even if actively used. A new session requires a new `open` command.

`close help` is an authenticated revocation request. The host acknowledges it and shuts down; the connector erases its session directory. If acknowledgment fails, credentials are retained and the command reports that closure is unconfirmed. Ask the host to close its terminal in that case. `forget help` explicitly erases local credentials without revoking the host.

The host retains its SSH and Iroh secret keys in memory. The connector temporarily stores a disposable SSH private key and pinned host key in a private directory. Linux uses 0700 directories and 0600 files. Windows restricts its ACL to the current user and SYSTEM; elevated host state is administrator-owned and protected under ProgramData. If the host closes independently, saved connector credentials become unusable immediately. Expired credentials are erased on the next connection operation. No cleanup daemon runs in the background.

Windows contains host descendants in a Job Object that terminates them when the host exits. Linux uses parent-death signals for helper processes and cancellable process groups for exec commands. Deliberately detached processes and software installed during assistance can persist: ending access does not undo the work you requested.

Cached binaries remain under `%LOCALAPPDATA%\ssh-magic\bin\0.1.6` or `${XDG_CACHE_HOME:-$HOME/.cache}/ssh-magic/bin/0.1.6`. The state lives in the corresponding product cache root. Close connections, then run `ssh-magic remove` to delete the product cache. Windows uses a short-lived helper to remove its locked executable after exit. This helper is never registered for startup.

## How the internet connection works

[Magic Wormhole](https://github.com/psanford/wormhole-william) pairs both endpoints through its public TLS rendezvous service, using a password-authenticated key exchange. Only a small encrypted descriptor is exchanged: temporary client key, exact SSH host key, Iroh endpoint, permissions, and expiry. Redeeming the code enrolls one operator computer. Losing that computer's saved descriptor requires a fresh host session.

[Iroh SSH](https://github.com/rustonbsd/iroh-ssh/tree/0.2.12) establishes the transport. It tries direct NAT traversal and falls back to Iroh's public relays. Both endpoints make outbound connections. The embedded [sshd-lite](https://github.com/jpillora/sshd-lite/tree/v1.55.4) listens on a randomly assigned **127.0.0.1** port. The native SSH configuration and built-in client pin its exact Ed25519 host key and disable password authentication and agent forwarding. TCP forwarding is disabled.

The public services can observe connection metadata, but cannot decrypt the pairing payload or SSH command/file contents. Availability depends on those services and the local network allowing outbound traffic. Restrictive proxies or firewalls can still block a session. This release does not operate its own relay infrastructure or promise a relay SLA.

The bootstrap trusts GitHub HTTPS for the current script and its embedded SHA256 values; this does not provide protection if the publisher account itself is compromised. Both component binaries are verified before installation, and the Iroh binary is verified again before every launch. Administrator elevation copies verified binaries into a protected temporary directory before executing them. Release binaries are currently unsigned; platform reputation checks may warn.

## Build and test

Go 1.26.8, Windows or Linux. Node.js and native SSH are needed only for the optional internet integration test.

```text
go test ./...
go vet ./...
```

Windows: `scripts/release.ps1` builds both architectures, downloads the pinned Iroh binaries, generates root bootstraps and release copies with binary hashes, and writes `dist/SHA256SUMS`. Commit the generated `boot.ps1` and `boot.sh` to `master` when publishing a release. GitHub Pages publishes from `master` at `/`. Git and Cargo are required by release packaging to collect the Rust dependency license notices. Dependencies are pinned in go.mod/go.sum and Iroh SSH's Cargo.lock. Release license notices are supplied in `licenses.zip` and THIRD_PARTY.md, and the bootstrap caches the license archive alongside the binaries.

`tests/e2e.cjs` exercises real public pairing and Iroh connectivity, repeated open/connect, command streams and exit status, binary SFTP round trips, native SSH, and revocation. Prepare `dist/ssh-magic.exe` and `dist/iroh-ssh.exe` on Windows, or `dist/ssh-magic` and `dist/iroh-ssh` on Linux, then run `node tests/e2e.cjs`. Run the Linux test as an ordinary user. Test outputs stay in ignored `docs/temp/`.

`node tests/lifecycle.cjs` checks a healthy command across the keepalive interval, then forcibly terminates the host during another command and verifies that the client fails promptly.

`SSH_MAGIC_HOME` selects an isolated state/cache root for tests. Windows ignores it for elevated processes. ARM and macOS builds are not supplied. No remote-desktop GUI is included.
