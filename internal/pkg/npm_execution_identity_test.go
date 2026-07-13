package pkg_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/tekierz/dotfiles/internal/pkg"
)

const exactNPMEnvNodeShebang = "#!/usr/bin/env node\n"

func TestNPMExecutionIdentityObservesExactEnvNodeChainDeterministically(t *testing.T) {
	npmPath := writeNPMExecutable(t, "console.log('npm');\n")
	nodePath := writeNodeExecutable(t, "exit 0\n")

	first, err := pkg.ObserveNPMExecutionIdentity(npmPath, nodePath)
	if err != nil {
		t.Fatalf("observe npm execution identity: %v", err)
	}
	second, err := pkg.ObserveNPMExecutionIdentity(npmPath, nodePath)
	if err != nil {
		t.Fatalf("repeat npm execution identity observation: %v", err)
	}
	if first.SchemaVersion() <= 0 || first.SchemaVersion() != second.SchemaVersion() {
		t.Fatalf("schema versions = %d and %d, want one stable positive version", first.SchemaVersion(), second.SchemaVersion())
	}
	if first.Digest() == "" || first.Digest() != second.Digest() {
		t.Fatalf("digests = %q and %q, want one deterministic digest", first.Digest(), second.Digest())
	}
	if err := first.Revalidate(); err != nil {
		t.Fatalf("unchanged execution chain did not revalidate: %v", err)
	}

	npmComponent, err := pkg.ObserveExecutableIdentity(npmPath)
	if err != nil {
		t.Fatalf("observe npm component: %v", err)
	}
	nodeComponent, err := pkg.ObserveExecutableIdentity(nodePath)
	if err != nil {
		t.Fatalf("observe node component: %v", err)
	}
	naive := sha256.Sum256([]byte(npmComponent.Digest() + nodeComponent.Digest()))
	if first.Digest() == npmComponent.Digest() || first.Digest() == nodeComponent.Digest() || first.Digest() == hex.EncodeToString(naive[:]) {
		t.Fatal("aggregate digest is not domain-separated from its components")
	}

	otherNPM := writeNPMExecutable(t, "console.log('npm');\n")
	other, err := pkg.ObserveNPMExecutionIdentity(otherNPM, nodePath)
	if err != nil {
		t.Fatalf("observe alternate npm path: %v", err)
	}
	if first.Digest() == other.Digest() {
		t.Fatal("different npm invocation paths produced the same aggregate digest")
	}
}

func TestNPMExecutionIdentityRejectsEveryComponentDrift(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, string)
	}{
		{name: "content", mutate: func(t *testing.T, path string) {
			t.Helper()
			if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 7\n"), 0o700); err != nil {
				t.Fatalf("replace content: %v", err)
			}
		}},
		{name: "mode", mutate: func(t *testing.T, path string) {
			t.Helper()
			if err := os.Chmod(path, 0o600); err != nil {
				t.Fatalf("remove executable mode: %v", err)
			}
		}},
		{name: "delete", mutate: func(t *testing.T, path string) {
			t.Helper()
			if err := os.Remove(path); err != nil {
				t.Fatalf("delete component: %v", err)
			}
		}},
		{name: "symlink-retarget", mutate: retargetObservedSymlink},
	}

	for _, component := range []string{"npm", "node"} {
		for _, test := range tests {
			t.Run(component+"-"+test.name, func(t *testing.T) {
				npmPath, nodePath := npmChainForDrift(t, component, test.name == "symlink-retarget")
				identity, err := pkg.ObserveNPMExecutionIdentity(npmPath, nodePath)
				if err != nil {
					t.Fatalf("observe chain: %v", err)
				}
				before := identity.Digest()
				target := npmPath
				if component == "node" {
					target = nodePath
				}
				test.mutate(t, target)
				if err := identity.Revalidate(); err == nil {
					t.Fatal("component drift revalidated successfully")
				} else {
					assertPathFreeError(t, err, npmPath, nodePath)
				}
				if identity.Digest() != before {
					t.Fatal("immutable accepted digest changed after filesystem drift")
				}
			})
		}
	}
}

