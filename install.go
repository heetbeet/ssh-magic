package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/gofrs/flock"
)

// Online bootstraps and offline bundles both run this same installer.
func installProduct(root string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	side, err := sidecar()
	if err != nil {
		return err
	}
	lock := flock.New(filepath.Join(root, "install.lock"))
	if err = lock.Lock(); err != nil {
		return err
	}
	defer lock.Unlock()
	bin := filepath.Join(root, "bin")
	dir := filepath.Join(bin, version)
	if err = privateDir(dir); err != nil {
		return err
	}
	name := "ssh-magic"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	for _, file := range [][2]string{{exe, name}, {side, filepath.Base(side)}, {filepath.Join(filepath.Dir(exe), "licenses.zip"), "licenses.zip"}} {
		if err = installFile(file[0], filepath.Join(dir, file[1])); err != nil {
			return err
		}
	}
	if err = installPath(root, filepath.Join(dir, name)); err != nil {
		return err
	}
	fmt.Println("Installed ssh-magic. Open a new terminal, then run: ssh-magic open")
	fmt.Println("Uninstall: ssh-magic remove")
	return nil
}

func installFile(source, dest string) error {
	b, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if old, e := os.ReadFile(dest); e == nil && bytes.Equal(old, b) {
		return nil
	}
	f, err := os.CreateTemp(filepath.Dir(dest), ".install-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Chmod(0700); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), dest)
}
