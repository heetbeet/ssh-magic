package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

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
func shellCommand(command string) (string, []string) { return "bash", []string{"-lc", command} }
func elevate() error                                 { return elevateWith("sha256sum", "/root") }