func TestObserveNPMExecutionIdentityRejectsUnsafeInputsWithoutLeakingPaths(t *testing.T) {
	validNPM := writeNPMExecutable(t, "exit 0\n")
	validNode := writeNodeExecutable(t, "exit 0\n")
	directory := t.TempDir()
	missing := filepath.Join(t.TempDir(), "private-missing-executable-marker")
	nonExecutable := writeFile(t, "private-nonexec-marker", exactNPMEnvNodeShebang+"exit 0\n", 0o600)
	oversize := filepath.Join(t.TempDir(), "private-oversize-marker")
	if err := os.WriteFile(oversize, []byte(exactNPMEnvNodeShebang), 0o700); err != nil {
		t.Fatalf("create oversize executable: %v", err)
	}
	if err := os.Truncate(oversize, pkg.MaxExecutableIdentityBytes+1); err != nil {
		t.Fatalf("grow oversize executable: %v", err)
	}
	loop := symlinkLoop(t)

	tests := []struct {
		name string
		npm  string
		node string
	}{
		{name: "relative-npm", npm: "private-relative-npm-marker", node: validNode},
		{name: "relative-node", npm: validNPM, node: "private-relative-node-marker"},
		{name: "missing-npm", npm: missing, node: validNode},
		{name: "missing-node", npm: validNPM, node: missing},
		{name: "nonexec-npm", npm: nonExecutable, node: validNode},
		{name: "nonexec-node", npm: validNPM, node: nonExecutable},
		{name: "oversize-npm", npm: oversize, node: validNode},
		{name: "oversize-node", npm: validNPM, node: oversize},
		{name: "directory-npm", npm: directory, node: validNode},
		{name: "directory-node", npm: validNPM, node: directory},
		{name: "loop-npm", npm: loop, node: validNode},
		{name: "loop-node", npm: validNPM, node: loop},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := pkg.ObserveNPMExecutionIdentity(test.npm, test.node)
			if err == nil {
				t.Fatal("unsafe input was accepted")
			}
			assertPathFreeError(t, err, test.npm, test.node)
		})
	}
}

func TestObserveNPMExecutionIdentityRejectsFIFOComponentsWithoutHanging(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("FIFO execution identity is supported on Darwin and Linux")
	}
	validNPM := writeNPMExecutable(t, "exit 0\n")
	validNode := writeNodeExecutable(t, "exit 0\n")
	for _, component := range []string{"npm", "node"} {
		t.Run(component, func(t *testing.T) {
			fifo := filepath.Join(t.TempDir(), "private-fifo-marker")
			if err := syscall.Mkfifo(fifo, 0o700); err != nil {
				t.Fatalf("create FIFO: %v", err)
			}
			npmPath, nodePath := validNPM, validNode
			if component == "npm" {
				npmPath = fifo
			} else {
				nodePath = fifo
			}
			result := make(chan error, 1)
			go func() {
				_, err := pkg.ObserveNPMExecutionIdentity(npmPath, nodePath)
				result <- err
			}()
			select {
			case err := <-result:
				if err == nil {
					t.Fatal("FIFO component was accepted")
				}
				assertPathFreeError(t, err, npmPath, nodePath)
			case <-time.After(2 * time.Second):
				t.Fatal("FIFO component observation hung")
			}
		})
	}
}

func TestObserveNPMExecutionIdentityRequiresExactEnvNodeShebang(t *testing.T) {
	nodePath := writeNodeExecutable(t, "exit 0\n")
	tests := map[string]string{
		"missing":           "console.log('npm');\n",
		"empty-interpreter": "#!\nconsole.log('npm');\n",
		"absolute-node":     "#!/usr/bin/node\nconsole.log('npm');\n",
		"env-python":        "#!/usr/bin/env python\nprint('npm')\n",
		"env-options":       "#!/usr/bin/env -S node --no-warnings\nconsole.log('npm');\n",
		"extra-argument":    "#!/usr/bin/env node --no-warnings\nconsole.log('npm');\n",
		"carriage-return":   "#!/usr/bin/env node\r\nconsole.log('npm');\n",
		"leading-space":     " #!/usr/bin/env node\nconsole.log('npm');\n",
		"no-newline":        "#!/usr/bin/env node",
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			npmPath := writeFile(t, "private-shebang-marker", content, 0o700)
			_, err := pkg.ObserveNPMExecutionIdentity(npmPath, nodePath)
			if err == nil {
				t.Fatal("non-exact npm shebang was accepted")
			}
			assertPathFreeError(t, err, npmPath, nodePath)
		})
	}
}

