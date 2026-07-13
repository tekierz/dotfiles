package pkg_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/tekierz/dotfiles/internal/pkg"
)

const nativeNodeFixtureMode = "DOTFILES_NATIVE_NODE_FIXTURE_MODE"

func TestMain(tests *testing.M) {
	mode := os.Getenv(nativeNodeFixtureMode)
	if mode == "" {
		os.Exit(tests.Run())
	}
	if mode == "cancel" {
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}
	cwd, _ := os.Getwd()
	fmt.Printf("cwd=%s\n", cwd)
	for _, value := range os.Args[1:] {
		fmt.Printf("arg=%s\n", value)
	}
	for _, value := range os.Environ() {
		fmt.Println(value)
	}
	if mode == "nonzero" {
		fmt.Println("node-output")
		os.Exit(7)
	}
	os.Exit(0)
}

func TestNPMExecutionIdentityStartStreamingUsesAcceptedChainAndSanitizedEnvironment(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "injection-marker")
	npmPath := writeNPMExecutable(t, "exit 0\n")
	nodePath := writeRunnableNodeExecutable(t)
	identity, err := pkg.ObserveNPMExecutionIdentity(npmPath, nodePath)
	if err != nil {
		t.Fatalf("observe accepted chain: %v", err)
	}
	canonicalNPM, _ := filepath.EvalSymlinks(npmPath)
	hostile := t.TempDir()
	for _, name := range []string{"node", "npm"} {
		path := filepath.Join(hostile, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\n: > \""+marker+"\"\n"), 0o700); err != nil {
			t.Fatalf("write hostile %s: %v", name, err)
		}
	}
	t.Setenv("PATH", hostile)
	t.Setenv("DOTFILES_SAFE_MARKER", "preserved")
	t.Setenv(nativeNodeFixtureMode, "success")
	t.Setenv("PWD", "private-env-marker")
	t.Setenv("PwD", "private-env-marker")
	for _, key := range []string{"NoDe_OpTiOnS", "NODE_PATH", "nPm_CoNfIg_Registry", "npm_config_cache", "NPM_CONFIG_USERCONFIG", "NPM_CONFIG_GLOBALCONFIG", "LD_PRELOAD", "LD_LIBRARY_PATH", "DYLD_INSERT_LIBRARIES", "DYLD_LIBRARY_PATH", "DYLD_FRAMEWORK_PATH", "BASH_ENV", "ENV", "SHELLOPTS", "CDPATH"} {
		t.Setenv(key, "private-env-marker")
	}
	payload := "$(touch " + marker + ");`touch " + marker + "`"
	stream, err := identity.StartStreaming(context.Background(), "install", "-g", payload)
	if err != nil {
		t.Fatalf("start accepted npm stream: %v", err)
	}
	lines, waitErr := collectNPMStream(stream.Output(), stream.Done())
	if waitErr != nil {
		t.Fatalf("accepted npm stream: %v", waitErr)
	}
	for _, want := range []string{"cwd=/", "arg=" + canonicalNPM, "arg=install", "arg=-g", "arg=" + payload, "DOTFILES_SAFE_MARKER=preserved"} {
		if !containsNPMLine(lines, want) {
			t.Fatalf("output omitted %q: %q", want, lines)
		}
	}
	for _, forbidden := range []string{"NODE_OPTIONS=", "NoDe_OpTiOnS=", "NODE_PATH=", "NPM_CONFIG_REGISTRY=", "nPm_CoNfIg_Registry=", "npm_config_cache=", "LD_PRELOAD=", "LD_LIBRARY_PATH=", "DYLD_", "BASH_ENV=", "ENV=private-env-marker", "SHELLOPTS=", "CDPATH=", "private-env-marker"} {
		if containsNPMLine(lines, forbidden) {
			t.Fatalf("sanitized environment retained %q: %q", forbidden, lines)
		}
	}
	for _, neutral := range []string{"NPM_CONFIG_USERCONFIG=/dev/null", "NPM_CONFIG_GLOBALCONFIG=/dev/null"} {
		if countExactNPMLine(lines, neutral) != 1 {
			t.Fatalf("neutral config %q count != 1: %q", neutral, lines)
		}
	}
	if countExactNPMLine(lines, "PWD=/") != 1 {
		t.Fatalf("sanitized PWD count != 1: %q", lines)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("hostile PATH or literal argument executed: %v", err)
	}
}

