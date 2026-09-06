package pkg

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestExecutableIdentityValidAndDeterministic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manager")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	first, err := ObserveExecutableIdentity(path)
	if err != nil {
		t.Fatalf("observe first identity: %v", err)
	}
	second, err := ObserveExecutableIdentity(path)
	if err != nil {
		t.Fatalf("observe second identity: %v", err)
	}
	if first.SchemaVersion() != 1 {
		t.Fatalf("schema version = %d, want 1", first.SchemaVersion())
	}
	if first.Digest() == "" || first.Digest() != second.Digest() {
		t.Fatalf("identity digest is empty or nondeterministic: %q != %q", first.Digest(), second.Digest())
	}
	if err := first.Revalidate(); err != nil {
		t.Fatalf("revalidate unchanged identity: %v", err)
	}
}

func expectedExecutableIdentityDigest(invocation, canonical string, size int64, mode uint32, device, inode uint64, content []byte) string {
	contentSum := sha256.Sum256(content)
	digest := sha256.New()
	_, _ = digest.Write([]byte("dotfiles.pkg.executable-identity.v1\x00"))
	fields := [][]byte{
		[]byte("1"), []byte(invocation), []byte(canonical),
		[]byte(strconv.FormatInt(size, 10)), []byte(strconv.FormatUint(uint64(mode), 10)),
		[]byte(strconv.FormatUint(device, 10)), []byte(strconv.FormatUint(inode, 10)), contentSum[:],
	}
	var length [8]byte
	for _, field := range fields {
		binary.BigEndian.PutUint64(length[:], uint64(len(field)))
		_, _ = digest.Write(length[:])
		_, _ = digest.Write(field)
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func TestExecutableIdentityDigestEncodingAndSeparation(t *testing.T) {
	if got := expectedExecutableIdentityDigest("/invocation/tool", "/canonical/tool", 3, 448, 9, 10, []byte("abc")); got != "85d61a5be2036936cb1d7173c59ebad3cccf12e52cfc8fa6128ab34b0dee1abd" {
		t.Fatalf("canonical digest golden = %q", got)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "manager")
	content := []byte("#!/bin/sh\nexit 0\n")
	if err := os.WriteFile(path, content, 0o700); err != nil {
		t.Fatal(err)
	}
	identity, err := ObserveExecutableIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(identity.Digest()) {
		t.Fatalf("digest is not lowercase 64-hex: %q", identity.Digest())
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		t.Fatal(err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatal("executable stat lacks Unix device/inode identity")
	}
	want := expectedExecutableIdentityDigest(path, canonical, info.Size(), uint32(info.Mode()), uint64(int64(stat.Dev)), stat.Ino, content)
	if identity.Digest() != want {
		t.Fatalf("digest = %q, want canonical encoding %q", identity.Digest(), want)
	}

	baseline := identity.Digest()
	alias := filepath.Join(dir, "alias")
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	aliased, err := ObserveExecutableIdentity(alias)
	if err != nil || aliased.Digest() == baseline {
		t.Fatalf("invocation alias did not separate digest: digest=%q err=%v", aliased.Digest(), err)
	}
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
	modeChanged, err := ObserveExecutableIdentity(path)
	if err != nil || modeChanged.Digest() == baseline {
		t.Fatalf("mode did not separate digest: digest=%q err=%v", modeChanged.Digest(), err)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	contentChanged, err := ObserveExecutableIdentity(path)
	if err != nil || contentChanged.Digest() == baseline {
		t.Fatalf("content did not separate digest: digest=%q err=%v", contentChanged.Digest(), err)
	}
	if err := os.WriteFile(path, append(content, '#'), 0o700); err != nil {
		t.Fatal(err)
	}
	sizeChanged, err := ObserveExecutableIdentity(path)
	if err != nil || sizeChanged.Digest() == baseline {
		t.Fatalf("size did not separate digest: digest=%q err=%v", sizeChanged.Digest(), err)
	}

	firstTarget := filepath.Join(dir, "first-target")
	secondTarget := filepath.Join(dir, "second-target")
	for _, target := range []string{firstTarget, secondTarget} {
		if err := os.WriteFile(target, content, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	retargetAlias := filepath.Join(dir, "retarget-alias")
	if err := os.Symlink(firstTarget, retargetAlias); err != nil {
		t.Fatal(err)
	}
	firstTargetIdentity, err := ObserveExecutableIdentity(retargetAlias)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(retargetAlias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secondTarget, retargetAlias); err != nil {
		t.Fatal(err)
	}
	secondTargetIdentity, err := ObserveExecutableIdentity(retargetAlias)
	if err != nil || secondTargetIdentity.Digest() == firstTargetIdentity.Digest() {
		t.Fatalf("canonical target did not separate digest: digest=%q err=%v", secondTargetIdentity.Digest(), err)
	}
}

func TestExecutableIdentityRejectsRelativePath(t *testing.T) {
	if _, err := ObserveExecutableIdentity("relative/manager"); err == nil {
		t.Fatal("relative executable path was accepted")
	}
}

func TestExecutableIdentityRefusesJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manager")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	identity, err := ObserveExecutableIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := json.Marshal(identity); err == nil {
		t.Fatal("executable identity unexpectedly marshaled to JSON")
	}
}

func TestExecutableIdentityRejectsInvalidPaths(t *testing.T) {
	dir := t.TempDir()
	tests := map[string]string{
		"empty":          "",
		"relative":       "relative/manager",
		"unclean":        dir + string(os.PathSeparator) + "nested/../manager",
		"nul":            dir + string(os.PathSeparator) + "manager\x00suffix",
		"control":        dir + string(os.PathSeparator) + "manager\nsecret",
		"invalid utf8":   dir + string(os.PathSeparator) + string([]byte{0xff}),
		"overlong":       dir + string(os.PathSeparator) + strings.Repeat("a", 4096),
		"missing target": filepath.Join(dir, "missing"),
	}

	for name, path := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ObserveExecutableIdentity(path); err == nil {
				t.Fatalf("invalid executable path was accepted")
			}
		})
	}
}

func TestExecutableIdentityRejectsDirectoryAndFIFOBeforeOpen(t *testing.T) {
	dir := t.TempDir()
	if _, err := ObserveExecutableIdentity(dir); err == nil {
		t.Fatal("directory was accepted as an executable")
	}

	fifo := filepath.Join(dir, "manager-fifo")
	if err := syscall.Mkfifo(fifo, 0o700); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := ObserveExecutableIdentity(fifo)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("FIFO was accepted as an executable")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("FIFO validation blocked while opening a non-regular file")
	}
}

