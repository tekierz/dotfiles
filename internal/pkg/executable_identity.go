package pkg

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"unicode"
	"unicode/utf8"
)

const (
	// CurrentExecutableIdentitySchemaVersion is the canonical executable snapshot schema.
	CurrentExecutableIdentitySchemaVersion = 1

	// MaxExecutableIdentityBytes is the inclusive observation size limit.
	MaxExecutableIdentityBytes int64 = 64 << 20

	// MaxExecutableIdentityPathBytes is the inclusive path byte limit.
	MaxExecutableIdentityPathBytes = 4096
)

var (
	errExecutableIdentityInvalidPath = errors.New("executable identity: invalid path")
	errExecutableIdentityUnavailable = errors.New("executable identity: target unavailable")
	errExecutableIdentityUnsafe      = errors.New("executable identity: unsafe target")
	errExecutableIdentityTooLarge    = errors.New("executable identity: target too large")
	errExecutableIdentityChanged     = errors.New("executable identity: target changed")
	errExecutableIdentityInvalid     = errors.New("executable identity: invalid snapshot")
	errExecutableIdentityJSON        = errors.New("executable identity: JSON encoding refused")
)

// ExecutableIdentity is an opaque, immutable snapshot used to observe later
// executable drift. It is not provenance or signature evidence and does not
// authorize spawning the executable or close a check-to-execution race. It
// deliberately excludes interpreters, sudo, shared libraries, and other
// transitive execution dependencies.
type ExecutableIdentity struct {
	schemaVersion  int
	invocationPath string
	canonicalPath  string
	size           int64
	mode           uint32
	device         uint64
	inode          uint64
	contentDigest  string
	digest         string
}

// SchemaVersion returns the snapshot schema version.
func (identity ExecutableIdentity) SchemaVersion() int { return identity.schemaVersion }

// Digest returns the aggregate lowercase SHA-256 identity digest.
func (identity ExecutableIdentity) Digest() string { return identity.digest }

// Revalidate observes the invocation path again and reports snapshot drift.
// It is not authorization to execute the observed path.
func (identity ExecutableIdentity) Revalidate() error {
	if !validExecutableIdentity(identity) {
		return errExecutableIdentityInvalid
	}
	current, err := ObserveExecutableIdentity(identity.invocationPath)
	if err != nil || current.Digest() != identity.Digest() {
		return errExecutableIdentityChanged
	}
	return nil
}

// String returns a path-free diagnostic representation.
func (identity ExecutableIdentity) String() string {
	if identity.schemaVersion != CurrentExecutableIdentitySchemaVersion || identity.digest == "" {
		return "executable_identity<invalid>"
	}
	return "executable_identity<v1:" + identity.digest + ">"
}

// GoString returns a path-free Go-syntax diagnostic representation.
func (identity ExecutableIdentity) GoString() string { return identity.String() }

// MarshalJSON refuses serialization because paths and other identity material
// are private inputs and this type has no reviewed public projection.
func (identity ExecutableIdentity) MarshalJSON() ([]byte, error) {
	return nil, errExecutableIdentityJSON
}

type executableIdentityObservationHooks struct {
	afterResolve   func()
	beforeOpen     func()
	afterOpen      func()
	afterFirstRead func()
	afterHash      func()
}

// ObserveExecutableIdentity captures a bounded executable snapshot for later
// drift observation. The result does not authorize a subsequent spawn.
func ObserveExecutableIdentity(path string) (ExecutableIdentity, error) {
	return observeExecutableIdentity(path, nil)
}

