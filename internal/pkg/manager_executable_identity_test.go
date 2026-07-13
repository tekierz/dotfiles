package pkg

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/tekierz/dotfiles/internal/runner"
)

type packageManagerWithoutExecutableIdentity struct{ PackageManager }

func writeManagerExecutable(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeCapturedManagerExecutable(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	script := "#!/bin/sh\n" +
		"printf 'target:%s' \"$0\" >> \"$MANAGER_CAPTURE_LOG\"\n" +
		"for arg in \"$@\"; do printf ' <%s>' \"$arg\" >> \"$MANAGER_CAPTURE_LOG\"; done\n" +
		"printf '\\n' >> \"$MANAGER_CAPTURE_LOG\"\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func installHostileManagerPath(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range names {
		path := filepath.Join(dir, name)
		script := "#!/bin/sh\nprintf 'hostile:" + name + "\\n' >> \"$MANAGER_CAPTURE_LOG\"\nexit 91\n"
		if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func installFakeSudo(t *testing.T, dir string) {
	t.Helper()
	script := "#!/bin/sh\n" +
		"printf 'sudo' >> \"$MANAGER_CAPTURE_LOG\"\n" +
		"for arg in \"$@\"; do printf ' <%s>' \"$arg\" >> \"$MANAGER_CAPTURE_LOG\"; done\n" +
		"printf '\\n' >> \"$MANAGER_CAPTURE_LOG\"\n" +
		"if [ \"$1\" = '-n' ]; then shift; fi\n" +
		"exec \"$@\"\n"
	if err := os.WriteFile(filepath.Join(dir, "sudo"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
}

func waitManagerStreaming(t *testing.T, command *runner.StreamingCmd, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	if command == nil {
		t.Fatal("expected streaming command")
	}
	for range command.Output {
	}
	if err := command.Wait(); err != nil {
		t.Fatal(err)
	}
}

func resetManagerCaptureLog(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
}

func requireManagerCaptureLog(t *testing.T, path string, expected ...string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.TrimSpace(string(content))
	if strings.Contains(text, "hostile:") {
		t.Fatalf("execution used hostile PATH replacement: %q", text)
	}
	var lines []string
	if text != "" {
		lines = strings.Split(text, "\n")
	}
	if !reflect.DeepEqual(lines, expected) {
		t.Fatalf("captured argv = %#v, want %#v", lines, expected)
	}
}

func requireManagerExecutableIdentity(t *testing.T, manager PackageManager, expected ExecutableIdentity) {
	t.Helper()
	provider, ok := manager.(ExecutableIdentityProvider)
	if !ok {
		t.Fatal("manager does not expose the optional executable identity capability")
	}
	identity, ok := provider.ExecutableIdentity()
	if !ok {
		t.Fatal("manager executable identity is unavailable")
	}
	if identity.Digest() != expected.Digest() {
		t.Fatalf("manager executable digest = %q, want %q", identity.Digest(), expected.Digest())
	}
}

func TestExecutableIdentityProviderIsOptional(t *testing.T) {
	manager := packageManagerWithoutExecutableIdentity{PackageManager: NewMockPackageManager()}
	if _, ok := any(manager).(ExecutableIdentityProvider); ok {
		t.Fatal("PackageManager unexpectedly requires ExecutableIdentityProvider")
	}
}

func TestManagerExecutableIdentityConstructorsNormalizeLookupPaths(t *testing.T) {
	tests := []struct {
		name string
		new  func(executableLookup) PackageManager
	}{
		{name: "brew", new: func(lookup executableLookup) PackageManager { return newBrewManager(lookup) }},
		{name: "apt", new: func(lookup executableLookup) PackageManager { return newAptManager(lookup) }},
		{name: "pacman", new: func(lookup executableLookup) PackageManager { return newPacmanManager(false, lookup) }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			target := writeManagerExecutable(t, test.name)
			calls := []string{}
			manager := test.new(func(name string) (string, error) {
				calls = append(calls, name)
				return target, nil
			})
			if !manager.IsAvailable() {
				t.Fatal("manager with a valid normalized executable is unavailable")
			}
			if !reflect.DeepEqual(calls, []string{test.name}) {
				t.Fatalf("lookup calls = %v, want [%s]", calls, test.name)
			}
			expected, err := ObserveExecutableIdentity(target)
			if err != nil {
				t.Fatal(err)
			}
			requireManagerExecutableIdentity(t, manager, expected)
		})
	}

	t.Run("relative lookup result rejected", func(t *testing.T) {
		target := writeManagerExecutable(t, "brew-relative")
		cwd, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		relative, err := filepath.Rel(cwd, target)
		if err != nil || filepath.IsAbs(relative) {
			t.Fatalf("build relative executable fixture: path=%q err=%v", relative, err)
		}
		manager := newBrewManager(func(name string) (string, error) {
			if name != "brew" {
				t.Fatalf("lookup requested %q, want brew", name)
			}
			return relative, nil
		})
		if manager.IsAvailable() {
			t.Fatal("relative lookup result became an available manager")
		}
		if identity, ok := manager.ExecutableIdentity(); ok || identity != (ExecutableIdentity{}) {
			t.Fatal("relative lookup result published executable identity")
		}
	})
}

func TestManagerExecutableIdentityConstructorsRejectInvalidLookups(t *testing.T) {
	lookupError := errors.New("lookup failed")
	nonExecutable := filepath.Join(t.TempDir(), "non-executable")
	if err := os.WriteFile(nonExecutable, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	dangling := filepath.Join(t.TempDir(), "dangling")
	if err := os.Symlink(filepath.Join(t.TempDir(), "missing-target"), dangling); err != nil {
		t.Fatal(err)
	}
	oversize := writeManagerExecutable(t, "oversize")
	if err := os.Truncate(oversize, MaxExecutableIdentityBytes+1); err != nil {
		t.Fatal(err)
	}
	valid := writeManagerExecutable(t, "valid")
	unclean := filepath.Dir(valid) + string(os.PathSeparator) + "nested/../" + filepath.Base(valid)
	symlinkDir := t.TempDir()
	symlinkParent := filepath.Join(symlinkDir, "linked")
	if err := os.Symlink(filepath.Dir(valid), symlinkParent); err != nil {
		t.Fatal(err)
	}
	symlinkDotDot := symlinkParent + string(os.PathSeparator) + "child/../" + filepath.Base(valid)
	tests := []struct {
		name   string
		lookup executableLookup
	}{
		{name: "not found", lookup: func(string) (string, error) { return "", exec.ErrNotFound }},
		{name: "empty nil", lookup: func(string) (string, error) { return "", nil }},
		{name: "nonempty error", lookup: func(string) (string, error) { return valid, lookupError }},
		{name: "err dot", lookup: func(string) (string, error) { return valid, exec.ErrDot }},
		{name: "unclean result", lookup: func(string) (string, error) { return unclean, nil }},
		{name: "symlink dotdot result", lookup: func(string) (string, error) { return symlinkDotDot, nil }},
		{name: "control result", lookup: func(string) (string, error) { return "/tmp/manager\nsecret", nil }},
		{name: "missing result", lookup: func(string) (string, error) { return filepath.Join(t.TempDir(), "missing"), nil }},
		{name: "non executable result", lookup: func(string) (string, error) { return nonExecutable, nil }},
		{name: "dangling symlink result", lookup: func(string) (string, error) { return dangling, nil }},
		{name: "oversize result", lookup: func(string) (string, error) { return oversize, nil }},
	}
	constructors := map[string]func(executableLookup) PackageManager{
		"brew": func(lookup executableLookup) PackageManager { return newBrewManager(lookup) },
		"apt":  func(lookup executableLookup) PackageManager { return newAptManager(lookup) },
	}
	for constructorName, constructor := range constructors {
		for _, test := range tests {
			t.Run(constructorName+"/"+test.name, func(t *testing.T) {
				manager := constructor(test.lookup)
				if manager.IsAvailable() {
					t.Fatal("manager accepted an invalid executable lookup")
				}
				provider, ok := manager.(ExecutableIdentityProvider)
				if !ok {
					t.Fatal("concrete manager lost its optional identity capability")
				}
				if identity, ok := provider.ExecutableIdentity(); ok || identity != (ExecutableIdentity{}) {
					t.Fatal("invalid manager lookup published executable identity material")
				}
			})
		}
	}
}

func TestManagerExecutableIdentityBrewAndAptProviderCoherence(t *testing.T) {
	for _, name := range []string{"brew", "apt"} {
		t.Run(name, func(t *testing.T) {
			path := writeManagerExecutable(t, name)
			lookup := func(requested string) (string, error) {
				if requested != name {
					t.Fatalf("lookup requested %q, want %q", requested, name)
				}
				return path, nil
			}
			var manager PackageManager
			if name == "brew" {
				manager = newBrewManager(lookup)
			} else {
				manager = newAptManager(lookup)
			}
			expected, err := ObserveExecutableIdentity(path)
			if err != nil {
				t.Fatal(err)
			}
			requireManagerExecutableIdentity(t, manager, expected)
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			provider := manager.(ExecutableIdentityProvider)
			if identity, ok := provider.ExecutableIdentity(); !ok || identity.Digest() != expected.Digest() {
				t.Fatal("provider performed I/O instead of returning its immutable constructor snapshot")
			}
		})
	}
}

func TestManagerExecutableIdentityParuPreferenceAndFallback(t *testing.T) {
	t.Run("valid paru preferred", func(t *testing.T) {
		paru := writeManagerExecutable(t, "paru")
		calls := []string{}
		manager := newPacmanManager(true, func(name string) (string, error) {
			calls = append(calls, name)
			if name != "paru" {
				t.Fatalf("unexpected fallback lookup %q", name)
			}
			return paru, nil
		})
		if manager.Name() != "paru" || !manager.useParu || !reflect.DeepEqual(calls, []string{"paru"}) {
			t.Fatalf("paru preference mismatch: name=%q useParu=%v calls=%v", manager.Name(), manager.useParu, calls)
		}
		expected, err := ObserveExecutableIdentity(paru)
		if err != nil {
			t.Fatal(err)
		}
		requireManagerExecutableIdentity(t, manager, expected)
	})

	for name, notFoundErr := range map[string]error{
		"direct not found":   exec.ErrNotFound,
		"wrapped exec error": &exec.Error{Name: "paru", Err: exec.ErrNotFound},
	} {
		t.Run(name+" falls back to pacman", func(t *testing.T) {
			pacman := writeManagerExecutable(t, "pacman")
			calls := []string{}
			manager := newPacmanManager(true, func(name string) (string, error) {
				calls = append(calls, name)
				if name == "paru" {
					return "", notFoundErr
				}
				return pacman, nil
			})
			if manager.Name() != "pacman" || manager.useParu || !reflect.DeepEqual(calls, []string{"paru", "pacman"}) {
				t.Fatalf("pacman fallback mismatch: name=%q useParu=%v calls=%v", manager.Name(), manager.useParu, calls)
			}
			expected, err := ObserveExecutableIdentity(pacman)
			if err != nil {
				t.Fatal(err)
			}
			requireManagerExecutableIdentity(t, manager, expected)
		})
	}

	t.Run("prefer false never looks up paru", func(t *testing.T) {
		pacman := writeManagerExecutable(t, "pacman")
		calls := []string{}
		manager := newPacmanManager(false, func(name string) (string, error) {
			calls = append(calls, name)
			if name != "pacman" {
				t.Fatalf("unexpected lookup %q", name)
			}
			return pacman, nil
		})
		if !manager.IsAvailable() || !reflect.DeepEqual(calls, []string{"pacman"}) {
			t.Fatalf("prefer=false lookup mismatch: available=%v calls=%v", manager.IsAvailable(), calls)
		}
		expected, err := ObserveExecutableIdentity(pacman)
		if err != nil {
			t.Fatal(err)
		}
		requireManagerExecutableIdentity(t, manager, expected)
	})

	t.Run("non-not-found paru error does not fallback", func(t *testing.T) {
		calls := []string{}
		manager := newPacmanManager(true, func(name string) (string, error) {
			calls = append(calls, name)
			return "", errors.New("unavailable")
		})
		if manager.IsAvailable() || manager.Name() != "pacman" || manager.useParu || !reflect.DeepEqual(calls, []string{"paru"}) {
			t.Fatalf("non-not-found error retried fallback: available=%v name=%q useParu=%v calls=%v", manager.IsAvailable(), manager.Name(), manager.useParu, calls)
		}
		if identity, ok := manager.ExecutableIdentity(); ok || identity != (ExecutableIdentity{}) {
			t.Fatal("unavailable pacman manager published executable identity")
		}
	})

	t.Run("found invalid paru does not fallback", func(t *testing.T) {
		validParu := writeManagerExecutable(t, "paru")
		missing := filepath.Join(t.TempDir(), "missing")
		unclean := filepath.Dir(validParu) + string(os.PathSeparator) + "child/../" + filepath.Base(validParu)
		directory := t.TempDir()
		fifo := filepath.Join(t.TempDir(), "paru-fifo")
		if err := syscall.Mkfifo(fifo, 0o700); err != nil {
			t.Fatal(err)
		}
		empty := filepath.Join(t.TempDir(), "empty-paru")
		if err := os.WriteFile(empty, nil, 0o700); err != nil {
			t.Fatal(err)
		}
		nonExecutable := filepath.Join(t.TempDir(), "unsafe-paru")
		if err := os.WriteFile(nonExecutable, []byte("data"), 0o600); err != nil {
			t.Fatal(err)
		}
		oversize := writeManagerExecutable(t, "oversize-paru")
		if err := os.Truncate(oversize, MaxExecutableIdentityBytes+1); err != nil {
			t.Fatal(err)
		}
		special := writeManagerExecutable(t, "special-paru")
		if err := os.Chmod(special, 0o700|os.ModeSetuid); err != nil {
			t.Fatal(err)
		}
		specialSupported := true
		if info, err := os.Stat(special); err != nil {
			t.Fatal(err)
		} else if info.Mode()&os.ModeSetuid == 0 {
			specialSupported = false
		}
		dangling := filepath.Join(t.TempDir(), "dangling-paru")
		if err := os.Symlink(missing, dangling); err != nil {
			t.Fatal(err)
		}
		loop := filepath.Join(t.TempDir(), "loop-paru")
		if err := os.Symlink(loop, loop); err != nil {
			t.Fatal(err)
		}
		for _, test := range []struct {
			name string
			path string
			err  error
			skip bool
		}{
			{name: "relative", path: "relative/paru"},
			{name: "unclean", path: unclean},
			{name: "errdot", path: validParu, err: exec.ErrDot},
			{name: "nonempty error", path: validParu, err: errors.New("found with error")},
			{name: "arbitrary error", err: errors.New("lookup failed")},
			{name: "empty nil"},
			{name: "control", path: filepath.Join(t.TempDir(), "paru\ncontrol")},
			{name: "invalid utf8", path: string(os.PathSeparator) + string([]byte{0xff})},
			{name: "overlong", path: string(os.PathSeparator) + strings.Repeat("p", MaxExecutableIdentityPathBytes+1)},
			{name: "missing", path: missing},
			{name: "directory", path: directory},
			{name: "fifo", path: fifo},
			{name: "empty file", path: empty},
			{name: "non executable", path: nonExecutable},
			{name: "oversize", path: oversize},
			{name: "special", path: special, skip: !specialSupported},
			{name: "dangling", path: dangling},
			{name: "loop", path: loop},
			{name: "nonempty not found", path: validParu, err: exec.ErrNotFound},
		} {
			t.Run(test.name, func(t *testing.T) {
				if test.skip {
					t.Skip("filesystem did not retain special mode")
				}
				calls := []string{}
				done := make(chan *PacmanManager, 1)
				go func() {
					done <- newPacmanManager(true, func(name string) (string, error) {
						calls = append(calls, name)
						return test.path, test.err
					})
				}()
				var manager *PacmanManager
				select {
				case manager = <-done:
				case <-time.After(500 * time.Millisecond):
					t.Fatal("invalid Paru constructor blocked")
				}
				if manager.IsAvailable() || manager.Name() != "pacman" || manager.useParu || !reflect.DeepEqual(calls, []string{"paru"}) {
					t.Fatalf("invalid Paru state mismatch: available=%v name=%q useParu=%v calls=%v", manager.IsAvailable(), manager.Name(), manager.useParu, calls)
				}
				if identity, ok := manager.ExecutableIdentity(); ok || identity != (ExecutableIdentity{}) {
					t.Fatal("invalid Paru published identity")
				}
			})
		}
	})

	t.Run("both not found", func(t *testing.T) {
		calls := []string{}
		manager := newPacmanManager(true, func(name string) (string, error) {
			calls = append(calls, name)
			return "", exec.ErrNotFound
		})
		if manager.IsAvailable() || manager.Name() != "pacman" || manager.useParu || !reflect.DeepEqual(calls, []string{"paru", "pacman"}) {
			t.Fatalf("both-not-found state mismatch: available=%v name=%q useParu=%v calls=%v", manager.IsAvailable(), manager.Name(), manager.useParu, calls)
		}
		if identity, ok := manager.ExecutableIdentity(); ok || identity != (ExecutableIdentity{}) {
			t.Fatal("both-not-found manager published identity")
		}
	})

	t.Run("not-found paru with invalid pacman stays unavailable", func(t *testing.T) {
		calls := []string{}
		manager := newPacmanManager(true, func(name string) (string, error) {
			calls = append(calls, name)
			if name == "paru" {
				return "", exec.ErrNotFound
			}
			return filepath.Join(t.TempDir(), "missing-pacman"), nil
		})
		if manager.IsAvailable() || manager.Name() != "pacman" || manager.useParu || !reflect.DeepEqual(calls, []string{"paru", "pacman"}) {
			t.Fatalf("invalid fallback Pacman state mismatch: available=%v name=%q useParu=%v calls=%v", manager.IsAvailable(), manager.Name(), manager.useParu, calls)
		}
		if identity, ok := manager.ExecutableIdentity(); ok || identity != (ExecutableIdentity{}) {
			t.Fatal("invalid fallback Pacman published identity")
		}
	})
}

func TestManagerExecutableIdentityManagersStoreNoLegacyPaths(t *testing.T) {
	for name, value := range map[string]any{
		"brew":   BrewManager{},
		"apt":    AptManager{},
		"pacman": PacmanManager{},
	} {
		t.Run(name, func(t *testing.T) {
			typeOf := reflect.TypeOf(value)
			for index := 0; index < typeOf.NumField(); index++ {
				field := typeOf.Field(index)
				if field.Type.Kind() == reflect.String || strings.Contains(strings.ToLower(field.Name), "path") {
					t.Fatalf("manager retains legacy executable path field %s %s", field.Name, field.Type)
				}
			}
		})
	}
}

func TestManagerExecutableIdentityDetectionDoesNotRetryPacman(t *testing.T) {
	source, err := parser.ParseFile(token.NewFileSet(), "manager.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, functionName := range []string{"detectManagerImpl", "AllManagers"} {
		t.Run(functionName, func(t *testing.T) {
			publicTrueCalls, forbiddenUses, forbiddenComposites := 0, 0, 0
			for _, declaration := range source.Decls {
				function, ok := declaration.(*ast.FuncDecl)
				if !ok || function.Name.Name != functionName {
					continue
				}
				publicTrueCalls, forbiddenUses, forbiddenComposites = classifyPacmanConstructorUsage(function.Body)
			}
			if publicTrueCalls != 1 || forbiddenUses != 0 || forbiddenComposites != 0 {
				t.Fatalf("%s Pacman construction: public-true=%d forbidden-uses=%d composites=%d, want 1/0/0", functionName, publicTrueCalls, forbiddenUses, forbiddenComposites)
			}
		})
	}
}

func classifyPacmanConstructorUsage(node ast.Node) (publicTrueCalls, forbiddenUses, forbiddenComposites int) {
	allowedDirectCalls := make(map[token.Pos]struct{})
	ast.Inspect(node, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.CallExpr:
			identifier, ok := value.Fun.(*ast.Ident)
			if !ok || identifier.Name != "NewPacmanManager" || len(value.Args) != 1 {
				return true
			}
			argument, ok := value.Args[0].(*ast.Ident)
			if !ok || argument.Name != "true" {
				return true
			}
			allowedDirectCalls[identifier.Pos()] = struct{}{}
			publicTrueCalls++
		case *ast.CompositeLit:
			identifier, ok := value.Type.(*ast.Ident)
			if ok && identifier.Name == "PacmanManager" {
				forbiddenComposites++
			}
		}
		return true
	})

	ast.Inspect(node, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok || (identifier.Name != "NewPacmanManager" && identifier.Name != "newPacmanManager") {
			return true
		}
		if _, allowed := allowedDirectCalls[identifier.Pos()]; !allowed {
			forbiddenUses++
		}
		return true
	})
	return publicTrueCalls, forbiddenUses, forbiddenComposites
}

func TestManagerExecutableIdentityDetectionRejectsConstructorAliases(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{name: "public", source: `package pkg; func detectManagerImpl() { ctor := NewPacmanManager; ctor(false) }`},
		{name: "private", source: `package pkg; func detectManagerImpl() { ctor := newPacmanManager; ctor(false) }`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source, err := parser.ParseFile(token.NewFileSet(), "fixture.go", test.source, 0)
			if err != nil {
				t.Fatal(err)
			}
			function := source.Decls[0].(*ast.FuncDecl)
			publicTrueCalls, forbiddenUses, forbiddenComposites := classifyPacmanConstructorUsage(function.Body)
			if publicTrueCalls != 0 || forbiddenUses != 1 || forbiddenComposites != 0 {
				t.Fatalf("alias classification = %d/%d/%d, want 0/1/0", publicTrueCalls, forbiddenUses, forbiddenComposites)
			}
		})
	}
}

