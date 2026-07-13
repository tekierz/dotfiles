package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tekierz/dotfiles/internal/pkg"
)

func uiManagerIdentity(t *testing.T, name, body string) pkg.ExecutableIdentity {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	identity, err := pkg.ObserveExecutableIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

func stableUIManagerIdentity() pkg.ExecutableIdentity {
	identity, err := pkg.ObserveExecutableIdentity("/bin/sh")
	if err != nil {
		panic(err)
	}
	return identity
}

func setUIManagerIdentity(t *testing.T, manager pkg.PackageManager, name string) pkg.ExecutableIdentity {
	t.Helper()
	identity := uiManagerIdentity(t, name, "exit 0")
	setter, ok := manager.(interface {
		SetExecutableIdentity(pkg.ExecutableIdentity) error
	})
	if !ok {
		t.Fatalf("manager %T cannot accept a test executable identity", manager)
	}
	if err := setter.SetExecutableIdentity(identity); err != nil {
		t.Fatal(err)
	}
	return identity
}