func TestExecutableIdentityRejectsInvalidExecutableFiles(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
		mode    os.FileMode
	}{
		{name: "empty", mode: 0o700},
		{name: "non executable", content: []byte("data"), mode: 0o600},
		{name: "setuid", content: []byte("data"), mode: 0o700 | os.ModeSetuid},
		{name: "setgid", content: []byte("data"), mode: 0o700 | os.ModeSetgid},
		{name: "sticky", content: []byte("data"), mode: 0o700 | os.ModeSticky},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "manager")
			if err := os.WriteFile(path, test.content, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, test.mode); err != nil {
				t.Fatal(err)
			}
			special := test.mode & (os.ModeSetuid | os.ModeSetgid | os.ModeSticky)
			if special != 0 {
				info, err := os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				if info.Mode()&special == 0 {
					t.Skip("test filesystem did not retain the requested special mode bit")
				}
			}
			if _, err := ObserveExecutableIdentity(path); err == nil {
				t.Fatalf("invalid executable file was accepted")
			}
		})
	}
}

func TestExecutableIdentityMalformedValuesFailClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manager")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	valid, err := ObserveExecutableIdentity(path)
	if err != nil {
		t.Fatal(err)
	}

	corruptions := map[string]ExecutableIdentity{
		"zero":             {},
		"schema":           valid,
		"invocation path":  valid,
		"canonical path":   valid,
		"size":             valid,
		"mode":             valid,
		"device":           valid,
		"inode":            valid,
		"content digest":   valid,
		"aggregate digest": valid,
	}
	value := corruptions["schema"]
	value.schemaVersion++
	corruptions["schema"] = value
	value = corruptions["invocation path"]
	value.invocationPath = ""
	corruptions["invocation path"] = value
	value = corruptions["canonical path"]
	value.canonicalPath = ""
	corruptions["canonical path"] = value
	value = corruptions["size"]
	value.size++
	corruptions["size"] = value
	value = corruptions["mode"]
	value.mode ^= 1
	corruptions["mode"] = value
	value = corruptions["device"]
	value.device++
	corruptions["device"] = value
	value = corruptions["inode"]
	value.inode++
	corruptions["inode"] = value
	value = corruptions["content digest"]
	value.contentDigest = strings.Repeat("0", 64)
	corruptions["content digest"] = value
	value = corruptions["aggregate digest"]
	value.digest = strings.Repeat("f", 64)
	corruptions["aggregate digest"] = value

	for name, identity := range corruptions {
		t.Run(name, func(t *testing.T) {
			if err := identity.Revalidate(); err == nil {
				t.Fatal("malformed identity revalidated")
			}
		})
	}
}

