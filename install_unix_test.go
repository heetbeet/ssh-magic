//go:build linux || darwin

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestShellInstallAndRemoval(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, "a cache's folder")
	original := "# user's settings\nexport EDITOR=vim\n"
	existing := []string{".profile", ".bashrc", ".bash_profile", ".zshrc"}
	absent, shell, startup := ".bash_login", "bash", `. "$HOME/.profile"; . "$HOME/.bashrc"`
	if runtime.GOOS == "darwin" {
		existing = []string{".zprofile", ".zshrc", ".profile", ".bash_profile"}
		absent, shell, startup = ".bash_login", "zsh", `. "$HOME/.zprofile"; . "$HOME/.zshrc"`
	}
	for _, name := range existing {
		if err := os.WriteFile(filepath.Join(home, name), []byte(original), 0640); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if err := updateShellPath(root, true); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range existing {
		b, _ := os.ReadFile(filepath.Join(home, name))
		if !strings.HasPrefix(string(b), original) || strings.Count(string(b), "# ssh-magic") != 1 {
			t.Fatalf("installation damaged or duplicated settings: %s", b)
		}
	}
	if _, err := os.Stat(filepath.Join(home, absent)); !os.IsNotExist(err) {
		t.Fatal("installer created a higher-priority startup file")
	}
	cmd := exec.Command(shell, "-c", startup+`; printf '%s' "$PATH"`)
	result, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("shell startup failed: %s", result)
	}
	count := 0
	for _, path := range strings.Split(string(result), ":") {
		if path == filepath.Join(root, "bin") {
			count++
		}
	}
	if count != 1 {
		t.Fatal("login and interactive startup duplicated the PATH entry")
	}
	if err := uninstallPath(root); err != nil {
		t.Fatal(err)
	}
	for _, name := range existing {
		b, _ := os.ReadFile(filepath.Join(home, name))
		if string(b) != original {
			t.Fatalf("removal changed user settings: %s", b)
		}
	}
}