func TestNPMExecutionIdentityStartStreamingRejectsInheritedCaseCollision(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "collision-side-effect")
	npmPath := writeNPMExecutable(t, ": > \""+marker+"\"\n")
	nodePath := writeRunnableNodeExecutable(t)
	identity, err := pkg.ObserveNPMExecutionIdentity(npmPath, nodePath)
	if err != nil {
		t.Fatalf("observe chain: %v", err)
	}
	t.Setenv(nativeNodeFixtureMode, "success")
	t.Setenv("DOTFILES_COLLIDE", "one")
	t.Setenv("dotfiles_collide", "two")
	stream, err := identity.StartStreaming(context.Background(), "install")
	if err == nil || stream != nil {
		t.Fatalf("case-colliding environment returned stream=%v error=%v", stream, err)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatalf("case-colliding environment started a process: %v", statErr)
	}
}

func TestObserveNPMExecutionIdentityRejectsAliasedComponents(t *testing.T) {
	npmPath := writeNPMExecutable(t, "console.log('npm');\n")
	tests := map[string]func(*testing.T) string{
		"same": func(*testing.T) string { return npmPath },
		"symlink": func(t *testing.T) string {
			path := filepath.Join(t.TempDir(), "node-link")
			if err := os.Symlink(npmPath, path); err != nil {
				t.Fatalf("create symlink: %v", err)
			}
			return path
		},
		"hardlink": func(t *testing.T) string {
			path := filepath.Join(t.TempDir(), "node-hardlink")
			if err := os.Link(npmPath, path); err != nil {
				t.Fatalf("create hardlink: %v", err)
			}
			return path
		},
	}
	for name, nodePath := range tests {
		t.Run(name, func(t *testing.T) {
			node := nodePath(t)
			identity, err := pkg.ObserveNPMExecutionIdentity(npmPath, node)
			if err == nil || identity.Digest() != "" {
				t.Fatalf("aliased components returned identity=%v error=%v", identity, err)
			}
			assertNPMStartErrorPrivate(t, err, npmPath, node)
		})
	}
}

func TestNPMExecutionIdentityStartStreamingRejectsInvalidOrDriftedAuthority(t *testing.T) {
	npmPath := writeNPMExecutable(t, "console.log('npm');\n")
	nodePath := writeRunnableNodeExecutable(t)
	accepted, err := pkg.ObserveNPMExecutionIdentity(npmPath, nodePath)
	if err != nil {
		t.Fatalf("observe accepted chain: %v", err)
	}
	t.Setenv(nativeNodeFixtureMode, "success")
	privateEnvironment := "private-env-marker"
	t.Setenv("NODE_OPTIONS", privateEnvironment)
	var nilContext context.Context
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, test := range []struct {
		name     string
		ctx      context.Context
		identity pkg.NPMExecutionIdentity
	}{
		{name: "zero", ctx: context.Background()},
		{name: "nil-context", ctx: nilContext, identity: accepted},
		{name: "pre-cancelled", ctx: cancelled, identity: accepted},
	} {
		t.Run(test.name, func(t *testing.T) {
			stream, err := test.identity.StartStreaming(test.ctx, "install")
			if err == nil || stream != nil {
				t.Fatalf("invalid start returned stream=%v error=%v", stream, err)
			}
			assertNPMStartErrorPrivate(t, err, npmPath, nodePath, privateEnvironment)
			if test.name == "pre-cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatalf("pre-cancelled start error=%v, want context.Canceled", err)
			}
		})
	}
	for _, component := range []string{"npm", "node"} {
		t.Run("drift-"+component, func(t *testing.T) {
			npm, node := writeNPMExecutable(t, "exit 0\n"), writeRunnableNodeExecutable(t)
			identity, err := pkg.ObserveNPMExecutionIdentity(npm, node)
			if err != nil {
				t.Fatalf("observe chain: %v", err)
			}
			path := npm
			if component == "node" {
				path = node
			}
			if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 9\n"), 0o700); err != nil {
				t.Fatalf("drift component: %v", err)
			}
			stream, err := identity.StartStreaming(context.Background(), "install")
			if err == nil || stream != nil {
				t.Fatalf("drifted start returned stream=%v error=%v", stream, err)
			}
			assertNPMStartErrorPrivate(t, err, npm, node, privateEnvironment)
		})
	}
}