func TestManagerExecutableIdentityMockValidationAndReset(t *testing.T) {
	mock := NewMockPackageManager()
	if identity, ok := mock.ExecutableIdentity(); ok || identity != (ExecutableIdentity{}) {
		t.Fatal("default mock unexpectedly has executable identity")
	}

	path := writeManagerExecutable(t, "mock-manager")
	valid, err := ObserveExecutableIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.SetExecutableIdentity(valid); err != nil {
		t.Fatalf("set valid mock identity: %v", err)
	}
	if identity, ok := mock.ExecutableIdentity(); !ok || identity.Digest() != valid.Digest() {
		t.Fatal("mock did not return configured executable identity")
	}

	invalid := valid
	invalid.digest = "invalid"
	if err := mock.SetExecutableIdentity(invalid); !errors.Is(err, errExecutableIdentityInvalid) {
		t.Fatalf("invalid mock identity error = %v", err)
	}
	if err := mock.SetExecutableIdentity(ExecutableIdentity{}); !errors.Is(err, errExecutableIdentityInvalid) {
		t.Fatalf("zero mock identity error = %v", err)
	}
	if identity, ok := mock.ExecutableIdentity(); !ok || identity.Digest() != valid.Digest() {
		t.Fatal("invalid setter call replaced the prior valid mock identity")
	}

	mock.executableIdentity = invalid
	if identity, ok := mock.ExecutableIdentity(); ok || identity != (ExecutableIdentity{}) {
		t.Fatal("mock published directly corrupted executable identity")
	}
	mock.executableIdentity = valid
	mock.InstallCalls = [][]string{{"one"}}
	mock.Reset()
	if len(mock.InstallCalls) != 0 {
		t.Fatal("mock reset did not clear call tracking")
	}
	if identity, ok := mock.ExecutableIdentity(); !ok || identity.Digest() != valid.Digest() {
		t.Fatal("mock reset cleared configured executable identity")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, ok := mock.ExecutableIdentity(); !ok {
		t.Fatal("mock provider performed I/O instead of validating its stored snapshot")
	}
}

func TestManagerExecutableIdentityFormattingIsPathFree(t *testing.T) {
	secretDir := t.TempDir()
	validPath := filepath.Join(secretDir, "secret-manager-target")
	if err := os.WriteFile(validPath, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	missingPath := filepath.Join(secretDir, "secret-rejected-target")
	brew := newBrewManager(func(string) (string, error) { return validPath, nil })
	apt := newAptManager(func(string) (string, error) { return missingPath, nil })
	pacman := newPacmanManager(false, func(string) (string, error) { return validPath, nil })
	mock := NewMockPackageManager()
	identity, err := ObserveExecutableIdentity(validPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.SetExecutableIdentity(identity); err != nil {
		t.Fatal(err)
	}

	values := []any{*brew, brew, *apt, apt, *pacman, pacman, *mock, mock}
	for index, value := range values {
		for _, rendered := range []string{fmt.Sprintf("%v", value), fmt.Sprintf("%+v", value), fmt.Sprintf("%#v", value)} {
			for _, forbidden := range []string{secretDir, filepath.Base(validPath), filepath.Base(missingPath), os.Getenv("HOME"), os.Getenv("USER")} {
				if forbidden != "" && len(forbidden) > 3 && strings.Contains(strings.ToLower(rendered), strings.ToLower(forbidden)) {
					t.Fatalf("formatted manager %d leaked private path or user material", index)
				}
			}
		}
	}
}

func managerBuilderName(call *ast.CallExpr) string {
	switch function := call.Fun.(type) {
	case *ast.Ident:
		if function.Name == "packageCommand" || function.Name == "packageCommandWithContext" {
			return function.Name
		}
	case *ast.SelectorExpr:
		owner, ok := function.X.(*ast.Ident)
		if !ok {
			return ""
		}
		qualified := owner.Name + "." + function.Sel.Name
		switch qualified {
		case "exec.CommandContext", "runner.RunStreaming", "runner.RunStreamingWithSudo":
			return qualified
		}
	}
	return ""
}

func exactManagerExecutableCall(expression ast.Expr, receiver string) bool {
	call, ok := expression.(*ast.CallExpr)
	if !ok || len(call.Args) != 0 {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "executablePath" {
		return false
	}
	identifier, ok := selector.X.(*ast.Ident)
	return ok && identifier.Name == receiver
}

func exactSudoManagerChild(arguments []ast.Expr, receiver string) bool {
	if len(arguments) == 0 {
		return false
	}
	index := 0
	if literal, ok := stringLiteral(arguments[0]); ok && literal == "-n" {
		index++
	}
	if index >= len(arguments) {
		return false
	}
	if exactManagerExecutableCall(arguments[index], receiver) {
		return true
	}
	appendCall, ok := arguments[index].(*ast.CallExpr)
	if !ok || len(appendCall.Args) == 0 {
		return false
	}
	function, ok := appendCall.Fun.(*ast.Ident)
	if !ok || function.Name != "append" {
		return false
	}
	slice, ok := appendCall.Args[0].(*ast.CompositeLit)
	return ok && len(slice.Elts) > 0 && exactManagerExecutableCall(slice.Elts[0], receiver)
}

func stringLiteral(expression ast.Expr) (string, bool) {
	literal, ok := expression.(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return "", false
	}
	return strings.Trim(literal.Value, `"`), true
}

func TestManagerExecutableIdentityExecutionBuildersAreExhaustivelyClassified(t *testing.T) {
	tests := []struct {
		file      string
		receiver  string
		auxiliary map[string]bool
		allowSudo bool
		expected  map[string]map[string]int
	}{
		{file: "brew.go", receiver: "b", expected: map[string]map[string]int{
			"Install": {"packageCommand": 1}, "Uninstall": {"packageCommand": 1}, "IsInstalledContext": {"packageCommandWithContext": 1},
			"GetVersion": {"packageCommand": 1}, "CheckOutdated": {"packageCommand": 1}, "CheckOutdatedNonGreedy": {"packageCommand": 1},
			"Update": {"packageCommand": 1}, "UpdateAll": {"packageCommand": 1}, "Search": {"packageCommand": 1},
			"ListInstalled": {"packageCommand": 1}, "ListInstalledCasks": {"packageCommand": 1},
			"InstallStreaming": {"runner.RunStreaming": 1}, "InstallCasksStreaming": {"runner.RunStreaming": 1},
			"UpdateStreaming": {"runner.RunStreaming": 1}, "UpdateAllStreaming": {"runner.RunStreaming": 1},
		}},
		{file: "apt.go", receiver: "a", auxiliary: map[string]bool{"dpkg": true, "dpkg-query": true}, allowSudo: true, expected: map[string]map[string]int{
			"Install": {"packageCommand": 1}, "Uninstall": {"packageCommand": 1}, "IsInstalledContext": {"packageCommandWithContext": 1},
			"GetVersion": {"packageCommand": 1}, "CheckOutdated": {"packageCommand": 1}, "Update": {"packageCommand": 2},
			"UpdateAll": {"packageCommand": 2}, "Search": {"packageCommand": 1}, "getInstalledVersions": {"packageCommand": 1},
			"ListInstalled": {"packageCommand": 1}, "InstallStreaming": {"runner.RunStreamingWithSudo": 1},
			"UpdateStreaming":    {"exec.CommandContext": 1, "runner.RunStreamingWithSudo": 1},
			"UpdateAllStreaming": {"runner.RunStreamingWithSudo": 2},
		}},
		{file: "pacman.go", receiver: "p", auxiliary: map[string]bool{"checkupdates": true}, allowSudo: true, expected: map[string]map[string]int{
			"Install": {"packageCommand": 2}, "Uninstall": {"packageCommand": 2}, "IsInstalledContext": {"packageCommandWithContext": 1},
			"GetVersion": {"packageCommand": 1}, "CheckOutdated": {"packageCommand": 1}, "checkOfficialUpdates": {"packageCommand": 2},
			"Update": {"packageCommand": 2}, "UpdateAll": {"packageCommand": 2}, "Search": {"packageCommand": 1},
			"ListInstalled": {"packageCommand": 1}, "InstallStreaming": {"runner.RunStreaming": 1, "runner.RunStreamingWithSudo": 1},
			"UpdateStreaming":    {"runner.RunStreaming": 1, "runner.RunStreamingWithSudo": 1},
			"UpdateAllStreaming": {"runner.RunStreaming": 1, "runner.RunStreamingWithSudo": 1},
		}},
	}
	for _, test := range tests {
		t.Run(test.file, func(t *testing.T) {
			source, err := parser.ParseFile(token.NewFileSet(), test.file, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			observed := make(map[string]map[string]int)
			allowedBuilderPositions := make(map[token.Pos]bool)
			for _, declaration := range source.Decls {
				function, ok := declaration.(*ast.FuncDecl)
				if !ok || function.Body == nil {
					continue
				}
				ast.Inspect(function.Body, func(node ast.Node) bool {
					call, ok := node.(*ast.CallExpr)
					if !ok {
						return true
					}
					builder := managerBuilderName(call)
					if builder == "" {
						return true
					}
					allowedBuilderPositions[call.Fun.Pos()] = true
					if observed[function.Name.Name] == nil {
						observed[function.Name.Name] = make(map[string]int)
					}
					observed[function.Name.Name][builder]++
					targetIndex := map[string]int{"packageCommand": 1, "packageCommandWithContext": 2, "exec.CommandContext": 1, "runner.RunStreaming": 1, "runner.RunStreamingWithSudo": 1}[builder]
					if len(call.Args) <= targetIndex {
						t.Errorf("%s has malformed %s builder", test.file, builder)
						return true
					}
					target := call.Args[targetIndex]
					if exactManagerExecutableCall(target, test.receiver) {
						return true
					}
					if literal, ok := stringLiteral(target); ok && test.auxiliary[literal] && builder != "runner.RunStreamingWithSudo" && builder != "exec.CommandContext" {
						return true
					}
					if literal, ok := stringLiteral(target); ok && literal == "sudo" && test.allowSudo {
						if exactSudoManagerChild(call.Args[targetIndex+1:], test.receiver) {
							return true
						}
						t.Errorf("%s %s sudo builder lacks exact-position %s.executablePath child", test.file, builder, test.receiver)
						return true
					}
					t.Errorf("%s has unclassified %s target", test.file, builder)
					return true
				})
			}
			if !reflect.DeepEqual(observed, test.expected) {
				t.Errorf("%s builder inventory = %#v, want %#v", test.file, observed, test.expected)
			}
			ast.Inspect(source, func(node ast.Node) bool {
				switch value := node.(type) {
				case *ast.Ident:
					if (value.Name == "packageCommand" || value.Name == "packageCommandWithContext") && !allowedBuilderPositions[value.Pos()] {
						t.Errorf("%s aliases builder identifier %s", test.file, value.Name)
					}
				case *ast.SelectorExpr:
					owner, ok := value.X.(*ast.Ident)
					if !ok {
						return true
					}
					qualified := owner.Name + "." + value.Sel.Name
					if (qualified == "exec.CommandContext" || qualified == "runner.RunStreaming" || qualified == "runner.RunStreamingWithSudo") && !allowedBuilderPositions[value.Pos()] {
						t.Errorf("%s aliases builder %s", test.file, qualified)
					}
					if qualified == "exec.Command" || qualified == "exec.Cmd" || qualified == "os.StartProcess" || qualified == "syscall.Exec" || qualified == "syscall.ForkExec" || qualified == "syscall.StartProcess" {
						t.Errorf("%s uses unapproved process API %s", test.file, qualified)
					}
				}
				return true
			})
		})
	}
}

func TestManagerExecutableIdentitySudoChildMustBeFirst(t *testing.T) {
	parsed, err := parser.ParseExpr(`packageCommand(timeout, "sudo", "apt", "install", a.executablePath())`)
	if err != nil {
		t.Fatal(err)
	}
	call := parsed.(*ast.CallExpr)
	if exactSudoManagerChild(call.Args[2:], "a") {
		t.Fatal("sudo classifier accepted literal manager with captured identity only in a later argument")
	}
}

func TestManagerExecutableIdentityBrewExecutionIgnoresHostilePATH(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "capture.log")
	t.Setenv("MANAGER_CAPTURE_LOG", logPath)
	captured := writeCapturedManagerExecutable(t, "brew")
	manager := newBrewManager(func(string) (string, error) { return captured, nil })
	hostileDir := installHostileManagerPath(t, "brew")
	t.Setenv("PATH", hostileDir)

	resetManagerCaptureLog(t, logPath)
	if err := manager.Install("git"); err != nil {
		t.Fatal(err)
	}
	requireManagerCaptureLog(t, logPath, "target:"+captured+" <install> <git>")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resetManagerCaptureLog(t, logPath)
	command, err := manager.InstallStreaming(ctx, "zsh")
	waitManagerStreaming(t, command, err)
	requireManagerCaptureLog(t, logPath, "target:"+captured+" <install> <zsh>")

	resetManagerCaptureLog(t, logPath)
	command, err = manager.InstallCasksStreaming(ctx, "ghostty")
	waitManagerStreaming(t, command, err)
	requireManagerCaptureLog(t, logPath, "target:"+captured+" <install> <--cask> <ghostty>")
}

func TestManagerExecutableIdentityAptExecutionUsesCapturedChild(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "capture.log")
	t.Setenv("MANAGER_CAPTURE_LOG", logPath)
	captured := writeCapturedManagerExecutable(t, "apt")
	manager := newAptManager(func(string) (string, error) { return captured, nil })
	hostileDir := installHostileManagerPath(t, "apt")
	installFakeSudo(t, hostileDir)
	t.Setenv("PATH", hostileDir)

	resetManagerCaptureLog(t, logPath)
	if _, err := manager.Search("git"); err != nil {
		t.Fatal(err)
	}
	requireManagerCaptureLog(t, logPath, "target:"+captured+" <search> <git>")

	resetManagerCaptureLog(t, logPath)
	if err := manager.Install("git"); err != nil {
		t.Fatal(err)
	}
	requireManagerCaptureLog(t, logPath,
		"sudo <"+captured+"> <install> <-y> <git>",
		"target:"+captured+" <install> <-y> <git>",
	)

	resetManagerCaptureLog(t, logPath)
	if err := manager.Update("tmux"); err != nil {
		t.Fatal(err)
	}
	requireManagerCaptureLog(t, logPath,
		"sudo <-n> <"+captured+"> <update>",
		"target:"+captured+" <update>",
		"sudo <"+captured+"> <install> <-y> <tmux>",
		"target:"+captured+" <install> <-y> <tmux>",
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resetManagerCaptureLog(t, logPath)
	command, err := manager.UpdateStreaming(ctx, "zsh")
	waitManagerStreaming(t, command, err)
	requireManagerCaptureLog(t, logPath,
		"sudo <-n> <"+captured+"> <update>",
		"target:"+captured+" <update>",
		"sudo <"+captured+"> <install> <-y> <zsh>",
		"target:"+captured+" <install> <-y> <zsh>",
	)
}

func TestManagerExecutableIdentityPacmanAndParuSudoRouting(t *testing.T) {
	t.Run("pacman uninstall uses captured sudo child", func(t *testing.T) {
		logPath := filepath.Join(t.TempDir(), "capture.log")
		t.Setenv("MANAGER_CAPTURE_LOG", logPath)
		captured := writeCapturedManagerExecutable(t, "pacman")
		manager := newPacmanManager(false, func(string) (string, error) { return captured, nil })
		hostileDir := installHostileManagerPath(t, "pacman")
		installFakeSudo(t, hostileDir)
		t.Setenv("PATH", hostileDir)

		resetManagerCaptureLog(t, logPath)
		if _, err := manager.Search("git"); err != nil {
			t.Fatal(err)
		}
		requireManagerCaptureLog(t, logPath, "target:"+captured+" <-Ss> <git>")

		resetManagerCaptureLog(t, logPath)
		if err := manager.Install("git"); err != nil {
			t.Fatal(err)
		}
		requireManagerCaptureLog(t, logPath,
			"sudo <"+captured+"> <-S> <--noconfirm> <--needed> <git>",
			"target:"+captured+" <-S> <--noconfirm> <--needed> <git>",
		)

		resetManagerCaptureLog(t, logPath)
		if err := manager.Uninstall("git"); err != nil {
			t.Fatal(err)
		}
		requireManagerCaptureLog(t, logPath,
			"sudo <"+captured+"> <-R> <--noconfirm> <git>",
			"target:"+captured+" <-R> <--noconfirm> <git>",
		)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		resetManagerCaptureLog(t, logPath)
		command, err := manager.InstallStreaming(ctx, "zsh")
		waitManagerStreaming(t, command, err)
		requireManagerCaptureLog(t, logPath,
			"sudo <"+captured+"> <-S> <--noconfirm> <--needed> <zsh>",
			"target:"+captured+" <-S> <--noconfirm> <--needed> <zsh>",
		)
	})

	t.Run("paru is always direct", func(t *testing.T) {
		logPath := filepath.Join(t.TempDir(), "capture.log")
		t.Setenv("MANAGER_CAPTURE_LOG", logPath)
		captured := writeCapturedManagerExecutable(t, "paru")
		manager := newPacmanManager(true, func(name string) (string, error) {
			if name != "paru" {
				t.Fatalf("unexpected fallback %q", name)
			}
			return captured, nil
		})
		hostileDir := installHostileManagerPath(t, "paru")
		installFakeSudo(t, hostileDir)
		t.Setenv("PATH", hostileDir)

		resetManagerCaptureLog(t, logPath)
		if err := manager.Install("git"); err != nil {
			t.Fatal(err)
		}
		requireManagerCaptureLog(t, logPath, "target:"+captured+" <-S> <--noconfirm> <--needed> <git>")

		resetManagerCaptureLog(t, logPath)
		if err := manager.Uninstall("git"); err != nil {
			t.Fatal(err)
		}
		requireManagerCaptureLog(t, logPath, "target:"+captured+" <-R> <--noconfirm> <git>")

		resetManagerCaptureLog(t, logPath)
		if err := manager.Update("git"); err != nil {
			t.Fatal(err)
		}
		requireManagerCaptureLog(t, logPath, "target:"+captured+" <-Syu> <--noconfirm> <git>")

		resetManagerCaptureLog(t, logPath)
		if err := manager.UpdateAll(); err != nil {
			t.Fatal(err)
		}
		requireManagerCaptureLog(t, logPath, "target:"+captured+" <-Syu> <--noconfirm>")

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		resetManagerCaptureLog(t, logPath)
		command, err := manager.InstallStreaming(ctx, "git")
		waitManagerStreaming(t, command, err)
		requireManagerCaptureLog(t, logPath, "target:"+captured+" <-S> <--noconfirm> <--needed> <--skipreview> <--noprovides> <--removemake> <git>")

		resetManagerCaptureLog(t, logPath)
		command, err = manager.UpdateStreaming(ctx, "git")
		waitManagerStreaming(t, command, err)
		requireManagerCaptureLog(t, logPath, "target:"+captured+" <-Syu> <--noconfirm> <--skipreview> <--noprovides> <git>")

		resetManagerCaptureLog(t, logPath)
		command, err = manager.UpdateAllStreaming(ctx)
		waitManagerStreaming(t, command, err)
		requireManagerCaptureLog(t, logPath, "target:"+captured+" <-Syu> <--noconfirm> <--skipreview> <--noprovides>")
	})
}

func exerciseUnavailablePackageManager(t *testing.T, manager PackageManager) {
	t.Helper()
	if manager.IsAvailable() {
		t.Fatal("invalid manager reported available")
	}
	if manager.IsInstalled("git") {
		t.Fatal("invalid manager reported package installed")
	}
	if err := manager.Install("git"); err == nil {
		t.Fatal("invalid manager Install succeeded")
	}
	if err := manager.Uninstall("git"); err == nil {
		t.Fatal("invalid manager Uninstall succeeded")
	}
	if _, err := manager.GetVersion("git"); err == nil {
		t.Fatal("invalid manager GetVersion succeeded")
	}
	if _, err := manager.CheckOutdated(); err == nil {
		t.Fatal("invalid manager CheckOutdated succeeded")
	}
	if err := manager.Update("git"); err == nil {
		t.Fatal("invalid manager Update succeeded")
	}
	if err := manager.UpdateAll(); err == nil {
		t.Fatal("invalid manager UpdateAll succeeded")
	}
	if _, err := manager.Search("git"); err == nil {
		t.Fatal("invalid manager Search succeeded")
	}
	if _, err := manager.ListInstalled(); err == nil {
		t.Fatal("invalid manager ListInstalled succeeded")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for name, result := range map[string]func() (*runner.StreamingCmd, error){
		"install streaming":    func() (*runner.StreamingCmd, error) { return manager.InstallStreaming(ctx, "git") },
		"update streaming":     func() (*runner.StreamingCmd, error) { return manager.UpdateStreaming(ctx, "git") },
		"update all streaming": func() (*runner.StreamingCmd, error) { return manager.UpdateAllStreaming(ctx) },
	} {
		command, err := result()
		if err == nil || command != nil {
			if command != nil {
				command.Cancel()
			}
			t.Fatalf("invalid manager %s returned command=%v error=%v", name, command != nil, err)
		}
	}
}

func TestManagerExecutableIdentityInvalidManagersNeverStartProcesses(t *testing.T) {
	identityPath := writeManagerExecutable(t, "identity-fixture")
	valid, err := ObserveExecutableIdentity(identityPath)
	if err != nil {
		t.Fatal(err)
	}
	invalid := valid
	invalid.digest = "invalid"

	logPath := filepath.Join(t.TempDir(), "sentinel.log")
	t.Setenv("MANAGER_CAPTURE_LOG", logPath)
	hostileDir := installHostileManagerPath(t, "brew", "apt", "pacman", "paru", "sudo", "dpkg", "dpkg-query", "checkupdates")
	t.Setenv("PATH", hostileDir)

	tests := []struct {
		name    string
		manager PackageManager
		extra   func(t *testing.T, ctx context.Context)
	}{
		{
			name: "brew zero", manager: &BrewManager{},
			extra: func(t *testing.T, ctx context.Context) {
				manager := &BrewManager{}
				if manager.IsInstalledContext(ctx, "git") {
					t.Fatal("zero Brew context query succeeded")
				}
				if _, err := manager.CheckOutdatedNonGreedy(); err == nil {
					t.Fatal("zero Brew non-greedy query succeeded")
				}
				if _, err := manager.ListInstalledCasks(); err == nil {
					t.Fatal("zero Brew cask list succeeded")
				}
				if command, err := manager.InstallCasksStreaming(ctx, "ghostty"); err == nil || command != nil {
					t.Fatal("zero Brew cask streaming did not fail closed")
				}
			},
		},
		{
			name: "brew invalid", manager: &BrewManager{identity: invalid},
			extra: func(t *testing.T, ctx context.Context) {
				manager := &BrewManager{identity: invalid}
				if manager.IsInstalledContext(ctx, "git") {
					t.Fatal("invalid Brew context query succeeded")
				}
				if _, err := manager.CheckOutdatedNonGreedy(); err == nil {
					t.Fatal("invalid Brew non-greedy query succeeded")
				}
				if _, err := manager.ListInstalledCasks(); err == nil {
					t.Fatal("invalid Brew cask list succeeded")
				}
				if command, err := manager.InstallCasksStreaming(ctx, "ghostty"); err == nil || command != nil {
					t.Fatal("invalid Brew cask streaming did not fail closed")
				}
			},
		},
		{name: "apt zero", manager: &AptManager{}, extra: func(t *testing.T, ctx context.Context) {
			if (&AptManager{}).IsInstalledContext(ctx, "git") {
				t.Fatal("zero Apt context query succeeded")
			}
		}},
		{name: "apt invalid", manager: &AptManager{identity: invalid}, extra: func(t *testing.T, ctx context.Context) {
			if (&AptManager{identity: invalid}).IsInstalledContext(ctx, "git") {
				t.Fatal("invalid Apt context query succeeded")
			}
		}},
		{name: "pacman zero", manager: &PacmanManager{}, extra: func(t *testing.T, ctx context.Context) {
			if (&PacmanManager{}).IsInstalledContext(ctx, "git") {
				t.Fatal("zero Pacman context query succeeded")
			}
		}},
		{name: "pacman invalid", manager: &PacmanManager{identity: invalid}, extra: func(t *testing.T, ctx context.Context) {
			if (&PacmanManager{identity: invalid}).IsInstalledContext(ctx, "git") {
				t.Fatal("invalid Pacman context query succeeded")
			}
		}},
		{name: "paru zero", manager: &PacmanManager{useParu: true}, extra: func(t *testing.T, ctx context.Context) {
			if (&PacmanManager{useParu: true}).IsInstalledContext(ctx, "git") {
				t.Fatal("zero Paru context query succeeded")
			}
		}},
		{name: "paru invalid", manager: &PacmanManager{identity: invalid, useParu: true}, extra: func(t *testing.T, ctx context.Context) {
			if (&PacmanManager{identity: invalid, useParu: true}).IsInstalledContext(ctx, "git") {
				t.Fatal("invalid Paru context query succeeded")
			}
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resetManagerCaptureLog(t, logPath)
			exerciseUnavailablePackageManager(t, test.manager)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			test.extra(t, ctx)
			content, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatal(err)
			}
			if len(content) != 0 {
				t.Fatalf("invalid manager started PATH process: %q", content)
			}
		})
	}
}
