//go:build darwin || linux

package runner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPrivilegedLauncherBuildsActualCommand(t *testing.T) {
	t.Setenv("PATH", "/hostile")
	t.Setenv("PRIVATE_TEST_SECRET", "must not reach child")
	for _, manager := range []string{"apt", "pacman"} {
		t.Run(manager, func(t *testing.T) {
			target := "/trusted tools/" + manager
			args := []string{"install", "-y", "git:arm64", "libfoo+bar"}
			if manager == "pacman" {
				args = []string{"-S", "--noconfirm", "--needed", "git"}
			}
			var resolutions []string
			resolvers := privilegedLauncherResolvers{
				target: func(name string) (string, error) {
					resolutions = append(resolutions, "target:"+name)
					return target, nil
				},
				sudo: func() (string, error) { resolutions = append(resolutions, "sudo"); return "/trusted sudo/sudo", nil },
				supervisor: func() (string, error) {
					resolutions = append(resolutions, "supervisor")
					return "/reviewed apps/dotfiles", nil
				},
			}
			command, err := buildPrivilegedLauncher(context.Background(), target, args, resolvers)
			if err != nil {
				t.Fatal(err)
			}
			want := append([]string{"/trusted sudo/sudo", "-n", "--", "/reviewed apps/dotfiles", privilegedSupervisorDispatchArg, target}, args...)
			args[0] = "caller changed argv"
			if command.Path != want[0] || !reflect.DeepEqual(command.Args, want) || command.Dir != "/" || command.Process != nil || command.Stdin != nil {
				t.Fatalf("actual command path=%q args=%q dir=%q process=%v", command.Path, command.Args, command.Dir, command.Process)
			}
			if !reflect.DeepEqual(command.Env, []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "LC_ALL=C"}) ||
				!reflect.DeepEqual(resolutions, []string{"target:" + target, "sudo", "supervisor"}) {
				t.Fatalf("environment=%q resolution=%q", command.Env, resolutions)
			}
		})
	}
}

func TestPrivilegedLauncherFailsBeforeUnreviewedResolution(t *testing.T) {
	for _, boundary := range []string{"nil-context", "cancelled", "target", "sudo", "supervisor", "grammar"} {
		t.Run(boundary, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch boundary {
			case "nil-context":
				ctx = nil
			case "cancelled":
				cancel()
			}
			calls := 0
			resolve := func(stage, path string) (string, error) {
				calls++
				if stage == boundary {
					return "", errors.New("untrusted executable")
				}
				return path, nil
			}
			resolvers := privilegedLauncherResolvers{
				target:     func(string) (string, error) { return resolve("target", "/reviewed/apt") },
				sudo:       func() (string, error) { return resolve("sudo", "/reviewed/sudo") },
				supervisor: func() (string, error) { return resolve("supervisor", "/reviewed/dotfiles") },
			}
			args := []string{"update"}
			if boundary == "grammar" {
				args = []string{"install", "-y", "git; touch marker"}
			}
			command, err := buildPrivilegedLauncher(ctx, "/reviewed/apt", args, resolvers)
			wantCalls := map[string]int{"nil-context": 0, "cancelled": 0, "target": 1, "sudo": 2, "supervisor": 3, "grammar": 3}[boundary]
			if err == nil || command != nil || calls != wantCalls {
				t.Fatalf("command=%v error=%v resolutions=%d, want no command and %d resolutions", command, err, calls, wantCalls)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if command, err := RunStreamingWithSudo(ctx, "/never-execute/apt", "update"); command != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("public pre-cancel returned command=%v error=%v", command, err)
	}
}

func TestPrivilegedTrustedFilesAndTargetValidation(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "apt")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 97\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := validateTrustedPrivilegedFile(path, false); err != nil {
		t.Fatal(err)
	}
	if os.Geteuid() != 0 {
		if _, err := validatePrivilegedTarget(path); !errors.Is(err, errInvalidPrivilegedRequest) {
			t.Fatalf("non-root-owned target accepted: %v", err)
		}
	}
	for _, mode := range []os.FileMode{0o600, 0o720, 0o702} {
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
		if err := validateTrustedPrivilegedFile(path, false); err == nil {
			t.Fatalf("untrusted mode %o accepted", mode)
		}
	}
	if err := os.Chmod(path, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "symlink")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{root, link, filepath.Join(root, "missing")} {
		if err := validateTrustedPrivilegedFile(invalid, false); err == nil {
			t.Fatalf("non-regular trusted executable accepted: %q", invalid)
		}
	}
	for _, invalid := range []string{"apt", "/tmp/../apt", filepath.Join(root, "sh")} {
		if _, err := validatePrivilegedTarget(invalid); err == nil {
			t.Fatalf("invalid target accepted: %q", invalid)
		}
	}
	t.Setenv("PATH", root)
	if _, err := findTrustedPrivilegedExecutable("sudo"); !errors.Is(err, errPrivilegedSupervisorUnavailable) {
		t.Fatalf("missing trusted sudo accepted: %v", err)
	}
}

func TestPrivilegedGrammarAndArgumentBounds(t *testing.T) {
	for _, test := range []struct {
		target string
		args   []string
		valid  bool
	}{
		{"apt", []string{"update"}, true}, {"apt", []string{"upgrade", "-y"}, true},
		{"apt", []string{"install", "-y", "libc6:arm64"}, true},
		{"pacman", []string{"-Syu", "--noconfirm"}, true},
		{"pacman", []string{"-S", "--noconfirm", "--needed", "git"}, true},
		{"apt", []string{"install", "-y", "--option"}, false},
		{"apt", []string{"install", "-y", "a/b"}, false},
		{"apt", []string{"install", "-y", "$(touch marker)"}, false},
		{"apt", []string{"install", "-y"}, false}, {"apt", []string{"update", "-y"}, false},
		{"pacman", []string{"-R", "--noconfirm", "git"}, false}, {"sh", []string{"update"}, false},
	} {
		if got := validPrivilegedCommand("/reviewed/"+test.target, test.args); got != test.valid {
			t.Errorf("%s %q valid=%v", test.target, test.args, got)
		}
	}
	for _, args := range [][]string{make([]string, maxPrivilegedArguments+1), {strings.Repeat("x", maxPrivilegedArgumentBytes+1)}, {"nul\x00byte"}} {
		if validPrivilegedArguments(args) {
			t.Error("invalid argument bounds accepted")
		}
	}
}
