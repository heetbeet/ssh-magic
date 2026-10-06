# SSH Wormhole

Temporary SSH access for helping someone with their computer. They run one command, read back a four-word code, and keep the terminal open. You or your agent redeem the code, run commands, transfer files, and close the session when finished.

Windows 10/11 x64 and Linux x64. No account registration or router configuration. No permanent SSH service, firewall rule, account, startup task, PATH change, or execution-policy change. Downloads are cached for repeat use.

## Open the computer being helped

Windows, from PowerShell or a Windows Run dialog:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command "& ([scriptblock]::Create((Invoke-RestMethod 'https://github.com/heetbeet/ssh-wormhole/releases/download/v0.1.0/bootstrap.ps1'))) open"
```

Linux:

```bash
bash -c 's=$(mktemp); trap "rm -f -- \"$s\"" EXIT; curl -fL --retry 3 https://github.com/heetbeet/ssh-wormhole/releases/download/v0.1.0/bootstrap.sh -o "$s" && bash "$s" open'
```

Read back the line starting with `CODE:`. Keep that terminal open. Only share the code with the person or agent you want to give access to.

For administrator access, append `--admin` to `open` inside the command. Windows asks for UAC approval and displays the code in the elevated terminal. Linux asks through sudo. An already elevated terminal must use `open --admin` explicitly. User access runs with the opening user's permissions and reaches their accessible files; it is not a filesystem sandbox.

## Connect from the operator or agent computer

Windows, replace `CODE` with the received code:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command "& ([scriptblock]::Create((Invoke-RestMethod 'https://github.com/heetbeet/ssh-wormhole/releases/download/v0.1.0/bootstrap.ps1'))) connect CODE"
```

Linux:

```bash
bash -c 's=$(mktemp); trap "rm -f -- \"$s\"" EXIT; curl -fL --retry 3 https://github.com/heetbeet/ssh-wormhole/releases/download/v0.1.0/bootstrap.sh -o "$s" && bash "$s" connect CODE'
```

The connector prints the absolute path of `wh` and a session-specific SSH configuration. Give those paths to your agent. No installed SSH client is needed for the built-in commands. Run the Windows executable with PowerShell's `&` operator if its path is quoted.

```text
wh exec help -- "remote command"
wh put help local-file remote-file
wh get help remote-file local-file
wh close help
```

Windows hosts execute commands with Windows PowerShell, `-NoProfile -NonInteractive -Command`. Linux hosts use Bash, `-lc`. stdout, stderr, stdin, and the remote exit status pass through. Each exec starts a fresh shell in the host user's home directory. To keep a working directory, include `cd` in the command. Paths for file transfers may contain spaces, Unicode, Windows drive letters, or Unix absolute paths. Transfers write a temporary sibling file and rename only after completing; a remote server without atomic overwrite support may reject an existing destination instead of replacing it.

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
| Assistance finished | Operator runs `wh close help`, or host presses Ctrl+C |

The bootstrap verifies cached files and repairs missing or corrupt downloads. It does not overwrite a healthy cached install. Repeating `open` shows the existing waiting code or session status and does not restart the host. A saved `help` connection cannot silently be replaced by a different code. `connect` without a code prompts without echo on a terminal, keeping the code out of command history.

## Lifetime and cleanup

Pairing expires after ten minutes. SSH must authenticate within two minutes of redemption. Access ends two hours after opening, even if actively used. A new session requires a new `open` command.

`close help` is an authenticated revocation request. The host acknowledges it and shuts down; the connector erases its session directory. If acknowledgment fails, credentials are retained and the command reports that closure is unconfirmed. Ask the host to close its terminal in that case. `forget help` explicitly erases local credentials without revoking the host.

The host retains its SSH and Iroh secret keys in memory. The connector temporarily stores a disposable SSH private key and pinned host key in a private directory. Linux uses 0700 directories and 0600 files. Windows restricts its ACL to the current user and SYSTEM; elevated host state is administrator-owned and protected under ProgramData. If the host closes independently, saved connector credentials become unusable immediately. Expired credentials are erased on the next connection operation. No cleanup daemon runs in the background.

Windows contains host descendants in a Job Object that terminates them when the host exits. Linux uses parent-death signals for helper processes and cancellable process groups for exec commands. Deliberately detached processes and software installed during assistance can persist: ending access does not undo the work you requested.

Cached binaries remain under `%LOCALAPPDATA%\ssh-wormhole\bin\0.1.0` or `${XDG_CACHE_HOME:-$HOME/.cache}/ssh-wormhole/bin/0.1.0`. The state lives in the corresponding product cache root. Close connections, then run `wh remove` to delete the product cache. Windows uses a short-lived helper to remove its locked executable after exit. This helper is never registered for startup.

## How the internet connection works

[Magic Wormhole](https://github.com/psanford/wormhole-william) pairs both endpoints through its public TLS rendezvous service, using a password-authenticated key exchange. Only a small encrypted descriptor is exchanged: temporary client key, exact SSH host key, Iroh endpoint, permissions, and expiry. Redeeming the code enrolls one operator computer. Losing that computer's saved descriptor requires a fresh host session.

[Iroh SSH](https://github.com/rustonbsd/iroh-ssh/tree/0.2.12) establishes the transport. It tries direct NAT traversal and falls back to Iroh's public relays. Both endpoints make outbound connections. The embedded [sshd-lite](https://github.com/jpillora/sshd-lite/tree/v1.55.4) listens on a randomly assigned **127.0.0.1** port. The native SSH configuration and built-in client pin its exact Ed25519 host key and disable password authentication and agent forwarding. TCP forwarding is disabled.

The public services can observe connection metadata, but cannot decrypt the pairing payload or SSH command/file contents. Availability depends on those services and the local network allowing outbound traffic. Restrictive proxies or firewalls can still block a session. This release does not operate its own relay infrastructure or promise a relay SLA.

The bootstrap trusts GitHub HTTPS for the pinned script and its embedded SHA256 values; this does not provide protection if the publisher account itself is compromised. Both component binaries are verified before installation, and the Iroh binary is verified again before every launch. Administrator elevation copies verified binaries into a protected temporary directory before executing them. Release binaries are currently unsigned; platform reputation checks may warn.

## Build and test

Go 1.26.8, Windows or Linux. Node.js and native SSH are needed only for the optional internet integration test.

```text
go test ./...
go vet ./...
```

Windows: `scripts/release.ps1` builds both architectures, downloads the pinned Iroh binaries, generates bootstraps with binary hashes, and writes `dist/SHA256SUMS`. Git and Cargo are required by release packaging to collect the Rust dependency license notices. Dependencies are pinned in go.mod/go.sum and Iroh SSH's Cargo.lock. Release license notices are supplied in `licenses.zip` and THIRD_PARTY.md, and the bootstrap caches the license archive alongside the binaries.

`tests/e2e.cjs` exercises real public pairing and Iroh connectivity, repeated open/connect, command streams and exit status, binary SFTP round trips, native SSH, and revocation. Prepare `dist/wh.exe` and `dist/iroh-ssh.exe` on Windows, or `dist/wh` and `dist/iroh-ssh` on Linux, then run `node tests/e2e.cjs`. Run the Linux test as an ordinary user. Test outputs stay in ignored `docs/temp/`.

`WH_HOME` selects an isolated state/cache root for tests. Windows ignores it for elevated processes. ARM and macOS builds are not supplied. No remote-desktop GUI is included.
