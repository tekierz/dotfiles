package pkg

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/tekierz/dotfiles/internal/runner"
)

const (
	currentNPMExecutionIdentitySchemaVersion = 2
	maxNPMExecutionShebangBytes              = 64
	npmEnvNodeShebang                        = "#!/usr/bin/env node\n"
)

type npmNodeNativeFormat uint8

const (
	npmNodeNativeFormatInvalid npmNodeNativeFormat = iota
	npmNodeNativeFormatELF
	npmNodeNativeFormatMachO32
	npmNodeNativeFormatMachO64
	npmNodeNativeFormatFat32
	npmNodeNativeFormatFat64
)

var (
	errNPMExecutionIdentityObservation = errors.New("npm execution identity: observation failed")
	errNPMExecutionIdentityShebang     = errors.New("npm execution identity: unsupported shebang")
	errNPMExecutionIdentityChanged     = errors.New("npm execution identity: chain changed")
	errNPMExecutionIdentityInvalid     = errors.New("npm execution identity: invalid snapshot")
	errNPMExecutionIdentityJSON        = errors.New("npm execution identity: JSON encoding refused")
	errNPMExecutionEnvironment         = errors.New("npm execution identity: unsafe environment")
	errNPMExecutionStart               = errors.New("npm execution identity: process start failed")
	errNPMExecutionTerminal            = errors.New("npm execution identity: process failed")
)

// NPMExecutionIdentity is an opaque, immutable observation of one exact npm
// script and its direct Node interpreter with a narrow start capability. PATH
// use by lifecycle code and runtime packages, network results, child commands,
// loaders, libraries, and the revalidation-to-spawn race remain unbound.
// Native format recognition is not Node provenance or authenticity evidence.
type NPMExecutionIdentity struct {
	schemaVersion int
	npm           ExecutableIdentity
	node          ExecutableIdentity
	nodeFormat    npmNodeNativeFormat
	digest        string
}

// SchemaVersion returns the aggregate observation schema version.
func (identity NPMExecutionIdentity) SchemaVersion() int { return identity.schemaVersion }

// Digest returns the aggregate lowercase SHA-256 identity digest.
func (identity NPMExecutionIdentity) Digest() string { return identity.digest }

// Revalidate checks the two stored invocation paths without PATH lookup or
// interpreter rediscovery.
func (identity NPMExecutionIdentity) Revalidate() error {
	if !validNPMExecutionIdentity(identity) {
		return errNPMExecutionIdentityInvalid
	}
	if identity.npm.Revalidate() != nil || identity.node.Revalidate() != nil {
		return errNPMExecutionIdentityChanged
	}
	return nil
}

// String returns a path-free diagnostic representation.
func (identity NPMExecutionIdentity) String() string {
	if !validNPMExecutionIdentity(identity) {
		return "npm_execution_identity<invalid>"
	}
	return "npm_execution_identity<v2:" + identity.digest + ">"
}

// GoString returns a path-free Go-syntax diagnostic representation.
func (identity NPMExecutionIdentity) GoString() string { return identity.String() }

// MarshalJSON refuses serialization because component paths and metadata are
// private authority inputs with no reviewed public projection.
func (identity NPMExecutionIdentity) MarshalJSON() ([]byte, error) {
	return nil, errNPMExecutionIdentityJSON
}

// ObserveNPMExecutionIdentity captures npm, verifies its exact env-node
// shebang with a bounded read, confirms npm did not change during that read,
// and only then captures the explicit Node interpreter.
func ObserveNPMExecutionIdentity(npmPath, nodePath string) (NPMExecutionIdentity, error) {
	firstNPM, err := ObserveExecutableIdentity(npmPath)
	if err != nil {
		return NPMExecutionIdentity{}, errNPMExecutionIdentityObservation
	}
	line, err := readNPMExecutionShebang(firstNPM)
	if err != nil {
		return NPMExecutionIdentity{}, errNPMExecutionIdentityObservation
	}
	if !bytes.Equal(line, []byte(npmEnvNodeShebang)) {
		return NPMExecutionIdentity{}, errNPMExecutionIdentityShebang
	}
	secondNPM, err := ObserveExecutableIdentity(npmPath)
	if err != nil || secondNPM.Digest() != firstNPM.Digest() {
		return NPMExecutionIdentity{}, errNPMExecutionIdentityChanged
	}
	firstNode, err := ObserveExecutableIdentity(nodePath)
	if err != nil {
		return NPMExecutionIdentity{}, errNPMExecutionIdentityObservation
	}
	nodeFormat, err := readNPMExecutionNodeFormat(firstNode)
	if err != nil {
		return NPMExecutionIdentity{}, errNPMExecutionIdentityObservation
	}
	secondNode, err := ObserveExecutableIdentity(nodePath)
	if err != nil || secondNode.Digest() != firstNode.Digest() {
		return NPMExecutionIdentity{}, errNPMExecutionIdentityChanged
	}
	if sameNPMExecutionComponent(secondNPM, secondNode) {
		return NPMExecutionIdentity{}, errNPMExecutionIdentityObservation
	}
	identity := NPMExecutionIdentity{
		schemaVersion: currentNPMExecutionIdentitySchemaVersion,
		npm:           secondNPM,
		node:          secondNode,
		nodeFormat:    nodeFormat,
	}
	identity.digest = npmExecutionIdentityDigest(identity)
	if !validNPMExecutionIdentity(identity) {
		return NPMExecutionIdentity{}, errNPMExecutionIdentityInvalid
	}
	return identity, nil
}

