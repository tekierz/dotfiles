package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// GetScript dispatch
// ---------------------------------------------------------------------------

func TestGetScriptKnownNames(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"hk":   HKScript,
		"caff": CaffScript,
		"sshh": SSHHScript,
	}
	for name, want := range cases {
		name, want := name, want
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := GetScript(name)
			if got != want {
				t.Fatalf("GetScript(%q) did not return the expected constant", name)
			}
			if got == "" {
				t.Fatalf("GetScript(%q) returned empty content", name)
			}
			if !strings.HasPrefix(got, "#!/usr/bin/env bash") {
				t.Fatalf("GetScript(%q) is missing a bash shebang", name)
			}
		})
	}
}

func TestGetScriptUnknownReturnsEmpty(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"", "unknown", "HK", "Caff", "rm -rf /", "../caff"} {
		if got := GetScript(name); got != "" {
			t.Fatalf("GetScript(%q) = %q, want empty string", name, got)
		}
	}
}

// ---------------------------------------------------------------------------
// 0700 write contract
//
// The production write happens in internal/ui/installation.go (installUtilities)
// with os.WriteFile(scriptPath, []byte(script), 0700). This package only owns
// the *content*. These tests pin the contract that the content this package
// returns is meant to be written owner-only (rwx, no group/other bits) and that
// the OS honors 0700, locking in the hardening from 0755 -> 0700 for private
// per-user executables.
// ---------------------------------------------------------------------------

func TestScriptsWrittenAt0700(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not meaningful on Windows")
	}
	t.Parallel()

	dir := t.TempDir()
	for _, name := range []string{"hk", "caff", "sshh"} {
		content := GetScript(name)
		if content == "" {
			t.Fatalf("GetScript(%q) returned empty", name)
		}
		p := filepath.Join(dir, name)
		// Mirror the production write mode exactly.
		if err := os.WriteFile(p, []byte(content), 0o700); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
		if perm := info.Mode().Perm(); perm != 0o700 {
			t.Fatalf("%s mode = %04o, want 0700 (owner-only; no group/other bits)", name, perm)
		}
		// Explicitly assert no group/other access bits are set.
		if info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("%s has group/other permission bits set: %04o", name, info.Mode().Perm())
		}
	}
}

// ---------------------------------------------------------------------------
// Embedded caffeine PID-file validation logic
//
// The security fix moved the PID file off the predictable, world-writable /tmp
// path and added validation (numeric-only PID + caffeinate/systemd-inhibit
// process check) before killing. We extract the REAL read_pid / is_caffeine
// function bodies from the shipped CaffScript constant and exercise them in a
// controlled harness so the tests track production code, never a copy.
// ---------------------------------------------------------------------------

// extractFunc pulls a bash function body ("name() { ... }") out of the given
// script source by brace-matching, so tests run the exact shipped code.
func extractFunc(t *testing.T, src, fn string) string {
	t.Helper()
	marker := fn + "() {"
	start := strings.Index(src, marker)
	if start < 0 {
		t.Fatalf("function %q not found in script", fn)
	}
	// Walk from the opening brace, tracking depth.
	i := start + len(marker) - 1 // position of '{'
	depth := 0
	for ; i < len(src); i++ {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[start : i+1]
			}
		}
	}
	t.Fatalf("unbalanced braces extracting %q", fn)
	return ""
}

func TestReadPidValidation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash harness not applicable on Windows")
	}
	t.Parallel()

	cases := []struct {
		name        string
		writeFile   bool
		content     string
		wantRC      int    // exit code of `read_pid`
		wantStdout  string // expected echoed pid (when valid)
		mustBeEmpty bool
	}{
		{name: "missing file", writeFile: false, wantRC: 1, mustBeEmpty: true},
		{name: "valid pid", writeFile: true, content: "12345\n", wantRC: 0, wantStdout: "12345"},
		{name: "empty file", writeFile: true, content: "", wantRC: 1, mustBeEmpty: true},
		{name: "garbage", writeFile: true, content: "not-a-pid\n", wantRC: 1, mustBeEmpty: true},
		{name: "leading text", writeFile: true, content: "abc123\n", wantRC: 1, mustBeEmpty: true},
		{name: "injection semicolon", writeFile: true, content: "12; rm -rf /\n", wantRC: 1, mustBeEmpty: true},
		{name: "command substitution", writeFile: true, content: "$(id)\n", wantRC: 1, mustBeEmpty: true},
		{name: "negative", writeFile: true, content: "-1\n", wantRC: 1, mustBeEmpty: true},
		{name: "float", writeFile: true, content: "12.5\n", wantRC: 1, mustBeEmpty: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			pidfile := filepath.Join(dir, "caffeine.pid")
			if tc.writeFile {
				if err := os.WriteFile(pidfile, []byte(tc.content), 0o600); err != nil {
					t.Fatalf("write pidfile: %v", err)
				}
			}

			// Capture stdout of read_pid into a marker so we can assert on it.
			body := `out="$(read_pid)"; rc=$?; printf 'PID=[%s]\n' "$out"; exit $rc`
			cmd := buildHarness(t, pidfile, "", body)
			outBytes, err := cmd.CombinedOutput()
			out := string(outBytes)
			rc := 0
			if err != nil {
				if ee, ok := err.(*exec.ExitError); ok {
					rc = ee.ExitCode()
				} else {
					t.Fatalf("harness error: %v", err)
				}
			}

			if rc != tc.wantRC {
				t.Fatalf("read_pid rc = %d, want %d (out=%q)", rc, tc.wantRC, out)
			}
			if tc.mustBeEmpty {
				if !strings.Contains(out, "PID=[]") {
					t.Fatalf("expected empty PID, got %q", out)
				}
			}
			if tc.wantStdout != "" {
				if !strings.Contains(out, "PID=["+tc.wantStdout+"]") {
					t.Fatalf("expected PID=[%s], got %q", tc.wantStdout, out)
				}
			}
		})
	}
}

