package runner

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	privilegedSupervisorDispatchArg = "__dotfiles_privileged_supervisor_v1"
	maxPrivilegedArguments          = 256
	maxPrivilegedArgumentBytes      = 64 << 10
)

var (
	errInvalidPrivilegedRequest           = errors.New("invalid privileged execution request")
	errPrivilegedSupervisorUnavailable    = errors.New("privileged supervisor unavailable")
	errPrivilegedSupervisorCleanup        = errors.New("privileged supervisor failed to clean descendants")
	acceptedPrivilegedExecutableBasenames = map[string]struct{}{
		"apt": {}, "pacman": {},
	}
)

func privilegedLauncherEnvironment() []string {
	return []string{
		"PATH=/usr/sbin:/usr/bin:/sbin:/bin",
		"LANG=C",
		"LC_ALL=C",
	}
}

func privilegedTargetEnvironment() []string {
	return []string{
		"PATH=/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME=/root",
		"USER=root",
		"LOGNAME=root",
		"SHELL=/bin/sh",
		"LANG=C",
		"LC_ALL=C",
		"DEBIAN_FRONTEND=noninteractive",
	}
}

// DispatchPrivilegedSupervisor recognizes only the private supervisor argv.
// It is called before the normal root-user refusal so this one bounded helper
// mode can run beneath sudo without exposing the rest of the application as
// root. Non-matching argv is returned to the ordinary CLI unchanged.
func DispatchPrivilegedSupervisor(args []string, control io.Reader, stdout, stderr io.Writer) (bool, int) {
	if len(args) == 0 || args[0] != privilegedSupervisorDispatchArg {
		return false, 0
	}
	return true, runPrivilegedSupervisor(args[1:], control, stdout, stderr)
}

func validPrivilegedArguments(args []string) bool {
	if len(args) > maxPrivilegedArguments {
		return false
	}
	total := 0
	for _, argument := range args {
		if strings.ContainsRune(argument, 0) {
			return false
		}
		total += len(argument)
		if total > maxPrivilegedArgumentBytes {
			return false
		}
	}
	return true
}

func validPrivilegedCommand(target string, args []string) bool {
	switch filepath.Base(target) {
	case "apt":
		switch {
		case len(args) == 1 && args[0] == "update":
			return true
		case len(args) == 2 && args[0] == "upgrade" && args[1] == "-y":
			return true
		case len(args) > 2 && args[0] == "install" && args[1] == "-y":
			return validPrivilegedPackageNames(args[2:])
		}
	case "pacman":
		switch {
		case len(args) > 3 && args[0] == "-S" && args[1] == "--noconfirm" && args[2] == "--needed":
			return validPrivilegedPackageNames(args[3:])
		case len(args) >= 2 && args[0] == "-Syu" && args[1] == "--noconfirm":
			return validPrivilegedPackageNames(args[2:])
		}
	}
	return false
}

func validPrivilegedPackageNames(packages []string) bool {
	for _, name := range packages {
		if name == "" || len(name) > 255 || !privilegedPackageInitial(name[0]) {
			return false
		}
		for _, character := range name {
			if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' ||
				character >= '0' && character <= '9' || strings.ContainsRune("+-.@_:", character) {
				continue
			}
			return false
		}
	}
	return true
}

func privilegedPackageInitial(character byte) bool {
	return character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9'
}

func validatePrivilegedTarget(path string) (string, error) {
	if !validExactPath(path) || !validPrivilegedArguments([]string{path}) {
		return "", errInvalidPrivilegedRequest
	}
	if _, accepted := acceptedPrivilegedExecutableBasenames[filepath.Base(path)]; !accepted {
		return "", errInvalidPrivilegedRequest
	}
	if err := validateTrustedPrivilegedFile(path, true); err != nil {
		return "", errInvalidPrivilegedRequest
	}
	return path, nil
}

func findTrustedPrivilegedExecutable(name string) (string, error) {
	path, err := execLookPath(name)
	if err != nil || !validExactPath(path) || filepath.Base(path) != name {
		return "", errPrivilegedSupervisorUnavailable
	}
	if err := validateTrustedPrivilegedFile(path, true); err != nil {
		return "", errPrivilegedSupervisorUnavailable
	}
	return path, nil
}

func currentPrivilegedSupervisorExecutable() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", errPrivilegedSupervisorUnavailable
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil || !validExactPath(path) {
		return "", errPrivilegedSupervisorUnavailable
	}
	if err := validateTrustedPrivilegedFile(path, false); err != nil {
		return "", errPrivilegedSupervisorUnavailable
	}
	return path, nil
}
