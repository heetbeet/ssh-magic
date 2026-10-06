//go:build linux || darwin

package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func interruptSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP}
}
func elevated() bool { return os.Geteuid() == 0 }
func privateDir(path string) error {
	if e := os.MkdirAll(path, 0700); e != nil {
		return e
	}
	info, e := os.Lstat(path)
	if e != nil {
		return e
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("unsafe state directory %s", path)
	}
	st := info.Sys().(*syscall.Stat_t)
	if int(st.Uid) != os.Geteuid() {
		return fmt.Errorf("state directory is owned by another user")
	}
	return os.Chmod(path, 0700)
}
func finishCommand(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}

// sudo runs a fixed script that copies both binaries into a private temporary
// directory and verifies the copied bytes before executing them as root.
func elevateWith(checker, rootHome string) error {
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	side, e := sidecar()
	if e != nil {
		return e
	}
	b, e := os.ReadFile(exe)
	if e != nil {
		return e
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(b))
	b, e = os.ReadFile(side)
	if e != nil {
		return e
	}
	sideHash := fmt.Sprintf("%x", sha256.Sum256(b))
	script := `set -eu; umask 077; p=$(mktemp -d /tmp/ssh-magic-admin.XXXXXX); trap 'rm -rf -- "$p"' EXIT; cp -- "$1" "$p/ssh-magic"; cp -- "$2" "$p/iroh-ssh"; printf '%s  %s\n' "$3" "$p/ssh-magic" "$4" "$p/iroh-ssh" | ` + checker + ` -c --status; chmod 700 "$p/ssh-magic" "$p/iroh-ssh"; unset SSH_MAGIC_HOME XDG_CACHE_HOME; export HOME=` + rootHome + `; "$p/ssh-magic" open --admin`
	cmd := exec.Command("sudo", "--", "bash", "-c", script, "ssh-magic-admin", exe, side, hash, sideHash)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
func removeProduct(root string) error { return os.RemoveAll(root) }