func TestNPMExecutionIdentityPublicSurfaceIsOpaqueAndPathFree(t *testing.T) {
	npmPath := writeNPMExecutable(t, "console.log('private-npm-body-marker');\n")
	nodePath := writeNodeExecutable(t, "echo private-node-body-marker\n")
	identity, err := pkg.ObserveNPMExecutionIdentity(npmPath, nodePath)
	if err != nil {
		t.Fatalf("observe npm execution identity: %v", err)
	}

	identityType := reflect.TypeOf(identity)
	if identityType.Kind() != reflect.Struct {
		t.Fatalf("identity kind = %s, want an opaque value struct", identityType.Kind())
	}
	for index := 0; index < identityType.NumField(); index++ {
		if field := identityType.Field(index); field.IsExported() {
			t.Fatalf("identity exposes component field %q", field.Name)
		}
	}
	for _, forbidden := range []string{"NPMPath", "NodePath", "NPMIdentity", "NodeIdentity", "Components", "ExecutableIdentities"} {
		if _, ok := identityType.MethodByName(forbidden); ok {
			t.Fatalf("identity exposes component accessor %q", forbidden)
		}
	}

	for _, formatted := range []string{identity.String(), identity.GoString(), fmt.Sprint(identity), fmt.Sprintf("%#v", identity)} {
		assertPathFreeText(t, formatted, npmPath, nodePath, "private-npm-body-marker", "private-node-body-marker")
		if !strings.Contains(formatted, identity.Digest()) {
			t.Fatalf("diagnostic %q omits aggregate digest", formatted)
		}
	}
	encoded, err := json.Marshal(identity)
	if err == nil || len(encoded) != 0 {
		t.Fatalf("JSON marshal = %q, %v; want refusal with no data", encoded, err)
	}
	assertPathFreeError(t, err, npmPath, nodePath)

	var zero pkg.NPMExecutionIdentity
	assertPathFreeText(t, zero.String(), npmPath, nodePath)
	if encoded, err := json.Marshal(zero); err == nil || len(encoded) != 0 {
		t.Fatalf("zero identity JSON marshal = %q, %v; want refusal", encoded, err)
	}
}

func npmChainForDrift(t *testing.T, component string, symlinked bool) (string, string) {
	t.Helper()
	npmPath := writeNPMExecutable(t, "console.log('npm');\n")
	nodePath := writeNodeExecutable(t, "exit 0\n")
	if !symlinked {
		return npmPath, nodePath
	}
	if component == "npm" {
		return executableSymlink(t, npmPath), nodePath
	}
	return npmPath, executableSymlink(t, nodePath)
}

func retargetObservedSymlink(t *testing.T, link string) {
	t.Helper()
	replacement := filepath.Join(t.TempDir(), "replacement-executable")
	content := "#!/bin/sh\nexit 9\n"
	if strings.Contains(filepath.Base(link), "npm") {
		content = exactNPMEnvNodeShebang + "console.log('replacement');\n"
	}
	if err := os.WriteFile(replacement, []byte(content), 0o700); err != nil {
		t.Fatalf("write replacement: %v", err)
	}
	if err := os.Remove(link); err != nil {
		t.Fatalf("remove observed symlink: %v", err)
	}
	if err := os.Symlink(replacement, link); err != nil {
		t.Fatalf("retarget observed symlink: %v", err)
	}
}

func executableSymlink(t *testing.T, target string) string {
	t.Helper()
	name := "node-link"
	if strings.Contains(filepath.Base(target), "npm") {
		name = "npm-link"
	}
	link := filepath.Join(t.TempDir(), name)
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("create executable symlink: %v", err)
	}
	return link
}

func symlinkLoop(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	first, second := filepath.Join(dir, "private-loop-a-marker"), filepath.Join(dir, "private-loop-b-marker")
	if err := os.Symlink(second, first); err != nil {
		t.Fatalf("create first loop link: %v", err)
	}
	if err := os.Symlink(first, second); err != nil {
		t.Fatalf("create second loop link: %v", err)
	}
	return first
}

func writeNPMExecutable(t *testing.T, body string) string {
	t.Helper()
	return writeFile(t, "npm", exactNPMEnvNodeShebang+body, 0o700)
}

func writeNodeExecutable(t *testing.T, body string) string {
	t.Helper()
	return writeFile(t, "node", "#!/bin/sh\n"+body, 0o700)
}

func writeFile(t *testing.T, name, content string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func assertPathFreeError(t *testing.T, err error, paths ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error")
	}
	assertPathFreeText(t, err.Error(), paths...)
	assertPathFreeText(t, fmt.Sprintf("%#v", err), paths...)
}

func assertPathFreeText(t *testing.T, value string, secrets ...string) {
	t.Helper()
	for _, secret := range secrets {
		if secret != "" && strings.Contains(value, secret) {
			t.Fatalf("diagnostic leaks private value %q: %q", secret, value)
		}
		if base := filepath.Base(secret); strings.Contains(base, "private-") && strings.Contains(value, base) {
			t.Fatalf("diagnostic leaks private basename %q: %q", base, value)
		}
	}
}