func observeExecutableIdentity(path string, hooks *executableIdentityObservationHooks) (ExecutableIdentity, error) {
	if !validExecutableIdentityPath(path) {
		return ExecutableIdentity{}, errExecutableIdentityInvalidPath
	}

	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return ExecutableIdentity{}, errExecutableIdentityUnavailable
	}
	if !validExecutableIdentityPath(canonical) {
		return ExecutableIdentity{}, errExecutableIdentityUnsafe
	}
	if hooks != nil && hooks.afterResolve != nil {
		hooks.afterResolve()
	}

	before, err := os.Lstat(canonical)
	if err != nil {
		return ExecutableIdentity{}, errExecutableIdentityUnavailable
	}
	if category := executableInfoCategory(before); category != nil {
		return ExecutableIdentity{}, category
	}
	if hooks != nil && hooks.beforeOpen != nil {
		hooks.beforeOpen()
	}

	fd, err := syscall.Open(canonical, syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return ExecutableIdentity{}, errExecutableIdentityChanged
	}
	file := os.NewFile(uintptr(fd), "executable-identity")
	if file == nil {
		_ = syscall.Close(fd)
		return ExecutableIdentity{}, errExecutableIdentityChanged
	}
	defer func() { _ = file.Close() }()

	opened, err := file.Stat()
	if err != nil {
		return ExecutableIdentity{}, errExecutableIdentityChanged
	}
	if category := executableInfoCategory(opened); category != nil {
		return ExecutableIdentity{}, errExecutableIdentityChanged
	}
	beforeDevice, beforeInode, ok := executableFileIdentity(before)
	if !ok {
		return ExecutableIdentity{}, errExecutableIdentityUnsafe
	}
	openedDevice, openedInode, ok := executableFileIdentity(opened)
	if !ok || !os.SameFile(before, opened) || !sameExecutableMetadata(before, beforeDevice, beforeInode, opened, openedDevice, openedInode) {
		return ExecutableIdentity{}, errExecutableIdentityChanged
	}
	if hooks != nil && hooks.afterOpen != nil {
		hooks.afterOpen()
	}

	contentHash := sha256.New()
	limited := io.LimitReader(file, MaxExecutableIdentityBytes+1)
	buffer := make([]byte, 32*1024)
	var total int64
	firedFirstRead := false
	for {
		count, readErr := limited.Read(buffer)
		if count > 0 {
			if !firedFirstRead {
				firedFirstRead = true
				if hooks != nil && hooks.afterFirstRead != nil {
					hooks.afterFirstRead()
				}
			}
			total += int64(count)
			if total > MaxExecutableIdentityBytes {
				return ExecutableIdentity{}, errExecutableIdentityTooLarge
			}
			_, _ = contentHash.Write(buffer[:count])
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return ExecutableIdentity{}, errExecutableIdentityUnavailable
		}
	}
	if total == 0 {
		return ExecutableIdentity{}, errExecutableIdentityUnsafe
	}
	if hooks != nil && hooks.afterHash != nil {
		hooks.afterHash()
	}

	after, err := file.Stat()
	if err != nil {
		return ExecutableIdentity{}, errExecutableIdentityChanged
	}
	afterDevice, afterInode, ok := executableFileIdentity(after)
	if !ok || executableInfoCategory(after) != nil || total != opened.Size() ||
		!sameExecutableMetadata(opened, openedDevice, openedInode, after, afterDevice, afterInode) {
		return ExecutableIdentity{}, errExecutableIdentityChanged
	}
	pathInfo, err := os.Lstat(canonical)
	if err != nil {
		return ExecutableIdentity{}, errExecutableIdentityChanged
	}
	pathDevice, pathInode, ok := executableFileIdentity(pathInfo)
	if !ok || executableInfoCategory(pathInfo) != nil || !os.SameFile(after, pathInfo) ||
		!sameExecutableMetadata(after, afterDevice, afterInode, pathInfo, pathDevice, pathInode) {
		return ExecutableIdentity{}, errExecutableIdentityChanged
	}
	resolvedAgain, err := filepath.EvalSymlinks(path)
	if err != nil || resolvedAgain != canonical {
		return ExecutableIdentity{}, errExecutableIdentityChanged
	}

	contentDigest := hex.EncodeToString(contentHash.Sum(nil))
	identity := ExecutableIdentity{
		schemaVersion:  CurrentExecutableIdentitySchemaVersion,
		invocationPath: path,
		canonicalPath:  canonical,
		size:           after.Size(),
		mode:           uint32(after.Mode()),
		device:         afterDevice,
		inode:          afterInode,
		contentDigest:  contentDigest,
	}
	identity.digest, ok = executableIdentityDigest(identity)
	if !ok {
		return ExecutableIdentity{}, errExecutableIdentityInvalid
	}
	return identity, nil
}

func validExecutableIdentityPath(path string) bool {
	if path == "" || len(path) > MaxExecutableIdentityPathBytes || !utf8.ValidString(path) || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return false
	}
	for _, value := range path {
		if unicode.IsControl(value) {
			return false
		}
	}
	return true
}

func executableInfoCategory(info os.FileInfo) error {
	if info == nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Mode().Perm()&0o111 == 0 ||
		info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return errExecutableIdentityUnsafe
	}
	if info.Size() > MaxExecutableIdentityBytes {
		return errExecutableIdentityTooLarge
	}
	return nil
}

func executableFileIdentity(info os.FileInfo) (uint64, uint64, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, false
	}
	// #nosec G115 -- kernel device IDs are non-negative and require their full width on Linux.
	device := uint64(stat.Dev)
	inode := stat.Ino
	return device, inode, true
}

func sameExecutableMetadata(first os.FileInfo, firstDevice, firstInode uint64, second os.FileInfo, secondDevice, secondInode uint64) bool {
	return first.Size() == second.Size() && first.Mode() == second.Mode() && firstDevice == secondDevice && firstInode == secondInode
}

func validLowerHexDigest(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && hex.EncodeToString(decoded) == value
}

func validExecutableIdentity(identity ExecutableIdentity) bool {
	if identity.schemaVersion != CurrentExecutableIdentitySchemaVersion || !validExecutableIdentityPath(identity.invocationPath) ||
		!validExecutableIdentityPath(identity.canonicalPath) || identity.size <= 0 || identity.size > MaxExecutableIdentityBytes ||
		!validLowerHexDigest(identity.contentDigest) || !validLowerHexDigest(identity.digest) {
		return false
	}
	mode := os.FileMode(identity.mode)
	if !mode.IsRegular() || mode.Perm()&0o111 == 0 || mode&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return false
	}
	digest, ok := executableIdentityDigest(identity)
	return ok && digest == identity.digest
}

func executableIdentityDigest(identity ExecutableIdentity) (string, bool) {
	contentDigest, err := hex.DecodeString(identity.contentDigest)
	if err != nil || len(contentDigest) != sha256.Size || hex.EncodeToString(contentDigest) != identity.contentDigest {
		return "", false
	}
	fields := [][]byte{
		[]byte(strconv.Itoa(identity.schemaVersion)),
		[]byte(identity.invocationPath),
		[]byte(identity.canonicalPath),
		[]byte(strconv.FormatInt(identity.size, 10)),
		[]byte(strconv.FormatUint(uint64(identity.mode), 10)),
		[]byte(strconv.FormatUint(identity.device, 10)),
		[]byte(strconv.FormatUint(identity.inode, 10)),
		contentDigest,
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte("dotfiles.pkg.executable-identity.v1\x00"))
	var length [8]byte
	for _, field := range fields {
		binary.BigEndian.PutUint64(length[:], uint64(len(field)))
		_, _ = hash.Write(length[:])
		_, _ = hash.Write(field)
	}
	return hex.EncodeToString(hash.Sum(nil)), true
}
