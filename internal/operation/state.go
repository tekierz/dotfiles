package operation

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tekierz/dotfiles/internal/safefile"
)

const stagingAttempts = 32

var stateNamespaceChildren = []string{"operations", "locks", "backups", "staging"}

type StatePlan struct {
	root     string
	stateRel string
	entries  []statePlanEntry
}

type statePlanEntry struct {
	rel     string
	exists  bool
	parents *safefile.ParentChain
	leaf    *safefile.DirectorySnapshot
}

type StateAuthority struct {
	root     string
	stateRel string
	children map[string]stateDirectoryAuthority
	created  map[string]*safefile.DirectorySnapshot
}

type stateDirectoryAuthority struct {
	rel     string
	parents *safefile.ParentChain
	leaf    *safefile.DirectorySnapshot
}

// CaptureStatePlan records the exact existing/missing operational namespace
// without creating any filesystem entry. It is safe to call while rendering a
// preview.
func CaptureStatePlan() (*StatePlan, error) {
	root, stateRel, err := stateAnchor()
	if err != nil {
		return nil, err
	}
	paths := statePathPrefixes(stateRel)
	for _, child := range stateNamespaceChildren {
		paths = append(paths, filepath.ToSlash(filepath.Join(stateRel, child)))
	}
	plan := &StatePlan{root: root, stateRel: stateRel, entries: make([]statePlanEntry, 0, len(paths))}
	for _, rel := range paths {
		info, statErr := os.Lstat(filepath.Join(root, filepath.FromSlash(rel)))
		if errors.Is(statErr, os.ErrNotExist) {
			parents, captureErr := safefile.CaptureParentChainWithin(root, rel)
			if captureErr != nil {
				return nil, fmt.Errorf("capture operation state parent %s: %w", rel, captureErr)
			}
			plan.entries = append(plan.entries, statePlanEntry{rel: rel, parents: parents})
			continue
		} else if statErr == nil {
			leaf, parents, captureErr := safefile.CaptureDirectoryRootWithin(root, rel)
			if captureErr != nil {
				return nil, fmt.Errorf("validate operation state directory %s: %w", rel, captureErr)
			}
			plan.entries = append(plan.entries, statePlanEntry{rel: rel, exists: true, parents: parents, leaf: leaf})
		}
		requirePrivate := rel == stateRel || strings.HasPrefix(rel, stateRel+"/")
		badMode := info != nil && (info.Mode().Perm()&0o022 != 0 || (requirePrivate && info.Mode().Perm() != 0o700))
		if statErr != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || badMode {
			if statErr == nil {
				statErr = fmt.Errorf("mode is %04o", info.Mode().Perm())
			}
			return nil, fmt.Errorf("operation state directory %s is not a private real directory: %w", rel, statErr)
		}
	}
	return plan, nil
}

// BootstrapStateNamespaceTracked creates only plan-accepted missing
// components, binds each created inode into deeper chains, and rejects any
// existing component replaced after preview.
func BootstrapStateNamespaceTracked(plan *StatePlan) (*StateAuthority, error) {
	if plan == nil || plan.root == "" || plan.stateRel == "" {
		return nil, fmt.Errorf("operation state plan is unavailable")
	}
	created := make(map[string]*safefile.DirectorySnapshot)
	for _, entry := range plan.entries {
		if entry.exists {
			leaf, parents, err := safefile.CaptureDirectoryRootWithin(plan.root, entry.rel)
			if err != nil || !safefile.SameParentChain(parents, entry.parents) || !safefile.SameDirectoryRootState(leaf, entry.leaf) {
				return nil, fmt.Errorf("%w: operation state directory %s changed after preview", safefile.ErrParentChanged, entry.rel)
			}
			continue
		}
		parents, err := safefile.BindParentChainWithin(plan.root, entry.rel, entry.parents, created)
		if err != nil {
			return nil, fmt.Errorf("bind operation state parent %s: %w", entry.rel, err)
		}
		leaf, err := safefile.EnsureShallowDirectoryWithinParentChainTracked(plan.root, entry.rel, nil, parents, 0o700)
		if err != nil {
			return nil, fmt.Errorf("create accepted operation state directory %s: %w", entry.rel, err)
		}
		created[entry.rel] = leaf
	}
	authority := &StateAuthority{root: plan.root, stateRel: plan.stateRel, children: make(map[string]stateDirectoryAuthority), created: created}
	for _, child := range stateNamespaceChildren {
		rel := filepath.ToSlash(filepath.Join(plan.stateRel, child))
		var accepted statePlanEntry
		for _, entry := range plan.entries {
			if entry.rel == rel {
				accepted = entry
				break
			}
		}
		leaf := accepted.leaf
		if !accepted.exists {
			leaf = created[rel]
		}
		parents, err := safefile.BindParentChainWithin(plan.root, rel, accepted.parents, created)
		if err != nil {
			return nil, fmt.Errorf("bind operation state child %s: %w", child, err)
		}
		authority.children[child] = stateDirectoryAuthority{rel: rel, parents: parents, leaf: leaf}
	}
	for child, accepted := range authority.children {
		leaf, parents, err := safefile.CaptureDirectoryRootWithin(plan.root, accepted.rel)
		if err != nil || !safefile.SameParentChain(parents, accepted.parents) || !safefile.SameDirectoryRootState(leaf, accepted.leaf) {
			return nil, fmt.Errorf("%w: operation state child %s changed during bootstrap", safefile.ErrParentChanged, child)
		}
	}
	return authority, nil
}

