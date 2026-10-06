//go:build linux || darwin

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func installPath(root, exe string) error {
	launcher := "#!/bin/sh\nexec '" + strings.ReplaceAll(exe, "'", "'\\''") + "' \"$@\"\n"
	if err := os.WriteFile(filepath.Join(root, "bin", "ssh-magic"), []byte(launcher), 0700); err != nil {
		return err
	}
	return updateShellPath(root, true)
}
func uninstallPath(root string) error { return updateShellPath(root, false) }
func updateShellPath(root string, add bool) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	bin := "'" + strings.ReplaceAll(filepath.Join(root, "bin"), "'", "'\\''") + "'"
	line := "case \":$PATH:\" in *:" + bin + ":*) ;; *) export PATH=" + bin + ":\"$PATH\";; esac # ssh-magic\n"
	// The first `created` files are created when missing; the rest are only updated.
	// macOS terminals start zsh login shells, which read .zprofile, not .profile;
	// .profile still serves bash users without a .bash_profile or .bash_login.
	files, created := []string{".profile", ".bashrc", ".bash_profile", ".bash_login", ".zshrc"}, 2
	if runtime.GOOS == "darwin" {
		files, created = []string{".zprofile", ".zshrc", ".profile", ".bash_profile", ".bash_login", ".bashrc"}, 3
	}
	for i, name := range files {
		path := filepath.Join(home, name)
		b, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if !add && os.IsNotExist(err) {
			continue
		}
		// Do not create a file that changes the shell's startup-file precedence.
		if add && i >= created && os.IsNotExist(err) {
			continue
		}
		updated := bytes.ReplaceAll(b, []byte(line), nil)
		if add {
			if len(updated) > 0 && updated[len(updated)-1] != '\n' {
				updated = append(updated, '\n')
			}
			updated = append(updated, line...)
		}
		if !bytes.Equal(updated, b) {
			if err = os.WriteFile(path, updated, 0600); err != nil {
				return err
			}
		}
	}
	return nil
}
