package pkg

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strconv"
	"syscall"
)

const (
	currentNPMExecutionIdentitySchemaVersion = 1
	maxNPMExecutionShebangBytes              = 64
	npmEnvNodeShebang                        = "#!/usr/bin/env node\n"
)

var (
	errNPMExecutionIdentityObservation = errors.New("npm execution identity: observation failed")
	errNPMExecutionIdentityShebang     = errors.New("npm execution identity: unsupported shebang")
	errNPMExecutionIdentityChanged     = errors.New("npm execution identity: chain changed")
	errNPMExecutionIdentityInvalid     = errors.New("npm execution identity: invalid snapshot")
	errNPMExecutionIdentityJSON        = errors.New("npm execution identity: JSON encoding refused")
)

// NPMExecutionIdentity is an opaque, immutable observation of one exact npm
// script and its direct Node interpreter. It excludes dynamic loaders, shared
// libraries, and runtime-loaded code. It is not spawn authority and does not
// close the remaining revalidation-to-execution race.
type NPMExecutionIdentity struct {
	schemaVersion int
	npm           ExecutableIdentity
	node          ExecutableIdentity
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
	return "npm_execution_identity<v1:" + identity.digest + ">"
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
	node, err := ObserveExecutableIdentity(nodePath)
	if err != nil {
		return NPMExecutionIdentity{}, errNPMExecutionIdentityObservation
	}
	identity := NPMExecutionIdentity{
		schemaVersion: currentNPMExecutionIdentitySchemaVersion,
		npm:           secondNPM,
		node:          node,
	}
	identity.digest = npmExecutionIdentityDigest(identity)
	if !validNPMExecutionIdentity(identity) {
		return NPMExecutionIdentity{}, errNPMExecutionIdentityInvalid
	}
	return identity, nil
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
		!validLowerHexDigest(identity.digest) {
		return false
	}
	return npmExecutionIdentityDigest(identity) == identity.digest
}

func npmExecutionIdentityDigest(identity NPMExecutionIdentity) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte("dotfiles.pkg.npm-execution-identity.v1\x00"))
	writeNPMExecutionIdentityField(hash, "schema", []byte(strconv.Itoa(identity.schemaVersion)))
	writeNPMExecutionIdentityField(hash, "npm", []byte(identity.npm.Digest()))
	writeNPMExecutionIdentityField(hash, "node", []byte(identity.node.Digest()))
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
