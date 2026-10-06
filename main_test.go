package main

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jpillora/sshd-lite/sshd"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

func TestSSHCommandAndFiles(t *testing.T) {
	host, hpem, e := key()
	if e != nil {
		t.Fatal(e)
	}
	client, _, e := key()
	if e != nil {
		t.Fatal(e)
	}
	wrong, _, e := key()
	if e != nil {
		t.Fatal(e)
	}
	dir := t.TempDir()
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s, e := sshd.NewServer(sshd.Config{KeyBytes: hpem, AuthKeys: []ssh.PublicKey{client.PublicKey()}, WorkDir: dir, SFTP: true, LogQuiet: true, Attach: commandHandler})
	if e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { done <- s.StartWithContext(ctx, listener) }()
	defer func() {
		cancel()
		select {
		case e := <-done:
			if e != nil {
				t.Error(e)
			}
		case <-time.After(5 * time.Second):
			t.Error("server did not stop")
		}
	}()
	cfg := &ssh.ClientConfig{User: "help", Auth: []ssh.AuthMethod{ssh.PublicKeys(wrong)}, HostKeyCallback: ssh.FixedHostKey(host.PublicKey()), Timeout: 5 * time.Second}
	if c, e := ssh.Dial("tcp", listener.Addr().String(), cfg); e == nil {
		c.Close()
		t.Fatal("unrelated key accepted")
	}
	cfg.Auth = []ssh.AuthMethod{ssh.PublicKeys(client)}
	cfg.HostKeyCallback = ssh.FixedHostKey(wrong.PublicKey())
	if c, e := ssh.Dial("tcp", listener.Addr().String(), cfg); e == nil {
		c.Close()
		t.Fatal("wrong host key accepted")
	}
	cfg.HostKeyCallback = ssh.FixedHostKey(host.PublicKey())
	c, e := ssh.Dial("tcp", listener.Addr().String(), cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	session, e := c.NewSession()
	if e != nil {
		t.Fatal(e)
	}
	var out, errOut strings.Builder
	session.Stdout = &out
	session.Stderr = &errOut
	cmd := "printf OUT; printf ERR >&2; exit 37"
	if runtime.GOOS == "windows" {
		cmd = "[Console]::Out.Write('OUT'); [Console]::Error.Write('ERR'); exit 37"
	}
	e = session.Run(cmd)
	session.Close()
	var ex *ssh.ExitError
	if !errors.As(e, &ex) || ex.ExitStatus() != 37 || out.String() != "OUT" || errOut.String() != "ERR" {
		t.Fatalf("exit/streams: %v %q %q", e, out.String(), errOut.String())
	}
	sc, e := sftp.NewClient(c)
	if e != nil {
		t.Fatal(e)
	}
	defer sc.Close()
	name := remotePath(filepath.Join(dir, "file Ω.txt"), runtime.GOOS)
	f, e := sc.Create(name)
	if e != nil {
		t.Fatal(e)
	}
	data := strings.Repeat("bytes\x00Ω", 8192)
	if _, e = io.Copy(f, strings.NewReader(data)); e != nil {
		t.Fatal(e)
	}
	if e = f.Close(); e != nil {
		t.Fatal(e)
	}
	f, e = sc.Open(name)
	if e != nil {
		t.Fatal(e)
	}
	b, e := io.ReadAll(f)
	f.Close()
	if e != nil || string(b) != data {
		t.Fatal("SFTP bytes differ", e)
	}
	if b, e = os.ReadFile(filepath.Join(dir, "file Ω.txt")); e != nil || string(b) != data {
		t.Fatal("server filesystem differs", e)
	}
}

func TestInvitationRejectsExpiredAndInvalidEndpoint(t *testing.T) {
	host, _, _ := key()
	_, cpem, _ := key()
	d := invitation{Schema: "ssh-magic/1", SessionID: strings.Repeat("a", 32), Endpoint: strings.Repeat("b", 64), HostKey: strings.TrimSpace(string(ssh.MarshalAuthorizedKey(host.PublicKey()))), ClientKey: string(cpem), OS: "linux", Privilege: "user", Expires: time.Now().Add(time.Hour)}
	if e := validate(d); e != nil {
		t.Fatal(e)
	}
	d.Endpoint = "127.0.0.1:22"
	if validate(d) == nil {
		t.Fatal("endpoint injection accepted")
	}
	d.Endpoint = strings.Repeat("b", 64)
	d.Expires = time.Now().Add(-time.Second)
	if validate(d) == nil {
		t.Fatal("expired credential accepted")
	}
}
