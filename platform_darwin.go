package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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
	// sudo can keep the invoking user's HOME; root's state stays in root's home.
	if elevated() {
		home = "/var/root"
	}
	return filepath.Join(home, "Library", "Caches", "ssh-magic"), nil
}

var parentWatch sync.Once

// macOS has no parent-death signal. A kqueue watch on the parent gives the same
// SIGTERM that Linux requests with PR_SET_PDEATHSIG.
func containHost() error {
	var err error
	parentWatch.Do(func() {
		ppid := os.Getppid()
		if ppid <= 1 {
			return
		}
		kq, e := unix.Kqueue()
		if e != nil {
			err = e
			return
		}
		ev := unix.Kevent_t{}
		unix.SetKevent(&ev, ppid, unix.EVFILT_PROC, unix.EV_ADD|unix.EV_ONESHOT)
		ev.Fflags = unix.NOTE_EXIT
		if _, e = unix.Kevent(kq, []unix.Kevent_t{ev}, nil, nil); e == unix.ESRCH {
			unix.Close(kq)
			syscall.Kill(os.Getpid(), syscall.SIGTERM)
			return
		} else if e != nil {
			unix.Close(kq)
			err = e
			return
		}
		go func() {
			out := make([]unix.Kevent_t, 1)
			for {
				if n, e := unix.Kevent(kq, nil, out, nil); n > 0 || (e != nil && e != unix.EINTR) {
					syscall.Kill(os.Getpid(), syscall.SIGTERM)
					return
				}
			}
		}()
	})
	return err
}
func prepareChild(cmd *exec.Cmd) {}
func prepareCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 2 * time.Second
}

// zsh is the macOS login shell; a login shell picks up Homebrew's PATH from .zprofile.
func shellCommand(command string) (string, []string) { return "/bin/zsh", []string{"-lc", command} }
func elevate() error                                 { return elevateWith("shasum -a 256", "/var/root") }

// Prevent idle sleep while hosting. caffeinate exits by itself when this process does.
func keepAwake() {
	cmd := exec.Command("/usr/bin/caffeinate", "-i", "-w", strconv.Itoa(os.Getpid()))
	if cmd.Start() == nil {
		go cmd.Wait()
	}
}

var reaper struct {
	sync.Mutex
	pipe   *os.File
	failed bool
}

func startTime(pid int) (unix.Timeval, bool) {
	kp, e := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if e != nil || int(kp.Proc.P_pid) != pid {
		return unix.Timeval{}, false
	}
	return kp.Proc.P_starttime, true
}

// Children are registered with a helper that holds the read end of a pipe. If
// this process dies, even by SIGKILL, the pipe closes and the helper kills every
// registered child that is still running, standing in for Linux's Pdeathsig.
// Each registration carries the child's start time, so the helper can never
// adopt a reused PID.
func trackChild(cmd *exec.Cmd) {
	kind := 'p'
	if cmd.SysProcAttr != nil && cmd.SysProcAttr.Setpgid {
		kind = 'g'
	}
	reaper.Lock()
	defer reaper.Unlock()
	if reaper.failed {
		return
	}
	var e error
	if reaper.pipe == nil {
		reaper.pipe, e = startReaper()
	}
	if e == nil {
		// The child is not reaped before trackChild returns, so this PID is still ours.
		st, ok := startTime(cmd.Process.Pid)
		if !ok {
			return
		}
		_, e = fmt.Fprintf(reaper.pipe, "%c %d %d %d\n", kind, cmd.Process.Pid, st.Sec, st.Usec)
	}
	if e != nil {
		reaper.failed = true
		fmt.Fprintln(os.Stderr, "ssh-magic: warning: process watcher unavailable; helpers can outlive a killed ssh-magic:", e)
	}
}
func startReaper() (*os.File, error) {
	exe, e := os.Executable()
	if e != nil {
		return nil, e
	}
	r, w, e := os.Pipe()
	if e != nil {
		return nil, e
	}
	defer r.Close()
	helper := exec.Command(exe, "__reap")
	helper.Stdin = r
	helper.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if e = helper.Start(); e != nil {
		w.Close()
		return nil, e
	}
	go helper.Wait()
	return w, nil
}
func reap() error {
	signal.Ignore(os.Interrupt, syscall.SIGHUP)
	kq, e := unix.Kqueue()
	if e != nil {
		return e
	}
	change := func(ident int, filter int16, flags uint16, fflags uint32) error {
		ev := unix.Kevent_t{}
		unix.SetKevent(&ev, ident, int(filter), int(flags))
		ev.Fflags = fflags
		_, e := unix.Kevent(kq, []unix.Kevent_t{ev}, nil, nil)
		return e
	}
	if e = change(0, unix.EVFILT_READ, unix.EV_ADD, 0); e != nil {
		return e
	}
	live := map[int]bool{} // pid -> kill its process group
	exited := func(events []unix.Kevent_t) {
		for _, ev := range events {
			if ev.Filter == unix.EVFILT_PROC {
				delete(live, int(ev.Ident))
			}
		}
	}
	register := func(line string) {
		f := strings.Fields(line)
		if len(f) != 4 || (f[0] != "p" && f[0] != "g") {
			return
		}
		pid, e1 := strconv.Atoi(f[1])
		sec, e2 := strconv.ParseInt(f[2], 10, 64)
		usec, e3 := strconv.ParseInt(f[3], 10, 32)
		if e1 != nil || e2 != nil || e3 != nil || pid <= 1 {
			return
		}
		// Watch first, then confirm identity: any later exit is then reported.
		if change(pid, unix.EVFILT_PROC, unix.EV_ADD|unix.EV_ONESHOT, unix.NOTE_EXIT) != nil {
			return
		}
		if st, ok := startTime(pid); !ok || st.Sec != sec || int64(st.Usec) != usec {
			change(pid, unix.EVFILT_PROC, unix.EV_DELETE, 0)
			return
		}
		live[pid] = f[0] == "g"
	}
	var pending []byte
	buf := make([]byte, 4096)
	events := make([]unix.Kevent_t, 64)
	for {
		n, e := unix.Kevent(kq, nil, events, nil)
		if e == unix.EINTR {
			continue
		}
		if e != nil {
			return e
		}
		exited(events[:n])
		eof := false
		for _, ev := range events[:n] {
			if ev.Filter != unix.EVFILT_READ {
				continue
			}
			r, e := unix.Read(0, buf)
			if r <= 0 && e != unix.EINTR {
				eof = true
				break
			}
			pending = append(pending, buf[:max(r, 0)]...)
			for {
				i := bytes.IndexByte(pending, '\n')
				if i < 0 {
					break
				}
				register(string(pending[:i]))
				pending = pending[i+1:]
			}
		}
		if !eof {
			continue
		}
		// Collect every exit already queued so an exited child is never signalled.
		// The read filter stays ready at EOF, so remove it before draining.
		change(0, unix.EVFILT_READ, unix.EV_DELETE, 0)
		zero := unix.Timespec{}
		for {
			n, e = unix.Kevent(kq, nil, events, &zero)
			if e == unix.EINTR {
				continue
			}
			if e != nil || n == 0 {
				break
			}
			exited(events[:n])
		}
		for pid, group := range live {
			if group {
				syscall.Kill(-pid, syscall.SIGKILL)
			}
			syscall.Kill(pid, syscall.SIGKILL)
		}
		return nil
	}
}
