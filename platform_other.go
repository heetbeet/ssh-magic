//go:build !darwin

package main

import (
	"errors"
	"os/exec"
)

// Linux and Windows tie children to the host in prepareChild and containHost.
func trackChild(*exec.Cmd) {}
func keepAwake()           {}
func reap() error          { return errors.New("unknown command; run ssh-magic --help") }