func TestExecutableIdentityZeroDeviceAndInodeAreValidNumericValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manager")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	identity, err := ObserveExecutableIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	identity.device = 0
	identity.inode = 0
	identity.digest, _ = executableIdentityDigest(identity)
	if !validExecutableIdentity(identity) {
		t.Fatal("numeric zero device/inode values were treated as missing stat identity")
	}
	if err := identity.Revalidate(); !errors.Is(err, errExecutableIdentityChanged) {
		t.Fatalf("zero device/inode snapshot validation result = %v, want changed", err)
	}
}

func TestExecutableIdentityShapeIsOpaqueAndValueOnly(t *testing.T) {
	typeOf := reflect.TypeOf(ExecutableIdentity{})
	want := map[string]reflect.Kind{
		"schemaVersion":  reflect.Int,
		"invocationPath": reflect.String,
		"canonicalPath":  reflect.String,
		"size":           reflect.Int64,
		"mode":           reflect.Uint32,
		"device":         reflect.Uint64,
		"inode":          reflect.Uint64,
		"contentDigest":  reflect.String,
		"digest":         reflect.String,
	}
	if typeOf.NumField() != len(want) {
		t.Fatalf("identity has %d fields, want %d", typeOf.NumField(), len(want))
	}
	for index := 0; index < typeOf.NumField(); index++ {
		field := typeOf.Field(index)
		kind, ok := want[field.Name]
		if !ok || field.Type.Kind() != kind {
			t.Fatalf("unexpected identity field %s %s", field.Name, field.Type)
		}
		if field.PkgPath == "" {
			t.Fatalf("identity field %s is exported", field.Name)
		}
	}
}

