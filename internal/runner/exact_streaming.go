package runner

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
)

var errInvalidExactStreamingRequest = errors.New("invalid exact streaming request")

// ExactStreamingRequest describes one process without PATH or shell lookup.
type ExactStreamingRequest struct {
	Path string
	Args []string
	Env  []string
	Dir  string
}

// RunExactStreaming validates and clones one exact process request before it
// starts the process and streams its stdout and stderr.
func RunExactStreaming(ctx context.Context, request ExactStreamingRequest) (*StreamingCmd, error) {
	if ctx == nil {
		return nil, errInvalidExactStreamingRequest
	}
	if err := ctx.Err(); err != nil {
		return nil, streamingContextCause(ctx)
	}
	if !validExactPath(request.Path) || !validExactPath(request.Dir) || !validExactStrings(request.Args) || !validExactEnvironment(request.Env) {
		return nil, errInvalidExactStreamingRequest
	}
	args := append([]string(nil), request.Args...)
	environment := append([]string(nil), request.Env...)
	// #nosec G204 -- Path is validated as an exact clean absolute path; Args are literal argv.
	command := exec.Command(request.Path, args...)
	command.Env = environment
	command.Dir = request.Dir
	command.Stdin = nil
	return startStreamingLifecycle(ctx, command)
}

func validExactPath(value string) bool {
	return value != "" && !strings.ContainsRune(value, 0) && filepath.IsAbs(value) && filepath.Clean(value) == value
}

func validExactStrings(values []string) bool {
	for _, value := range values {
		if strings.ContainsRune(value, 0) {
			return false
		}
	}
	return true
}

func validExactEnvironment(values []string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		key, _, ok := strings.Cut(value, "=")
		folded := strings.ToLower(key)
		if !ok || key == "" || strings.ContainsRune(value, 0) {
			return false
		}
		if _, duplicate := seen[folded]; duplicate {
			return false
		}
		seen[folded] = struct{}{}
	}
	return true
}
