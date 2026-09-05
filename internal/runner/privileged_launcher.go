package runner

import (
	"context"
	"os/exec"
)

// Resolvers are passed explicitly to the private factory. Production always
// binds the real trusted-file validators; tests need no global trust bypass.
type privilegedLauncherResolvers struct {
	target     func(string) (string, error)
	sudo       func() (string, error)
	supervisor func() (string, error)
}

func defaultPrivilegedLauncherResolvers() privilegedLauncherResolvers {
	return privilegedLauncherResolvers{
		target: validatePrivilegedTarget,
		sudo: func() (string, error) {
			return findTrustedPrivilegedExecutable("sudo")
		},
		supervisor: currentPrivilegedSupervisorExecutable,
	}
}

func buildPrivilegedLauncher(ctx context.Context, name string, args []string, resolvers privilegedLauncherResolvers) (*exec.Cmd, error) {
	if ctx == nil {
		return nil, errInvalidPrivilegedRequest
	}
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	target, err := resolvers.target(name)
	if err != nil {
		return nil, errInvalidPrivilegedRequest
	}
	sudo, err := resolvers.sudo()
	if err != nil {
		return nil, errPrivilegedSupervisorUnavailable
	}
	supervisor, err := resolvers.supervisor()
	if err != nil {
		return nil, errPrivilegedSupervisorUnavailable
	}
	if !validPrivilegedArguments(args) || !validPrivilegedCommand(target, args) {
		return nil, errInvalidPrivilegedRequest
	}
	sudoArgs := []string{"-n", "--", supervisor, privilegedSupervisorDispatchArg, target}
	sudoArgs = append(sudoArgs, args...)
	// #nosec G204 -- Production resolvers validate exact absolute sudo, supervisor and target paths; arguments remain literal argv and never enter a shell.
	//nolint:noctx // The caller's streaming lifecycle and supervisor control pipe own cancellation and cleanup.
	command := exec.Command(sudo, sudoArgs...)
	command.Env = privilegedLauncherEnvironment()
	command.Dir = "/"
	return command, nil
}