func TestExecutableIdentityFailuresReturnZeroValue(t *testing.T) {
	dir := t.TempDir()
	nonExecutable := filepath.Join(dir, "non-executable")
	if err := os.WriteFile(nonExecutable, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{
		"relative":       "relative/manager",
		"missing":        filepath.Join(dir, "missing"),
		"directory":      dir,
		"non executable": nonExecutable,
	} {
		t.Run(name, func(t *testing.T) {
			identity, err := ObserveExecutableIdentity(path)
			if err == nil {
				t.Fatal("invalid observation succeeded")
			}
			if identity != (ExecutableIdentity{}) {
				t.Fatal("failed observation returned partial executable identity")
			}
		})
	}
}

func TestExecutableIdentityFormattingAndErrorsDoNotLeakPaths(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "secret-target-marker")
	alias := filepath.Join(dir, "secret-alias-marker")
	if err := os.WriteFile(target, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, alias); err != nil {
		t.Fatal(err)
	}
	identity, err := ObserveExecutableIdentity(alias)
	if err != nil {
		t.Fatal(err)
	}

	home, _ := os.UserHomeDir()
	forbidden := []string{dir, filepath.Base(alias), filepath.Base(target), home, os.Getenv("USER"), "no such file or directory", "too many levels of symbolic links", "permission denied"}
	assertPrivate := func(t *testing.T, value string) {
		t.Helper()
		lower := strings.ToLower(value)
		for _, secret := range forbidden {
			if secret != "" && strings.Contains(lower, strings.ToLower(secret)) {
				t.Fatalf("private executable identity output leaked forbidden detail")
			}
		}
	}

	for _, rendered := range []string{fmt.Sprintf("%v", identity), fmt.Sprintf("%+v", identity), fmt.Sprintf("%#v", identity), identity.String(), identity.GoString()} {
		assertPrivate(t, rendered)
	}
	if _, jsonErr := json.Marshal(identity); jsonErr == nil {
		t.Fatal("identity unexpectedly marshaled to JSON")
	} else {
		assertPrivate(t, jsonErr.Error())
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := identity.Revalidate(); err == nil {
		t.Fatal("removed target unexpectedly revalidated")
	} else {
		assertPrivate(t, err.Error())
	}
	failed, observeErr := ObserveExecutableIdentity(alias)
	if observeErr == nil || failed != (ExecutableIdentity{}) {
		t.Fatal("dangling private alias did not fail closed")
	}
	assertPrivate(t, observeErr.Error())
}

func TestExecutableIdentityEveryFailureIsZeroAndPrivate(t *testing.T) {
	type failureCase struct {
		path    string
		observe func() (ExecutableIdentity, error)
		skip    bool
	}
	standard := func(path string) func() (ExecutableIdentity, error) {
		return func() (ExecutableIdentity, error) { return ObserveExecutableIdentity(path) }
	}
	makeFile := func(t *testing.T, name string, content []byte, mode os.FileMode) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(path, content, mode); err != nil {
			t.Fatal(err)
		}
		return path
	}

	missing := filepath.Join(t.TempDir(), "secret-missing-manager")
	control := filepath.Join(t.TempDir(), "secret-control\nmanager")
	overlong := string(os.PathSeparator) + strings.Repeat("secret-overlong-", 300)
	directory := filepath.Join(t.TempDir(), "secret-directory-manager")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(t.TempDir(), "secret-fifo-manager")
	if err := syscall.Mkfifo(fifo, 0o700); err != nil {
		t.Fatal(err)
	}
	empty := makeFile(t, "secret-empty-manager", nil, 0o700)
	nonExecutable := makeFile(t, "secret-nonexec-manager", []byte("data"), 0o600)
	special := makeFile(t, "secret-special-manager", []byte("data"), 0o700)
	if err := os.Chmod(special, 0o700|os.ModeSetuid); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(special); err != nil {
		t.Fatal(err)
	} else if info.Mode()&os.ModeSetuid == 0 {
		special = ""
	}
	oversize := makeFile(t, "secret-oversize-manager", []byte("x"), 0o700)
	if err := os.Truncate(oversize, MaxExecutableIdentityBytes+1); err != nil {
		t.Fatal(err)
	}
	symlinkDir := t.TempDir()
	dangling := filepath.Join(symlinkDir, "secret-dangling-manager")
	if err := os.Symlink(filepath.Join(symlinkDir, "secret-missing-target"), dangling); err != nil {
		t.Fatal(err)
	}
	loop := filepath.Join(symlinkDir, "secret-loop-manager")
	if err := os.Symlink(loop, loop); err != nil {
		t.Fatal(err)
	}
	raceDir := t.TempDir()
	raceFirst := makeFile(t, "secret-race-first", []byte("#!/bin/sh\nexit 0\n"), 0o700)
	raceSecond := makeFile(t, "secret-race-second", []byte("#!/bin/sh\nexit 0\n"), 0o700)
	raceAlias := filepath.Join(raceDir, "secret-race-alias")
	if err := os.Symlink(raceFirst, raceAlias); err != nil {
		t.Fatal(err)
	}
	raceObserve := func() (ExecutableIdentity, error) {
		return observeExecutableIdentity(raceAlias, &executableIdentityObservationHooks{afterResolve: func() {
			_ = os.Remove(raceAlias)
			_ = os.Symlink(raceSecond, raceAlias)
		}})
	}

	cases := map[string]failureCase{
		"invalid relative path": {path: "secret-relative-manager", observe: standard("secret-relative-manager")},
		"control path":          {path: control, observe: standard(control)},
		"overlong path":         {path: overlong, observe: standard(overlong)},
		"missing":               {path: missing, observe: standard(missing)},
		"directory":             {path: directory, observe: standard(directory)},
		"fifo":                  {path: fifo, observe: standard(fifo)},
		"empty":                 {path: empty, observe: standard(empty)},
		"non executable":        {path: nonExecutable, observe: standard(nonExecutable)},
		"oversize":              {path: oversize, observe: standard(oversize)},
		"dangling symlink":      {path: dangling, observe: standard(dangling)},
		"symlink loop":          {path: loop, observe: standard(loop)},
		"capture race":          {path: raceAlias, observe: raceObserve},
		"special mode":          {path: special, observe: standard(special), skip: special == ""},
	}

	home, _ := os.UserHomeDir()
	rawPhrases := []string{"no such file or directory", "file name too long", "invalid argument", "too many levels of symbolic links", "permission denied", "operation not permitted", "is a directory"}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			if test.skip {
				t.Skip("test filesystem did not retain the requested special mode bit")
			}
			type result struct {
				identity ExecutableIdentity
				err      error
			}
			done := make(chan result, 1)
			go func() {
				identity, err := test.observe()
				done <- result{identity: identity, err: err}
			}()
			select {
			case got := <-done:
				if got.err == nil || got.identity != (ExecutableIdentity{}) {
					t.Fatal("failure did not return a zero identity and error")
				}
				for _, rendered := range []string{fmt.Sprintf("%v", got.err), fmt.Sprintf("%+v", got.err), fmt.Sprintf("%#v", got.err)} {
					lower := strings.ToLower(rendered)
					for _, forbidden := range append([]string{test.path, filepath.Base(test.path), home, os.Getenv("USER")}, rawPhrases...) {
						if forbidden != "" && len(forbidden) > 3 && strings.Contains(lower, strings.ToLower(forbidden)) {
							t.Fatal("failure formatting leaked path, user, or raw OS detail")
						}
					}
				}
			case <-time.After(500 * time.Millisecond):
				t.Fatal("failure observation blocked")
			}
		})
	}
}

