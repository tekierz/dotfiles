package main

import (
	"testing"

	"github.com/tekierz/dotfiles/internal/pkg"
)

func setCommandManagerIdentity(t *testing.T, manager *pkg.MockPackageManager) {
	t.Helper()
	identity, err := pkg.ObserveExecutableIdentity("/bin/sh")
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SetExecutableIdentity(identity); err != nil {
		t.Fatal(err)
	}
}