func TestNPMExecutionIdentityStartStreamingRedactsDeterministicStartFailure(t *testing.T) {
	npmPath := writeNPMExecutable(t, "exit 0\n")
	magic := []byte{0x7f, 'E', 'L', 'F'}
	if runtime.GOOS == "linux" {
		magic = []byte{0xfe, 0xed, 0xfa, 0xcf}
	}
	nodePath := writeNodeMagic(t, magic)
	identity, err := pkg.ObserveNPMExecutionIdentity(npmPath, nodePath)
	if err != nil {
		t.Fatalf("observe chain: %v", err)
	}
	stream, err := identity.StartStreaming(context.Background(), "install")
	if err == nil || stream != nil {
		t.Fatalf("unstartable accepted node returned stream=%v error=%v", stream, err)
	}
	assertNPMStartErrorPrivate(t, err, npmPath, nodePath)
}

func TestNPMExecutionIdentityStreamingFacadePropagatesExitAndCancelWithoutExposure(t *testing.T) {
	t.Run("nonzero-and-opaque", func(t *testing.T) {
		npmPath := writeNPMExecutable(t, "printf 'node-output\\n'\nexit 7\n")
		nodePath := writeRunnableNodeExecutable(t)
		identity, err := pkg.ObserveNPMExecutionIdentity(npmPath, nodePath)
		if err != nil {
			t.Fatalf("observe chain: %v", err)
		}
		t.Setenv(nativeNodeFixtureMode, "nonzero")
		stream, err := identity.StartStreaming(context.Background(), "install")
		if err != nil {
			t.Fatalf("start stream: %v", err)
		}
		streamType := reflect.TypeOf(stream).Elem()
		for index := 0; index < streamType.NumField(); index++ {
			if streamType.Field(index).IsExported() {
				t.Fatalf("stream facade exposes field %q", streamType.Field(index).Name)
			}
		}
		for _, forbidden := range []string{"Cmd", "Command", "Path", "Args", "Env"} {
			if _, ok := reflect.TypeOf(stream).MethodByName(forbidden); ok {
				t.Fatalf("stream facade exposes %q", forbidden)
			}
		}
		var lines []string
		for line := range stream.Output() {
			lines = append(lines, line)
		}
		waitErr, doneErr, repeatedErr := stream.Wait(), <-stream.Done(), stream.Wait()
		if waitErr == nil || doneErr == nil || repeatedErr == nil || !containsNPMLine(lines, "node-output") {
			t.Fatalf("nonzero stream lines=%q error=%v", lines, waitErr)
		}
		assertNPMStartErrorPrivate(t, waitErr, npmPath, nodePath, "private-env-marker")
	})
	t.Run("cancel", func(t *testing.T) {
		npmPath := writeNPMExecutable(t, "exec /bin/sleep 30\n")
		nodePath := writeRunnableNodeExecutable(t)
		identity, err := pkg.ObserveNPMExecutionIdentity(npmPath, nodePath)
		if err != nil {
			t.Fatalf("observe chain: %v", err)
		}
		t.Setenv(nativeNodeFixtureMode, "cancel")
		stream, err := identity.StartStreaming(context.Background(), "install")
		if err != nil {
			t.Fatalf("start stream: %v", err)
		}
		started := time.Now()
		stream.Cancel()
		_, waitErr := collectNPMStream(stream.Output(), stream.Done())
		if !errors.Is(waitErr, context.Canceled) || time.Since(started) > 2*time.Second {
			t.Fatalf("cancel error=%v elapsed=%v", waitErr, time.Since(started))
		}
	})
}

func collectNPMStream(output <-chan string, done <-chan error) ([]string, error) {
	var lines []string
	for line := range output {
		lines = append(lines, line)
	}
	return lines, <-done
}

func containsNPMLine(lines []string, value string) bool {
	return countExactNPMSubstring(lines, value) > 0
}

func countExactNPMLine(lines []string, value string) int {
	count := 0
	for _, line := range lines {
		if line == value {
			count++
		}
	}
	return count
}

func countExactNPMSubstring(lines []string, value string) int {
	count := 0
	for _, line := range lines {
		if strings.Contains(line, value) {
			count++
		}
	}
	return count
}

func assertNPMStartErrorPrivate(t *testing.T, err error, values ...string) {
	t.Helper()
	text := err.Error() + fmt.Sprintf(" %#v", err)
	for _, value := range values {
		if value != "" && strings.Contains(text, value) {
			t.Fatalf("start error leaks private value %q: %q", value, text)
		}
	}
}

func writeRunnableNodeExecutable(t *testing.T) string {
	t.Helper()
	source, err := os.Executable()
	if err != nil {
		t.Fatalf("locate native test fixture: %v", err)
	}
	content, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("read native test fixture: %v", err)
	}
	return writeFile(t, "node", string(content), 0o700)
}