func TestExecutableIdentitySizeLimitIsInclusive(t *testing.T) {
	const maxExecutableSize = 128 << 20
	writeSparseExecutable := func(t *testing.T, size int64) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "manager")
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700)
		if err != nil {
			t.Fatal(err)
		}
		if err := file.Truncate(size); err != nil {
			_ = file.Close()
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		return path
	}

	if _, err := ObserveExecutableIdentity(writeSparseExecutable(t, maxExecutableSize)); err != nil {
		t.Fatalf("exactly 128 MiB executable was rejected: %v", err)
	}
	if _, err := ObserveExecutableIdentity(writeSparseExecutable(t, maxExecutableSize+1)); err == nil {
		t.Fatal("executable larger than 128 MiB was accepted")
	}
}

func TestExecutableIdentityAcceptsCurrentNodeSizedExecutable(t *testing.T) {
	// The largest Node 24.20.0 executable among supported Darwin/Linux amd64
	// and arm64 archives is linux-x64. Sparse extension avoids a fixture-sized
	// allocation while exercising the actual bounded hashing path.
	const nodeExecutableBytes = 126458664
	path := filepath.Join(t.TempDir(), "node")
	if err := os.WriteFile(path, []byte{0x7f, 'E', 'L', 'F'}, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(path, nodeExecutableBytes); err != nil {
		t.Fatal(err)
	}
	identity, err := ObserveExecutableIdentity(path)
	if err != nil {
		t.Fatalf("current Node-sized executable rejected: %v", err)
	}
	if err := identity.Revalidate(); err != nil {
		t.Fatalf("unchanged Node-sized executable rejected: %v", err)
	}
	if err := os.Truncate(path, MaxExecutableIdentityBytes+1); err != nil {
		t.Fatal(err)
	}
	if err := identity.Revalidate(); err == nil {
		t.Fatal("Node-sized executable grown beyond the bound was accepted")
	}
}

func TestExecutableIdentityAcceptsSymlinkAliasesAndBindsInvocationPath(t *testing.T) {
	realDir := t.TempDir()
	target := filepath.Join(realDir, "manager")
	if err := os.WriteFile(target, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	aliasDir := t.TempDir()
	firstAlias := filepath.Join(aliasDir, "manager-one")
	secondAlias := filepath.Join(aliasDir, "manager-two")
	if err := os.Symlink(target, firstAlias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, secondAlias); err != nil {
		t.Fatal(err)
	}
	first, err := ObserveExecutableIdentity(firstAlias)
	if err != nil {
		t.Fatalf("observe first symlink alias: %v", err)
	}
	second, err := ObserveExecutableIdentity(secondAlias)
	if err != nil {
		t.Fatalf("observe second symlink alias: %v", err)
	}
	if first.Digest() == second.Digest() {
		t.Fatal("distinct invocation aliases produced the same identity digest")
	}

	parentAlias := filepath.Join(t.TempDir(), "manager-parent")
	if err := os.Symlink(realDir, parentAlias); err != nil {
		t.Fatal(err)
	}
	if _, err := ObserveExecutableIdentity(filepath.Join(parentAlias, "manager")); err != nil {
		t.Fatalf("observe executable through symlinked parent: %v", err)
	}
}

func TestExecutableIdentityRejectsDanglingAndLoopingSymlinks(t *testing.T) {
	dir := t.TempDir()
	dangling := filepath.Join(dir, "dangling")
	if err := os.Symlink(filepath.Join(dir, "missing"), dangling); err != nil {
		t.Fatal(err)
	}
	if _, err := ObserveExecutableIdentity(dangling); err == nil {
		t.Fatal("dangling executable symlink was accepted")
	}

	loopOne := filepath.Join(dir, "loop-one")
	loopTwo := filepath.Join(dir, "loop-two")
	if err := os.Symlink(loopTwo, loopOne); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(loopOne, loopTwo); err != nil {
		t.Fatal(err)
	}
	if _, err := ObserveExecutableIdentity(loopOne); err == nil {
		t.Fatal("looping executable symlink was accepted")
	}
}

func TestExecutableIdentityRevalidateDetectsDrift(t *testing.T) {
	const firstContent = "#!/bin/sh\nexit 0\n"
	const sameSizeContent = "#!/bin/sh\nexit 1\n"
	if len(firstContent) != len(sameSizeContent) {
		t.Fatal("same-size drift fixture is invalid")
	}

	tests := []struct {
		name  string
		setup func(t *testing.T) (string, func())
	}{
		{
			name: "alias retarget",
			setup: func(t *testing.T) (string, func()) {
				dir := t.TempDir()
				firstTarget := filepath.Join(dir, "manager-one")
				secondTarget := filepath.Join(dir, "manager-two")
				alias := filepath.Join(dir, "manager")
				for _, target := range []string{firstTarget, secondTarget} {
					if err := os.WriteFile(target, []byte(firstContent), 0o700); err != nil {
						t.Fatal(err)
					}
				}
				if err := os.Symlink(firstTarget, alias); err != nil {
					t.Fatal(err)
				}
				return alias, func() {
					if err := os.Remove(alias); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(secondTarget, alias); err != nil {
						t.Fatal(err)
					}
				}
			},
		},
		{
			name: "target replacement",
			setup: func(t *testing.T) (string, func()) {
				dir := t.TempDir()
				target := filepath.Join(dir, "manager")
				replacement := filepath.Join(dir, "replacement")
				for _, path := range []string{target, replacement} {
					if err := os.WriteFile(path, []byte(firstContent), 0o700); err != nil {
						t.Fatal(err)
					}
				}
				return target, func() {
					if err := os.Rename(replacement, target); err != nil {
						t.Fatal(err)
					}
				}
			},
		},
		{
			name: "mode change",
			setup: func(t *testing.T) (string, func()) {
				path := filepath.Join(t.TempDir(), "manager")
				if err := os.WriteFile(path, []byte(firstContent), 0o700); err != nil {
					t.Fatal(err)
				}
				return path, func() {
					if err := os.Chmod(path, 0o755); err != nil {
						t.Fatal(err)
					}
				}
			},
		},
		{
			name: "same-size content rewrite with restored mtime",
			setup: func(t *testing.T) (string, func()) {
				path := filepath.Join(t.TempDir(), "manager")
				if err := os.WriteFile(path, []byte(firstContent), 0o700); err != nil {
					t.Fatal(err)
				}
				info, err := os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				return path, func() {
					if err := os.WriteFile(path, []byte(sameSizeContent), 0o700); err != nil {
						t.Fatal(err)
					}
					if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
						t.Fatal(err)
					}
				}
			},
		},
		{
			name: "size change",
			setup: func(t *testing.T) (string, func()) {
				path := filepath.Join(t.TempDir(), "manager")
				if err := os.WriteFile(path, []byte(firstContent), 0o700); err != nil {
					t.Fatal(err)
				}
				return path, func() {
					file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
					if err != nil {
						t.Fatal(err)
					}
					if _, err := file.WriteString("#"); err != nil {
						_ = file.Close()
						t.Fatal(err)
					}
					if err := file.Close(); err != nil {
						t.Fatal(err)
					}
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path, drift := test.setup(t)
			identity, err := ObserveExecutableIdentity(path)
			if err != nil {
				t.Fatalf("observe identity: %v", err)
			}
			copied := identity
			digest := identity.Digest()
			drift()

			// Revalidate reports drift since observation. It does not authorize a
			// later spawn and does not close the check-to-execution race.
			if err := identity.Revalidate(); err == nil {
				t.Fatal("executable drift was not detected")
			}
			if copied.Digest() != digest || identity.Digest() != digest {
				t.Fatal("copying or revalidating mutated the opaque identity value")
			}
			if err := copied.Revalidate(); err == nil {
				t.Fatal("copied identity did not observe the same executable drift")
			}
		})
	}
}

func TestExecutableIdentityObservationRejectsDeterministicCaptureRaces(t *testing.T) {
	const content = "#!/bin/sh\nexit 0\n"
	assertRejected := func(t *testing.T, path string, hooks *executableIdentityObservationHooks) {
		t.Helper()
		identity, err := observeExecutableIdentity(path, hooks)
		if err == nil {
			t.Fatal("capture-time executable drift was accepted")
		}
		if identity != (ExecutableIdentity{}) {
			t.Fatal("capture-time executable drift returned partial identity")
		}
	}

	t.Run("alias retarget after resolve", func(t *testing.T) {
		dir := t.TempDir()
		first := filepath.Join(dir, "first")
		second := filepath.Join(dir, "second")
		alias := filepath.Join(dir, "manager")
		for _, path := range []string{first, second} {
			if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.Symlink(first, alias); err != nil {
			t.Fatal(err)
		}
		assertRejected(t, alias, &executableIdentityObservationHooks{afterResolve: func() {
			if err := os.Remove(alias); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(second, alias); err != nil {
				t.Fatal(err)
			}
		}})
	})

	t.Run("leaf replacement after open", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "manager")
		replacement := filepath.Join(dir, "replacement")
		for _, path := range []string{target, replacement} {
			if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
				t.Fatal(err)
			}
		}
		assertRejected(t, target, &executableIdentityObservationHooks{afterOpen: func() {
			if err := os.Rename(replacement, target); err != nil {
				t.Fatal(err)
			}
		}})
	})

	t.Run("growth after first read", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "manager")
		if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
			t.Fatal(err)
		}
		assertRejected(t, path, &executableIdentityObservationHooks{afterFirstRead: func() {
			if err := os.Truncate(path, MaxExecutableIdentityBytes+1); err != nil {
				t.Fatal(err)
			}
		}})
	})

	t.Run("leaf replacement after hash", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "manager")
		replacement := filepath.Join(dir, "replacement")
		for _, path := range []string{target, replacement} {
			if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
				t.Fatal(err)
			}
		}
		assertRejected(t, target, &executableIdentityObservationHooks{afterHash: func() {
			if err := os.Rename(replacement, target); err != nil {
				t.Fatal(err)
			}
		}})
	})

	t.Run("symlinked parent retarget after resolve", func(t *testing.T) {
		root := t.TempDir()
		firstDir := filepath.Join(root, "first")
		secondDir := filepath.Join(root, "second")
		for _, dir := range []string{firstDir, secondDir} {
			if err := os.Mkdir(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "manager"), []byte(content), 0o700); err != nil {
				t.Fatal(err)
			}
		}
		parentAlias := filepath.Join(root, "current")
		if err := os.Symlink(firstDir, parentAlias); err != nil {
			t.Fatal(err)
		}
		assertRejected(t, filepath.Join(parentAlias, "manager"), &executableIdentityObservationHooks{afterResolve: func() {
			if err := os.Remove(parentAlias); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(secondDir, parentAlias); err != nil {
				t.Fatal(err)
			}
		}})
	})
}