// buildHarness constructs (but does not run) a bash -c command with the real
// read_pid + is_caffeine functions sourced against pidfile and optional fakeBin.
func buildHarness(t *testing.T, pidfile, fakeBinDir, body string) *exec.Cmd {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	readPid := extractFunc(t, CaffScript, "read_pid")
	isCaffeine := extractFunc(t, CaffScript, "is_caffeine")
	script := "PIDFILE=\"" + pidfile + "\"\n" + readPid + "\n" + isCaffeine + "\n" + body
	cmd := exec.Command("bash", "-c", script)
	cmd.Env = os.Environ()
	if fakeBinDir != "" {
		cmd.Env = append(cmd.Env, "PATH="+fakeBinDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	return cmd
}

// writeFakePS installs a fake `ps` in a temp dir that emulates
// `ps -p <pid> -o comm=` by echoing FAKE_COMM (or failing if FAKE_PS_FAIL set).
func writeFakePS(t *testing.T, comm string, fail bool) string {
	t.Helper()
	dir := t.TempDir()
	body := "#!/usr/bin/env bash\n"
	if fail {
		body += "exit 1\n"
	} else {
		body += "echo " + shellSingleQuote(comm) + "\n"
	}
	p := filepath.Join(dir, "ps")
	if err := os.WriteFile(p, []byte(body), 0o700); err != nil {
		t.Fatalf("write fake ps: %v", err)
	}
	return dir
}

func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func TestIsCaffeineRefusesUnrelatedProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash harness not applicable on Windows")
	}
	t.Parallel()

	cases := []struct {
		name   string
		comm   string
		psFail bool
		wantRC int // 0 => treated as caffeine (would act), 1 => refuses
	}{
		{name: "caffeinate path", comm: "/usr/bin/caffeinate", wantRC: 0},
		{name: "caffeinate bare", comm: "caffeinate", wantRC: 0},
		{name: "systemd-inhibit", comm: "systemd-inhibit", wantRC: 0},
		{name: "unrelated bash", comm: "bash", wantRC: 1},
		{name: "unrelated sshd", comm: "sshd", wantRC: 1},
		{name: "unrelated init", comm: "launchd", wantRC: 1},
		{name: "empty comm", comm: "", wantRC: 1},
		{name: "ps lookup fails (no such pid)", psFail: true, wantRC: 1},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fakeBin := writeFakePS(t, tc.comm, tc.psFail)
			// is_caffeine takes a pid arg; the fake ps ignores it.
			body := `is_caffeine 99999; exit $?`
			rc := runCaffHarnessWithBin(t, fakeBin, body)
			if rc != tc.wantRC {
				t.Fatalf("is_caffeine rc = %d, want %d (comm=%q, psFail=%v)", rc, tc.wantRC, tc.comm, tc.psFail)
			}
		})
	}
}

// runCaffHarnessWithBin runs the harness with a fake-bin PATH and a dummy
// pidfile (is_caffeine does not read the pidfile, only ps).
func runCaffHarnessWithBin(t *testing.T, fakeBinDir, body string) int {
	t.Helper()
	dir := t.TempDir()
	pidfile := filepath.Join(dir, "caffeine.pid")
	cmd := buildHarness(t, pidfile, fakeBinDir, body)
	out, err := cmd.CombinedOutput()
	t.Logf("harness output: %q", string(out))
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	t.Fatalf("harness error: %v", err)
	return -1
}

// ---------------------------------------------------------------------------
// Embedded-content security invariants
// ---------------------------------------------------------------------------

func TestCaffScriptDoesNotUsePredictableTmpPidPath(t *testing.T) {
	t.Parallel()

	// The hardening moved the PID file off the shared, world-writable /tmp.
	if regexp.MustCompile(`PIDFILE=.*?/tmp/`).MatchString(CaffScript) {
		t.Fatalf("CaffScript PIDFILE still uses a /tmp path; predictable-path hazard reintroduced")
	}
	// It should anchor on a per-user runtime dir.
	if !strings.Contains(CaffScript, "XDG_RUNTIME_DIR") {
		t.Fatalf("CaffScript no longer references XDG_RUNTIME_DIR for the PID file")
	}
	// The PID file write should be umask-protected (owner-only).
	if !strings.Contains(CaffScript, "umask 077") {
		t.Fatalf("CaffScript no longer creates the PID file under umask 077")
	}
}

func TestCaffScriptValidatesBeforeKilling(t *testing.T) {
	t.Parallel()

	// stop() must gate the kill behind is_caffeine, never kill a raw read_pid.
	stop := extractFunc(t, CaffScript, "stop")
	if !strings.Contains(stop, "is_caffeine") {
		t.Fatalf("stop() does not validate the process with is_caffeine before killing")
	}
	if !strings.Contains(stop, "kill ") {
		t.Fatalf("stop() unexpectedly has no kill statement")
	}
	// The numeric guard must be present in read_pid.
	if !strings.Contains(CaffScript, `[[ "$pid" =~ ^[0-9]+$ ]]`) {
		t.Fatalf("read_pid no longer enforces a numeric-only PID")
	}
}
