package main

import (
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func installPath(root, exe string) error {
	// Relative to the launcher, so paths with spaces need no registry quoting.
	launcher := "@\"%~dp0" + version + "\\ssh-magic.exe\" %*\r\n"
	if err := os.WriteFile(filepath.Join(root, "bin", "ssh-magic.cmd"), []byte(launcher), 0600); err != nil {
		return err
	}
	return updateUserPath(filepath.Join(root, "bin"), true)
}
func uninstallPath(root string) error {
	return updateUserPath(filepath.Join(root, "bin"), false)
}
func updateUserPath(dir string, add bool) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, "Environment", registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	value, kind, err := k.GetStringValue("Path")
	if err != nil && err != registry.ErrNotExist {
		return err
	}
	parts := strings.Split(value, ";")
	kept := make([]string, 0, len(parts)+1)
	found := false
	for _, part := range parts {
		if strings.EqualFold(strings.TrimRight(part, `\`), strings.TrimRight(dir, `\`)) {
			found = true
			if !add {
				continue
			}
		}
		kept = append(kept, part)
	}
	if add && !found {
		if value == "" {
			kept = nil
		}
		kept = append(kept, dir)
	}
	updated := strings.Join(kept, ";")
	if updated == value {
		return nil
	}
	if kind == registry.EXPAND_SZ {
		err = k.SetExpandStringValue("Path", updated)
	} else {
		err = k.SetStringValue("Path", updated)
	}
	if err != nil {
		return err
	}
	// New terminals launched by Explorer pick up the changed user environment.
	text, _ := windows.UTF16PtrFromString("Environment")
	proc := windows.NewLazySystemDLL("user32.dll").NewProc("SendMessageTimeoutW")
	proc.Call(0xffff, 0x001a, 0, uintptr(unsafe.Pointer(text)), 0x0002, 3000, 0)
	return nil
}