func TestExecutableIdentityPreOpenTypeSwapNeverBlocks(t *testing.T) {
	tests := []struct {
		name string
		swap func(t *testing.T, target string) error
	}{
		{
			name: "regular to fifo",
			swap: func(_ *testing.T, target string) error {
				if err := os.Remove(target); err != nil {
					return err
				}
				return syscall.Mkfifo(target, 0o700)
			},
		},
		{
			name: "regular to symlink to fifo",
			swap: func(t *testing.T, target string) error {
				fifo := filepath.Join(t.TempDir(), "secret-swap-fifo")
				if err := syscall.Mkfifo(fifo, 0o700); err != nil {
					return err
				}
				if err := os.Remove(target); err != nil {
					return err
				}
				return os.Symlink(fifo, target)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			target := filepath.Join(t.TempDir(), "secret-pre-open-manager")
			if err := os.WriteFile(target, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			var hookErr error
			type result struct {
				identity ExecutableIdentity
				err      error
			}
			done := make(chan result, 1)
			go func() {
				identity, err := observeExecutableIdentity(target, &executableIdentityObservationHooks{beforeOpen: func() {
					hookErr = test.swap(t, target)
				}})
				done <- result{identity: identity, err: err}
			}()
			select {
			case got := <-done:
				if hookErr != nil {
					t.Fatal(hookErr)
				}
				if got.err == nil || got.identity != (ExecutableIdentity{}) {
					t.Fatal("pre-open type swap did not fail closed")
				}
				for _, private := range []string{target, filepath.Base(target), "secret-swap-fifo"} {
					if strings.Contains(strings.ToLower(got.err.Error()), strings.ToLower(private)) {
						t.Fatal("pre-open type-swap error leaked private path material")
					}
				}
			case <-time.After(500 * time.Millisecond):
				t.Fatal("pre-open FIFO swap blocked executable observation")
			}
		})
	}
}
