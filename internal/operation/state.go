package operation

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tekierz/dotfiles/internal/safefile"
)

const stagingAttempts = 32

// StateStagingAuthority is the opaque namespace authority captured when a
// private staging leaf is created. It binds the state anchor, every staging
// parent identity, and the created leaf identity.
type StateStagingAuthority struct {
	root    string
	rel     string
	path    string
	parents *safefile.ParentChain
	leaf    *safefile.DirectorySnapshot
}

// StateSubdirectory returns an absolute path for one direct child of dotfiles'
// private operational-state namespace. It does not create the child.
func StateSubdirectory(name string) (string, error) {
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsAny(name, "/\\\x00\r\n\t ") {
		return "", fmt.Errorf("operation state subdirectory must be one non-empty path component")
	}
	root, stateRel, err := stateAnchor()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, filepath.FromSlash(stateRel), name), nil
}

// CreateStateStagingDirectoryTracked creates a private randomized directory
// and returns the opaque namespace authority required by every later staging
// operation.
func CreateStateStagingDirectoryTracked(scope string) (string, *StateStagingAuthority, error) {
	if strings.TrimSpace(scope) == "" {
		return "", nil, fmt.Errorf("operation staging scope is required")
	}
	root, stateRel, err := stateAnchor()
	if err != nil {
		return "", nil, err
	}
	stagingRel := filepath.ToSlash(filepath.Join(stateRel, "staging"))
	if err := safefile.EnsureDirectoryWithin(root, stagingRel, 0o700); err != nil {
		return "", nil, fmt.Errorf("create private operation staging namespace: %w", err)
	}
	scopeDigest := sha256.Sum256([]byte(scope))
	prefix := ".stage-" + hex.EncodeToString(scopeDigest[:6]) + "-"
	for range stagingAttempts {
		var random [12]byte
		if _, err := rand.Read(random[:]); err != nil {
			return "", nil, fmt.Errorf("generate operation staging name: %w", err)
		}
		name := prefix + hex.EncodeToString(random[:])
		rel := filepath.ToSlash(filepath.Join(stagingRel, name))
		parents, err := safefile.CaptureParentChainWithin(root, rel)
		if err != nil {
			return "", nil, fmt.Errorf("capture private staging parent authority: %w", err)
		}
		created, err := safefile.EnsureShallowDirectoryWithinParentChainTracked(root, rel, nil, parents, 0o700)
		if err == nil {
			path := filepath.Join(root, filepath.FromSlash(rel))
			return path, &StateStagingAuthority{root: root, rel: rel, path: path, parents: parents, leaf: created}, nil
		}
		var committed *safefile.CommittedError
		if errors.Is(err, safefile.ErrDirectoryChanged) && !errors.As(err, &committed) {
			continue
		}
		return "", nil, fmt.Errorf("create private operation staging directory: %w", err)
	}
	return "", nil, fmt.Errorf("create private operation staging directory: exhausted randomized names")
}

// StateStagingLocation returns the anchored location bound into authority.
// Callers use it for descriptor-relative staged edits instead of promoting the
// randomized leaf path to a new trust root.
func StateStagingLocation(path string, authority *StateStagingAuthority) (string, string, error) {
	if authority == nil || filepath.Clean(path) != filepath.Clean(authority.path) || authority.root == "" || authority.rel == "" || !authority.parents.Tracked() || authority.leaf == nil {
		return "", "", fmt.Errorf("operation staging authority is invalid")
	}
	wantPath := filepath.Join(authority.root, filepath.FromSlash(authority.rel))
	if filepath.Clean(path) != filepath.Clean(wantPath) || !strings.HasPrefix(filepath.Base(path), ".stage-") {
		return "", "", fmt.Errorf("operation staging authority path does not match")
	}
	return authority.root, authority.rel, nil
}

// StateStagingDescendantAuthority binds one existing descendant target to the
// exact staging leaf and every current non-recursive parent identity. It lets
// callers use safefile's authorized read/write/removal APIs without trusting
// the randomized staging pathname as a new root.
func StateStagingDescendantAuthority(path string, authority *StateStagingAuthority, descendant string) (string, string, *safefile.ParentChain, error) {
	root, stagingRel, err := StateStagingLocation(path, authority)
	if err != nil {
		return "", "", nil, err
	}
	descendant = filepath.ToSlash(filepath.Clean(filepath.FromSlash(descendant)))
	if descendant == "." || descendant == "" || filepath.IsAbs(descendant) || descendant == ".." || strings.HasPrefix(descendant, "../") {
		return "", "", nil, fmt.Errorf("invalid staging descendant %q", descendant)
	}
	targetRel := filepath.ToSlash(filepath.Join(stagingRel, filepath.FromSlash(descendant)))
	parents, err := safefile.ExtendParentChainWithinDirectory(root, targetRel, stagingRel, authority.parents, authority.leaf)
	if err != nil {
		return "", "", nil, fmt.Errorf("bind staging descendant %s: %w", descendant, err)
	}
	return root, targetRel, parents, nil
}

// SnapshotStateStagingDirectory captures the final staging tree only if its
// root still has the identity returned at creation. A replaced staging path is
// preserved and rejected instead of being installed or cleaned recursively.
func SnapshotStateStagingDirectory(path string, authority *StateStagingAuthority) (*safefile.DirectorySnapshot, error) {
	root, rel, err := StateStagingLocation(path, authority)
	if err != nil {
		return nil, err
	}
	if _, err := safefile.BindParentChainWithin(root, rel, authority.parents, nil); err != nil {
		return nil, fmt.Errorf("verify private staging parent authority: %w", err)
	}
	current, err := safefile.SnapshotDirectoryWithin(root, rel)
	if err != nil {
		return nil, fmt.Errorf("snapshot private operation staging directory: %w", err)
	}
	if !safefile.SameDirectoryRootState(authority.leaf, current) {
		return nil, fmt.Errorf("%w: private operation staging directory was replaced", safefile.ErrDirectoryChanged)
	}
	if _, err := safefile.BindParentChainWithin(root, rel, authority.parents, nil); err != nil {
		return nil, fmt.Errorf("reverify private staging parent authority: %w", err)
	}
	return current, nil
}

// RemoveStateStagingDirectoryAuthorized performs cleanup only inside the exact
// state namespace captured with the staging leaf.
func RemoveStateStagingDirectoryAuthorized(path string, authority *StateStagingAuthority, expected *safefile.DirectorySnapshot) error {
	if expected == nil {
		return fmt.Errorf("operation staging cleanup requires an exact snapshot")
	}
	root, rel, err := StateStagingLocation(path, authority)
	if err != nil {
		return err
	}
	if !safefile.SameDirectoryRootState(authority.leaf, expected) {
		return fmt.Errorf("%w: cleanup snapshot does not describe the created staging directory", safefile.ErrDirectoryChanged)
	}
	if err := safefile.RemoveDirectoryWithinSnapshotAuthorized(root, rel, expected, authority.parents); err != nil {
		return fmt.Errorf("remove private operation staging directory: %w", err)
	}
	return nil
}