func readNPMExecutionNodeFormat(identity ExecutableIdentity) (npmNodeNativeFormat, error) {
	if !validExecutableIdentity(identity) {
		return npmNodeNativeFormatInvalid, errNPMExecutionIdentityInvalid
	}
	fd, err := syscall.Open(identity.canonicalPath, syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return npmNodeNativeFormatInvalid, errNPMExecutionIdentityObservation
	}
	file := os.NewFile(uintptr(fd), "npm-execution-node-format")
	if file == nil {
		_ = syscall.Close(fd)
		return npmNodeNativeFormatInvalid, errNPMExecutionIdentityObservation
	}
	defer func() { _ = file.Close() }()
	before, err := file.Stat()
	if err != nil || !npmExecutionIdentityMatchesFile(identity, before) {
		return npmNodeNativeFormatInvalid, errNPMExecutionIdentityChanged
	}
	var magic [4]byte
	if _, err := io.ReadFull(file, magic[:]); err != nil {
		return npmNodeNativeFormatInvalid, errNPMExecutionIdentityObservation
	}
	after, err := file.Stat()
	if err != nil || !npmExecutionIdentityMatchesFile(identity, after) {
		return npmNodeNativeFormatInvalid, errNPMExecutionIdentityChanged
	}
	switch magic {
	case [4]byte{0x7f, 'E', 'L', 'F'}:
		return npmNodeNativeFormatELF, nil
	case [4]byte{0xfe, 0xed, 0xfa, 0xce}, [4]byte{0xce, 0xfa, 0xed, 0xfe}:
		return npmNodeNativeFormatMachO32, nil
	case [4]byte{0xfe, 0xed, 0xfa, 0xcf}, [4]byte{0xcf, 0xfa, 0xed, 0xfe}:
		return npmNodeNativeFormatMachO64, nil
	case [4]byte{0xca, 0xfe, 0xba, 0xbe}, [4]byte{0xbe, 0xba, 0xfe, 0xca}:
		return npmNodeNativeFormatFat32, nil
	case [4]byte{0xca, 0xfe, 0xba, 0xbf}, [4]byte{0xbf, 0xba, 0xfe, 0xca}:
		return npmNodeNativeFormatFat64, nil
	default:
		return npmNodeNativeFormatInvalid, errNPMExecutionIdentityObservation
	}
}

func npmExecutionIdentityMatchesFile(identity ExecutableIdentity, info os.FileInfo) bool {
	device, inode, ok := executableFileIdentity(info)
	return ok && executableInfoCategory(info) == nil && info.Size() == identity.size && uint32(info.Mode()) == identity.mode &&
		device == identity.device && inode == identity.inode
}

func readNPMExecutionShebang(identity ExecutableIdentity) ([]byte, error) {
	if !validExecutableIdentity(identity) {
		return nil, errNPMExecutionIdentityInvalid
	}
	fd, err := syscall.Open(identity.canonicalPath, syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, errNPMExecutionIdentityObservation
	}
	file := os.NewFile(uintptr(fd), "npm-execution-shebang")
	if file == nil {
		_ = syscall.Close(fd)
		return nil, errNPMExecutionIdentityObservation
	}
	defer func() { _ = file.Close() }()
	opened, err := file.Stat()
	if err != nil {
		return nil, errNPMExecutionIdentityObservation
	}
	device, inode, ok := executableFileIdentity(opened)
	if !ok || executableInfoCategory(opened) != nil || opened.Size() != identity.size || uint32(opened.Mode()) != identity.mode ||
		device != identity.device || inode != identity.inode {
		return nil, errNPMExecutionIdentityChanged
	}
	buffer := make([]byte, maxNPMExecutionShebangBytes)
	count, readErr := file.Read(buffer)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return nil, errNPMExecutionIdentityObservation
	}
	if count == 0 {
		return nil, errNPMExecutionIdentityObservation
	}
	if newline := bytes.IndexByte(buffer[:count], '\n'); newline >= 0 {
		return buffer[:newline+1], nil
	}
	return nil, errNPMExecutionIdentityShebang
}

