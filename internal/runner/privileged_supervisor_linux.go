//go:build linux

package runner

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func enablePrivilegedDescendantReaping() error {
	return unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0)
}

func reapPrivilegedDescendants() error {
	return reapPrivilegedChildrenWithDeps(func() (int, error) {
		return syscall.Wait4(-1, nil, syscall.WNOHANG, nil)
	}, killPrivilegedAdoptedDescendants, time.Now, time.Sleep)
}

func killPrivilegedAdoptedDescendants() error {
	childrenFiles, err := filepath.Glob("/proc/self/task/*/children")
	if err != nil {
		return err
	}
	if len(childrenFiles) == 0 {
		return errPrivilegedSupervisorCleanup
	}
	for _, childrenPath := range childrenFiles {
		data, readErr := os.ReadFile(childrenPath) // #nosec G304 -- path comes only from the literal /proc/self/task/*/children kernel glob.
		if readErr != nil {
			return readErr
		}
		for _, field := range strings.Fields(string(data)) {
			pid, parseErr := strconv.Atoi(field)
			if parseErr != nil || pid <= 0 {
				return errPrivilegedSupervisorCleanup
			}
			for {
				err = syscall.Kill(pid, syscall.SIGKILL)
				if errors.Is(err, syscall.EINTR) {
					continue
				}
				if err != nil && !errors.Is(err, syscall.ESRCH) {
					return err
				}
				break
			}
		}
	}
	return nil
}
