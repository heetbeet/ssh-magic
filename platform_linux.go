package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func interruptSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP}
}
func elevated() bool { return os.Geteuid() == 0 }
func dataRoot() (string, error) {
	if v := os.Getenv("SSH_MAGIC_HOME"); v != "" {
		return filepath.Abs(v)
	}
	home, e := os.UserHomeDir()
	if e != nil {
		return "", e
	}
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		base = filepath.Join(home, ".cache")
	}
	return filepath.Join(base, "ssh-magic"), nil
}
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
func containHost() error         { return unix.Prctl(unix.PR_SET_PDEATHSIG, uintptr(syscall.SIGTERM), 0, 0, 0) }
func prepareChild(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL} }
func prepareCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL, Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 2 * time.Second
}
func finishCommand(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
func shellCommand(command string) (string, []string) { return "bash", []string{"-lc", command} }
func elevate() error {
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
	script := `set -eu; umask 077; p=$(mktemp -d /tmp/ssh-magic-admin.XXXXXX); trap 'rm -rf -- "$p"' EXIT; cp -- "$1" "$p/ssh-magic"; cp -- "$2" "$p/iroh-ssh"; printf '%s  %s\n' "$3" "$p/ssh-magic" "$4" "$p/iroh-ssh" | sha256sum -c --status; chmod 700 "$p/ssh-magic" "$p/iroh-ssh"; unset SSH_MAGIC_HOME XDG_CACHE_HOME; export HOME=/root; "$p/ssh-magic" open --admin`
	cmd := exec.Command("sudo", "--", "bash", "-c", script, "ssh-magic-admin", exe, side, hash, sideHash)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
func removeProduct(root string) error { return os.RemoveAll(root) }
