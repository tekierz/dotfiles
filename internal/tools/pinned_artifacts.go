package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/tekierz/dotfiles/internal/operation"
)

const maxPinnedGitDiagnosticBytes = 32 * 1024

type pinnedGitDiagnostic struct{ bytes.Buffer }

func (output *pinnedGitDiagnostic) Write(data []byte) (int, error) {
	written := len(data)
	remaining := maxPinnedGitDiagnosticBytes - output.Len()
	if remaining > 0 {
		if len(data) > remaining {
			data = data[:remaining]
		}
		_, _ = output.Buffer.Write(data)
	}
	return written, nil
}

const (
	tmuxTPMCommit        = "e261deb1b47614eed3400089ce7197dc68acc4eb"
	kickstartNvimCommit  = "f0a2108ed51547793c758d9318bad94f242b22e5"
	lazyVimStarterCommit = "803bc181d7c0d6d5eeba9274d9be49b287294d99"
	nvChadStarterCommit  = "e3572e1f5e1c297212c3deeb17b7863139ce663e"
)

type pinnedGitArtifact struct {
	url     string
	commit  string
	version string
	risk    operation.RemoteArtifactRisk
}

func TPMRemoteArtifact() (operation.RemoteArtifact, error) {
	return newPinnedGitArtifact("config:tmux", ".tmux/plugins/tpm", pinnedGitArtifact{
		url: "https://github.com/tmux-plugins/tpm.git", commit: tmuxTPMCommit,
		version: "commit-e261deb1b476", risk: operation.RemoteArtifactRiskExecutable,
	})
}

func NeovimRemoteArtifact(preset string) (operation.RemoteArtifact, error) {
	var pin pinnedGitArtifact
	switch preset {
	case "kickstart":
		pin = pinnedGitArtifact{url: "https://github.com/nvim-lua/kickstart.nvim.git", commit: kickstartNvimCommit, version: "commit-f0a2108ed515"}
	case "lazyvim":
		pin = pinnedGitArtifact{url: "https://github.com/LazyVim/starter.git", commit: lazyVimStarterCommit, version: "commit-803bc181d7c0"}
	case "nvchad":
		pin = pinnedGitArtifact{url: "https://github.com/NvChad/starter.git", commit: nvChadStarterCommit, version: "commit-e3572e1f5e1c"}
	default:
		return operation.RemoteArtifact{}, fmt.Errorf("refusing unknown Neovim artifact preset %q", preset)
	}
	pin.risk = operation.RemoteArtifactRiskExecutable
	return newPinnedGitArtifact("config:neovim", ".config/nvim", pin)
}

func newPinnedGitArtifact(actionID, destination string, pin pinnedGitArtifact) (operation.RemoteArtifact, error) {
	return operation.NewRemoteArtifact(operation.RemoteArtifactSpec{
		ActionID: actionID, Destination: destination, Source: operation.RemoteArtifactGitRepository,
		URL: pin.url, Version: pin.version, ImmutableRef: pin.commit,
		Verification: operation.RemoteArtifactVerifyGitCommit, Digest: pin.commit, Risk: pin.risk,
	})
}

type pinnedGitCommandFactory func(context.Context, string, ...string) *exec.Cmd

func clonePinnedGitArtifact(ctx context.Context, staging string, artifact operation.RemoteArtifact, commandFactory pinnedGitCommandFactory) error {
	if ctx == nil || staging == "" || commandFactory == nil {
		return fmt.Errorf("pinned artifact staging is unavailable")
	}
	review := artifact.Review()
	if review.SchemaVersion != operation.CurrentRemoteArtifactSchemaVersion || review.Source != operation.RemoteArtifactGitRepository ||
		review.Verification != operation.RemoteArtifactVerifyGitCommit || review.ImmutableRef == "" || review.Digest != review.ImmutableRef {
		return fmt.Errorf("pinned artifact authority is invalid")
	}
	environment := pinnedGitEnvironment(staging)
	commands := [][]string{
		{"init", "--quiet", "."},
		{"remote", "add", "origin", review.URL},
		{"fetch", "--quiet", "--depth=1", "--no-tags", "origin", review.ImmutableRef},
		{"checkout", "--quiet", "--detach", review.ImmutableRef},
	}
	for _, arguments := range commands {
		command := commandFactory(ctx, "git", arguments...)
		command.Dir = staging
		command.Env = environment
		output := &pinnedGitDiagnostic{}
		command.Stdout, command.Stderr = output, output
		if err := command.Run(); err != nil {
			return fmt.Errorf("stage pinned git artifact: %w: %s", err, strings.TrimSpace(output.String()))
		}
	}
	verify := commandFactory(ctx, "git", "rev-parse", "--verify", "HEAD^{commit}")
	verify.Dir = staging
	verify.Env = environment
	output, err := verify.Output()
	if err != nil || strings.TrimSpace(string(output)) != review.ImmutableRef {
		return fmt.Errorf("verify pinned git artifact: %w", errors.Join(err, operation.ErrInvalidRemoteArtifact))
	}
	return nil
}

func pinnedGitEnvironment(staging string) []string {
	result := make([]string, 0, len(os.Environ())+6)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		upper := strings.ToUpper(key)
		if strings.HasPrefix(upper, "GIT_") || upper == "HOME" || upper == "XDG_CONFIG_HOME" || upper == "SSH_ASKPASS" {
			continue
		}
		result = append(result, entry)
	}
	return append(result,
		"HOME="+staging,
		"XDG_CONFIG_HOME="+staging,
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
		"LC_ALL=C",
	)
}
