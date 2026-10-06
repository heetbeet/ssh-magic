package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/jpillora/sshd-lite/xssh"
	"golang.org/x/crypto/ssh"
)

func sidecar() (string, error) {
	exe, e := os.Executable()
	if e != nil {
		return "", e
	}
	name := "iroh-ssh"
	expected := "6e39a6b22f14d683598600350ca642d3b67fd0b0a2f2a7b29928ad6cdf032826"
	if runtime.GOOS == "windows" {
		name += ".exe"
		expected = "d50813aea4425c2113edcb889ffcc1a97f5a0d85517a35ef56114de45d8bda64"
	}
	path := filepath.Join(filepath.Dir(exe), name)
	f, e := os.Open(path)
	if e != nil {
		return "", fmt.Errorf("missing Iroh component; rerun the bootstrap: %w", e)
	}
	defer f.Close()
	hash := sha256.New()
	if _, e = io.Copy(hash, f); e != nil {
		return "", e
	}
	if hex.EncodeToString(hash.Sum(nil)) != expected {
		return "", errors.New("Iroh component checksum mismatch; rerun the bootstrap")
	}
	return path, nil
}

// Exec requests own a cancellable process group. The client's stdin EOF alone
// does not terminate a command; closing its SSH channel or the host does.
func commandHandler(ctx context.Context, _ net.Addr) (xssh.ExecHandler, io.Closer, error) {
	return func(sess *xssh.Session, command string) (bool, error) {
		childCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		done := make(chan struct{})
		defer close(done)
		go func() {
			select {
			case <-sess.Done():
				cancel()
			case <-done:
			}
		}()
		shell, args := shellCommand(command)
		cmd := exec.CommandContext(childCtx, shell, args...)
		prepareCommand(cmd)
		cmd.Dir = sess.Config().WorkingDirectory
		cmd.Stdin = sess.Channel
		cmd.Stdout = sess.Channel
		cmd.Stderr = sess.Channel.Stderr()
		e := cmd.Run()
		status := uint32(0)
		if e != nil {
			status = 1
			var ex *exec.ExitError
			if errors.As(e, &ex) {
				status = uint32(ex.ExitCode())
			} else {
				fmt.Fprintln(sess.Channel.Stderr(), e)
			}
		}
		_, sendErr := sess.Channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{status}))
		sess.Channel.Close()
		return true, sendErr
	}, nil, nil
}
