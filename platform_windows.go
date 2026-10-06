package main

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

func interruptSignals() []os.Signal { return []os.Signal{os.Interrupt} }
func elevated() bool                { return windows.GetCurrentProcessToken().IsElevated() }
func currentSID() (string, error) {
	u, e := windows.GetCurrentProcessToken().GetTokenUser()
	if e != nil {
		return "", e
	}
	return u.User.Sid.String(), nil
}
func dataRoot() (string, error) {
	if v := os.Getenv("WH_HOME"); v != "" && !elevated() {
		return filepath.Abs(v)
	}
	base := os.Getenv("LOCALAPPDATA")
	if elevated() {
		sid, e := currentSID()
		if e != nil {
			return "", e
		}
		adminRoot := filepath.Join(os.Getenv("ProgramData"), "ssh-wormhole-admin")
		if e = privateDir(adminRoot); e != nil {
			return "", e
		}
		base = filepath.Join(adminRoot, sid)
		if e = privateDir(base); e != nil {
			return "", e
		}
	}
	if base == "" {
		return "", fmt.Errorf("local application directory unavailable")
	}
	return filepath.Join(base, "ssh-wormhole"), nil
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
		return fmt.Errorf("unsafe state directory")
	}
	sid, e := currentSID()
	if e != nil {
		return e
	}
	principal := sid
	if elevated() {
		principal = "BA"
	}
	sd, e := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + principal + ")(A;OICI;FA;;;SY)")
	if e != nil {
		return e
	}
	acl, _, e := sd.DACL()
	if e != nil {
		return e
	}
	var owner *windows.SID
	flags := windows.DACL_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION
	if elevated() {
		owner, e = windows.StringToSid("S-1-5-32-544")
		if e != nil {
			return e
		}
		flags |= windows.OWNER_SECURITY_INFORMATION
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.SECURITY_INFORMATION(flags), owner, nil, acl, nil)
}

// Holding the only job handle in this process makes Windows kill every member
// when the host exits, including when the terminal is closed or wh is killed.
func containHost() error {
	job, e := windows.CreateJobObject(nil, nil)
	if e != nil {
		return e
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	_, e = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
	if e != nil {
		windows.CloseHandle(job)
		return e
	}
	if e = windows.AssignProcessToJobObject(job, windows.CurrentProcess()); e != nil {
		windows.CloseHandle(job)
		return fmt.Errorf("cannot contain assistance processes: %w", e)
	}
	return nil
}
func prepareChild(cmd *exec.Cmd)   { cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} }
func prepareCommand(cmd *exec.Cmd) { prepareChild(cmd) }
func finishCommand(cmd *exec.Cmd)  {}
func shellCommand(command string) (string, []string) {
	return "powershell.exe", []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-Command", command}
}
func psQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
func encodedPS(s string) string {
	r := utf16.Encode([]rune(s))
	b := make([]byte, 2*len(r))
	for i, v := range r {
		b[2*i] = byte(v)
		b[2*i+1] = byte(v >> 8)
	}
	return base64.StdEncoding.EncodeToString(b)
}
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
	// UAC launches a fixed PowerShell command, which copies to an administrator-
	// protected directory and verifies the copied bytes before executing them.
	script := fmt.Sprintf(`$ErrorActionPreference='Stop'
$p=Join-Path $env:ProgramData ('ssh-wormhole-stage-'+[guid]::NewGuid())
New-Item -ItemType Directory $p|Out-Null
try {
 $acl=Get-Acl $p; $acl.SetAccessRuleProtection($true,$false)
 $admins=New-Object System.Security.Principal.SecurityIdentifier('S-1-5-32-544')
 $acl.SetOwner($admins)
 foreach($s in @('S-1-5-32-544','S-1-5-18')){$id=New-Object System.Security.Principal.SecurityIdentifier($s);$rule=New-Object System.Security.AccessControl.FileSystemAccessRule($id,'FullControl','ContainerInherit,ObjectInherit','None','Allow');$acl.AddAccessRule($rule)}
 Set-Acl $p $acl
 Copy-Item -LiteralPath %s -Destination (Join-Path $p 'wh.exe')
 Copy-Item -LiteralPath %s -Destination (Join-Path $p 'iroh-ssh.exe')
 if((Get-FileHash (Join-Path $p 'wh.exe')).Hash -ne '%s' -or (Get-FileHash (Join-Path $p 'iroh-ssh.exe')).Hash -ne '%s'){throw 'Elevated staging checksum mismatch'}
 & (Join-Path $p 'wh.exe') open --admin
}finally{Remove-Item -LiteralPath $p -Recurse -Force}`, psQuote(exe), psQuote(side), hash, sideHash)
	encoded := encodedPS(script)
	launch := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", `$p=Start-Process powershell.exe -Verb RunAs -ArgumentList '-NoProfile','-ExecutionPolicy','Bypass','-EncodedCommand',`+psQuote(encoded)+` -PassThru -Wait; exit $p.ExitCode`)
	launch.Stdout = os.Stdout
	launch.Stderr = os.Stderr
	fmt.Println("Approve UAC. The assistance code appears in the elevated terminal.")
	return launch.Run()
}
func removeProduct(root string) error {
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	rel, e := filepath.Rel(root, exe)
	if e != nil {
		return e
	}
	if strings.HasPrefix(rel, "..") {
		return os.RemoveAll(root)
	}
	// A short-lived helper waits for this exact process to exit before deleting
	// its locked executable. It is never installed as a service or startup task.
	ps := fmt.Sprintf(`$ErrorActionPreference='Stop'; Wait-Process -Id %d -ErrorAction SilentlyContinue; $root=%s; if([IO.Path]::GetFullPath($root) -ne %s){exit 1}; Remove-Item -LiteralPath $root -Recurse -Force`, os.Getpid(), psQuote(root), psQuote(root))
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-EncodedCommand", encodedPS(ps))
	prepareChild(cmd)
	if e = cmd.Start(); e != nil {
		return e
	}
	fmt.Println("Removal will finish after wh exits.")
	return nil
}