func validNPMExecutionIdentity(identity NPMExecutionIdentity) bool {
	if identity.schemaVersion != currentNPMExecutionIdentitySchemaVersion ||
		!validExecutableIdentity(identity.npm) || !validExecutableIdentity(identity.node) ||
		identity.nodeFormat == npmNodeNativeFormatInvalid || identity.nodeFormat > npmNodeNativeFormatFat64 ||
		sameNPMExecutionComponent(identity.npm, identity.node) || !validLowerHexDigest(identity.digest) {
		return false
	}
	return npmExecutionIdentityDigest(identity) == identity.digest
}

func sameNPMExecutionComponent(first, second ExecutableIdentity) bool {
	return first.canonicalPath == second.canonicalPath || first.device == second.device && first.inode == second.inode
}

// NPMStreamingCommand is a path-private streaming process facade.
type NPMStreamingCommand struct {
	output    <-chan string
	completed chan struct{}
	err       error
	cancel    func()
}

func (stream *NPMStreamingCommand) Output() <-chan string { return stream.output }
func (stream *NPMStreamingCommand) Cancel()               { stream.cancel() }
func (stream *NPMStreamingCommand) Wait() error           { <-stream.completed; return stream.err }
func (stream *NPMStreamingCommand) Done() <-chan error {
	done := make(chan error, 1)
	go func() { done <- stream.Wait(); close(done) }()
	return done
}

// StartStreaming revalidates and directly starts the accepted canonical Node
// path with the accepted canonical npm script as argv[1]. No PATH, shell, or
// shebang lookup selects either accepted component. Lifecycle PATH use and all
// transitive runtime/network authority described on NPMExecutionIdentity remain.
func (identity NPMExecutionIdentity) StartStreaming(ctx context.Context, args ...string) (*NPMStreamingCommand, error) {
	if ctx == nil {
		return nil, errNPMExecutionStart
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	environment, ok := npmExecutionEnvironment(os.Environ())
	if !ok {
		return nil, errNPMExecutionEnvironment
	}
	if identity.Revalidate() != nil {
		return nil, errNPMExecutionIdentityChanged
	}
	arguments := make([]string, 1, len(args)+1)
	arguments[0] = identity.npm.canonicalPath
	arguments = append(arguments, args...)
	command, err := runner.RunExactStreaming(ctx, runner.ExactStreamingRequest{
		Path: identity.node.canonicalPath, Args: arguments, Env: environment, Dir: "/",
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, errNPMExecutionStart
	}
	stream := &NPMStreamingCommand{output: command.Output, completed: make(chan struct{}), cancel: command.Cancel}
	go func() {
		stream.err = npmExecutionTerminalError(<-command.Done)
		close(stream.completed)
	}()
	return stream, nil
}

func npmExecutionTerminalError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, context.Canceled):
		return context.Canceled
	case errors.Is(err, context.DeadlineExceeded):
		return context.DeadlineExceeded
	default:
		return errNPMExecutionTerminal
	}
}

func npmExecutionEnvironment(inherited []string) ([]string, bool) {
	result := make([]string, 0, len(inherited)+2)
	seen := make(map[string]struct{}, len(inherited))
	for _, entry := range inherited {
		key, _, ok := strings.Cut(entry, "=")
		folded := strings.ToLower(key)
		if !ok || key == "" || strings.ContainsRune(entry, 0) {
			return nil, false
		}
		if npmExecutionEnvironmentForbidden(folded) {
			continue
		}
		if _, duplicate := seen[folded]; duplicate {
			return nil, false
		}
		seen[folded] = struct{}{}
		result = append(result, entry)
	}
	return append(result, "NPM_CONFIG_USERCONFIG=/dev/null", "NPM_CONFIG_GLOBALCONFIG=/dev/null", "PWD=/"), true
}

func npmExecutionEnvironmentForbidden(key string) bool {
	if strings.HasPrefix(key, "node_") || strings.HasPrefix(key, "npm_config_") || strings.HasPrefix(key, "ld_") || strings.HasPrefix(key, "dyld_") {
		return true
	}
	switch key {
	case "bash_env", "env", "shellopts", "cdpath", "pwd":
		return true
	default:
		return false
	}
}

func npmExecutionIdentityDigest(identity NPMExecutionIdentity) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte("dotfiles.pkg.npm-execution-identity.v2\x00"))
	writeNPMExecutionIdentityField(hash, "schema", []byte(strconv.Itoa(identity.schemaVersion)))
	writeNPMExecutionIdentityField(hash, "npm", []byte(identity.npm.Digest()))
	writeNPMExecutionIdentityField(hash, "node", []byte(identity.node.Digest()))
	writeNPMExecutionIdentityField(hash, "node-format", []byte(strconv.Itoa(int(identity.nodeFormat))))
	return hex.EncodeToString(hash.Sum(nil))
}

type npmExecutionIdentityHashWriter interface {
	Write([]byte) (int, error)
}

func writeNPMExecutionIdentityField(hash npmExecutionIdentityHashWriter, role string, value []byte) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(role)))
	_, _ = hash.Write(length[:])
	_, _ = hash.Write([]byte(role))
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = hash.Write(length[:])
	_, _ = hash.Write(value)
}
