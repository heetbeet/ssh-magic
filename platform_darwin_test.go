package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// trackChild starts `os.Executable() __reap`, which in tests is this binary.
// It must act as the helper rather than run the suite again.
func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == "__reap" {
		if e := reap(); e != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// Closing the registration pipe stands in for the owner dying by SIGKILL.
func TestReaperKillsRegisteredChildren(t *testing.T) {
	child := exec.Command("sleep", "60")
	group := exec.Command("sh", "-c", "sleep 60 & wait")
	group.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	for _, c := range []*exec.Cmd{child, group} {
		if e := c.Start(); e != nil {
			t.Fatal(e)
		}
	}
	defer child.Process.Kill()
	defer syscall.Kill(-group.Process.Pid, syscall.SIGKILL)
	exe, _ := os.Executable()
	helper := exec.Command(exe, "__reap")
	w, e := helper.StdinPipe()
	if e != nil {
		t.Fatal(e)
	}
	if e = helper.Start(); e != nil {
		t.Fatal(e)
	}
	for _, c := range []*exec.Cmd{child, group} {
		st, _ := startTime(c.Process.Pid)
		kind := 'p'
		if c == group {
			kind = 'g'
		}
		fmt.Fprintf(w, "%c %d %d %d\n", kind, c.Process.Pid, st.Sec, st.Usec)
	}
	// A registration whose start time does not match must be ignored.
	stray := exec.Command("sleep", "60")
	if e = stray.Start(); e != nil {
		t.Fatal(e)
	}
	defer stray.Process.Kill()
	fmt.Fprintf(w, "p %d 1 0\n", stray.Process.Pid)
	time.Sleep(200 * time.Millisecond)
	if syscall.Kill(child.Process.Pid, 0) != nil {
		t.Fatal("child killed before the owner closed the pipe")
	}
	w.Close()
	done := make(chan error, 2)
	go func() { done <- child.Wait() }()
	go func() { done <- group.Wait() }()
	for range 2 {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("registered child survived its owner")
		}
	}
	if e = helper.Wait(); e != nil {
		t.Fatal(e)
	}
	if syscall.Kill(-group.Process.Pid, 0) == nil {
		t.Fatal("process group member survived its owner")
	}
	if syscall.Kill(stray.Process.Pid, 0) != nil {
		t.Fatal("helper killed a process registered with the wrong start time")
	}
}
