package main

import (
	"bytes"
	"os"
	"path/filepath"
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
	line := "export PATH='" + strings.ReplaceAll(filepath.Join(root, "bin"), "'", "'\\''") + "':\"$PATH\" # ssh-magic\n"
	for _, name := range []string{".profile", ".bashrc", ".bash_profile", ".bash_login", ".zshrc"} {
		path := filepath.Join(home, name)
		b, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if !add && os.IsNotExist(err) {
			continue
		}
		// Do not create a file that changes the shell's startup-file precedence.
		if add && name != ".profile" && name != ".bashrc" && os.IsNotExist(err) {
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