// CreatedDirectoriesWithin returns exact shallow-creation evidence relative to
// root when the accepted state anchor is that same trusted root. Operational
// directories remain non-rollback state; callers use this map only to bind
// plan-time missing parent chains after reviewed bootstrap.
func (a *StateAuthority) CreatedDirectoriesWithin(root string) map[string]*safefile.DirectorySnapshot {
	if a == nil {
		return nil
	}
	if filepath.Clean(a.root) != filepath.Clean(root) {
		left, leftErr := os.Stat(a.root)
		right, rightErr := os.Stat(root)
		if leftErr != nil || rightErr != nil || !os.SameFile(left, right) {
			return nil
		}
	}
	created := make(map[string]*safefile.DirectorySnapshot, len(a.created))
	for rel, snapshot := range a.created {
		created[rel] = snapshot
	}
	return created
}

func BootstrapStateNamespace() error {
	plan, err := CaptureStatePlan()
	if err != nil {
		return err
	}
	_, err = BootstrapStateNamespaceTracked(plan)
	return err
}

func statePathPrefixes(rel string) []string {
	components := strings.Split(filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel))), "/")
	paths := make([]string, 0, len(components))
	for index := range components {
		paths = append(paths, strings.Join(components[:index+1], "/"))
	}
	return paths
}

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

func stateChildDescendantAuthority(authority *StateAuthority, child, targetRel string) (string, *safefile.ParentChain, error) {
	if authority == nil || authority.root == "" {
		return "", nil, fmt.Errorf("operation state authority is unavailable")
	}
	base, ok := authority.children[child]
	if !ok || base.leaf == nil || !base.parents.Tracked() {
		return "", nil, fmt.Errorf("operation state child authority %s is unavailable", child)
	}
	targetRel = filepath.ToSlash(filepath.Clean(filepath.FromSlash(targetRel)))
	parents, err := safefile.ExtendParentChainWithinDirectory(authority.root, targetRel, base.rel, base.parents, base.leaf)
	if err != nil {
		return "", nil, err
	}
	return authority.root, parents, nil
}

// StateChildTargetAuthority returns exact parent authority for one direct,
// currently absent child below a bound operational namespace directory.
func StateChildTargetAuthority(authority *StateAuthority, child, name string) (root, rel, path string, parents *safefile.ParentChain, err error) {
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsAny(name, "/\\\x00\r\n\t ") {
		return "", "", "", nil, fmt.Errorf("operation state target name must be one component")
	}
	if authority == nil {
		return "", "", "", nil, fmt.Errorf("operation state authority is unavailable")
	}
	base, ok := authority.children[child]
	if !ok {
		return "", "", "", nil, fmt.Errorf("operation state child %s is unavailable", child)
	}
	rel = filepath.ToSlash(filepath.Join(base.rel, name))
	root, parents, err = stateChildDescendantAuthority(authority, child, rel)
	if err != nil {
		return "", "", "", nil, err
	}
	return root, rel, filepath.Join(root, filepath.FromSlash(rel)), parents, nil
}

// CreateStateStagingDirectoryTracked creates a private randomized directory
// and returns the opaque namespace authority required by every later staging
// operation.
func CreateStateStagingDirectoryTracked(scope string) (string, *StateStagingAuthority, error) {
	plan, err := CaptureStatePlan()
	if err != nil {
		return "", nil, err
	}
	authority, err := BootstrapStateNamespaceTracked(plan)
	if err != nil {
		return "", nil, err
	}
	return CreateStateStagingDirectoryWithAuthorityTracked(authority, scope)
}

func CreateStateStagingDirectoryWithAuthorityTracked(state *StateAuthority, scope string) (string, *StateStagingAuthority, error) {
	if strings.TrimSpace(scope) == "" {
		return "", nil, fmt.Errorf("operation staging scope is required")
	}
	if state == nil {
		return "", nil, fmt.Errorf("operation state authority is unavailable")
	}
	root := state.root
	stagingRel := filepath.ToSlash(filepath.Join(state.stateRel, "staging"))
	scopeDigest := sha256.Sum256([]byte(scope))
	prefix := ".stage-" + hex.EncodeToString(scopeDigest[:6]) + "-"
	for range stagingAttempts {
		var random [12]byte
		if _, err := rand.Read(random[:]); err != nil {
			return "", nil, fmt.Errorf("generate operation staging name: %w", err)
		}
		name := prefix + hex.EncodeToString(random[:])
		rel := filepath.ToSlash(filepath.Join(stagingRel, name))
		_, parents, err := stateChildDescendantAuthority(state, "staging", rel)
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

func OpenStateStagingDirectory(path string, authority *StateStagingAuthority) (*os.File, error) {
	root, rel, err := StateStagingLocation(path, authority)
	if err != nil {
		return nil, err
	}
	file, err := safefile.OpenDirectoryWithinAuthorized(root, rel, authority.parents, authority.leaf)
	if err != nil {
		return nil, fmt.Errorf("open private staging directory: %w", err)
	}
	return file, nil
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
