package tools

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/safefile"
)

// Category represents a tool category
type Category string

// PartialMutationError reports targets whose product mutation definitely
// committed before a later step failed. Callers may conditionally roll back
// only these paths; every omitted target remains ambiguous and must fail closed.
type PartialMutationError struct {
	Err      error
	Evidence []MutationEvidence
}

type MutationEvidence struct {
	Path      string
	Revision  safefile.Revision
	Directory *safefile.DirectorySnapshot
	Parents   *safefile.ParentChain
}

func (e *PartialMutationError) Error() string { return e.Err.Error() }
func (e *PartialMutationError) Unwrap() error { return e.Err }
func (e *PartialMutationError) CommittedEvidence() []MutationEvidence {
	return append([]MutationEvidence(nil), e.Evidence...)
}

func partialMutationError(err error, evidence []MutationEvidence) error {
	if err == nil || len(evidence) == 0 {
		return err
	}
	return &PartialMutationError{Err: err, Evidence: append([]MutationEvidence(nil), evidence...)}
}

const (
	CategoryShell     Category = "shell"
	CategoryTerminal  Category = "terminal"
	CategoryEditor    Category = "editor"
	CategoryFile      Category = "file"
	CategoryGit       Category = "git"
	CategoryContainer Category = "container"
	CategoryUtility   Category = "utility"
	CategoryApp       Category = "app"
)

// UIGroup represents which installer section a tool belongs to
type UIGroup string

const (
	UIGroupNone         UIGroup = "" // Has dedicated config screen
	UIGroupCLITools     UIGroup = "cli-tools"
	UIGroupCLIUtilities UIGroup = "cli-utilities"
	UIGroupGUIApps      UIGroup = "gui-apps"
	UIGroupMacApps      UIGroup = "macos-apps"
	UIGroupUtilities    UIGroup = "utilities" // Shell scripts (hk, caff, sshh)
)

// Tool defines the interface for all managed tools
type Tool interface {
	// Identity
	ID() string          // Unique identifier (e.g., "ghostty", "lazygit")
	Name() string        // Display name (e.g., "Ghostty", "LazyGit")
	Description() string // Short description
	Icon() string        // Nerd font icon
	Category() Category  // Tool category

	// Package management
	Packages() map[pkg.Platform][]string // Platform-specific package names
	IsInstalled() bool                   // Check if tool is installed
	Install(mgr pkg.PackageManager) error

	// Configuration
	ConfigPaths() []string // Config file paths (e.g., ~/.config/ghostty/config)
	HasConfig() bool       // Whether this tool has configurable options

	// Resource requirements
	IsHeavy() bool // Whether this tool requires significant resources (skip on low-memory systems)

	// UI metadata for installer screens
	UIGroup() UIGroup             // Which installer group (empty = dedicated screen)
	ConfigScreen() int            // Which config screen constant (0 = part of group screen)
	DefaultEnabled() bool         // Default state in installer
	PlatformFilter() pkg.Platform // Empty for all platforms, or specific platform
}

// BaseTool provides common functionality for tools
type BaseTool struct {
	id          string
	name        string
	description string
	icon        string
	category    Category
	packages    map[pkg.Platform][]string
	configPaths []string
	heavyTool   bool // If true, tool is skipped on low-memory systems (e.g., Pi Zero 2)

	// UI metadata
	uiGroup        UIGroup
	configScreen   int // Screen constant as int to avoid import cycle
	defaultEnabled bool
	platformFilter pkg.Platform
}

func (t *BaseTool) ID() string          { return t.id }
func (t *BaseTool) Name() string        { return t.name }
func (t *BaseTool) Description() string { return t.description }
func (t *BaseTool) Icon() string        { return t.icon }
func (t *BaseTool) Category() Category  { return t.category }

func (t *BaseTool) Packages() map[pkg.Platform][]string {
	return t.packages
}

func (t *BaseTool) ConfigPaths() []string {
	return t.configPaths
}

func (t *BaseTool) HasConfig() bool {
	return len(t.configPaths) > 0
}

func (t *BaseTool) IsHeavy() bool {
	return t.heavyTool
}

func (t *BaseTool) UIGroup() UIGroup             { return t.uiGroup }
func (t *BaseTool) ConfigScreen() int            { return t.configScreen }
func (t *BaseTool) DefaultEnabled() bool         { return t.defaultEnabled }
func (t *BaseTool) PlatformFilter() pkg.Platform { return t.platformFilter }

// PackagesForPlatform resolves the package names for a given platform from a
// tool's package map. Resolution order is: exact platform, then (for a
// Raspberry Pi, which uses Debian packages) the Debian entry, then the "all"
// fallback key. This is the single source of truth for package resolution and
// must be used everywhere packages are looked up so that documented platforms
// like the Raspberry Pi are never silently skipped.
func PackagesForPlatform(packages map[pkg.Platform][]string, platform pkg.Platform) []string {
	if pkgs := packages[platform]; len(pkgs) > 0 {
		return pkgs
	}
	// Raspberry Pi reuses Debian packages (same apt manager).
	if platform == pkg.PlatformPi {
		if pkgs := packages[pkg.PlatformDebian]; len(pkgs) > 0 {
			return pkgs
		}
	}
	return packages["all"]
}

// allPackagesInstalled reports whether EVERY package in pkgs is installed
// according to mgr. A tool is only "installed" when all of its platform
// packages are present; a tool with multiple packages (e.g. zsh ->
// {zsh, zsh-autosuggestions, ...}) that is only partially installed must
// report false so the install flow does not skip it (C6). An empty pkgs list
// is never considered installed.
func allPackagesInstalled(mgr pkg.PackageManager, pkgs []string) bool {
	if len(pkgs) == 0 {
		return false
	}
	for _, p := range pkgs {
		if !mgr.IsInstalled(p) {
			return false
		}
	}
	return true
}

func (t *BaseTool) IsInstalled() bool {
	mgr := pkg.DetectManager()
	return t.IsInstalledForPlatform(mgr, pkg.DetectPlatform())
}

func (t *BaseTool) Install(mgr pkg.PackageManager) error {
	return t.InstallForPlatform(mgr, pkg.DetectPlatform())
}

// IsInstalledForPlatform is the environment-explicit form used by planning and
// tests that model a platform other than the host running the process. Custom
// tools whose package metadata is only a prerequisite still own IsInstalled;
// this helper is authoritative only for ordinary BaseTool package installs.
func (t *BaseTool) IsInstalledForPlatform(mgr pkg.PackageManager, platform pkg.Platform) bool {
	if mgr == nil {
		return false
	}
	pkgs := PackagesForPlatform(t.packages, platform)
	// Require ALL platform packages, not just the primary one. A partially
	// installed multi-package tool must report false (C6).
	return allPackagesInstalled(mgr, pkgs)
}

// InstallForPlatform resolves packages from the caller's platform snapshot.
// Install remains the public Tool contract for compatibility, while dashboard
// execution uses this explicit variant for BaseTool-backed package installs so
// the planned and executed platform cannot drift.
func (t *BaseTool) InstallForPlatform(mgr pkg.PackageManager, platform pkg.Platform) error {
	pkgs := PackagesForPlatform(t.packages, platform)
	if len(pkgs) == 0 {
		return nil // No packages to install for this platform
	}
	if mgr == nil {
		return fmt.Errorf("no package manager available for %s", platform)
	}
	return mgr.Install(pkgs...)
}

var errGeneratedConfigOutsideTrustedRoots = errors.New("generated config path is outside HOME and XDG_CONFIG_HOME")

// ErrUnmanagedConfig is returned when a whole-file generator would replace an
// existing native config that dotfiles cannot prove it owns. Per-tool adapters
// that can merge a managed fragment (for example Zsh, Git, and Ghostty) use
// their locked read/modify/write paths instead. Legacy whole-file generators
// may update only files carrying one of the exact headers emitted by this
// project.
var ErrUnmanagedConfig = errors.New("refusing to replace existing unmanaged config")

var generatedConfigHeaders = [][]byte{
	[]byte("# Generated by dotfiles TUI\n"),
	[]byte("#? Generated by dotfiles TUI\n"),
	[]byte("-- Generated by dotfiles TUI\n"),
}

// ConfigOwnershipObservation is a side-effect-free ownership fact for a
// generated whole-file target. Digest identifies the observed bytes without
// exposing their contents to plans, diagnostics, or operation journals.
type ConfigOwnershipObservation struct {
	Exists  bool
	Managed bool
	Digest  string
}

// ObserveGeneratedConfig reads a prospective whole-file target through the
// descriptor-anchored no-follow path and reports whether it carries an exact
// product ownership header. It never creates a missing XDG root.
func ObserveGeneratedConfig(path string) (ConfigOwnershipObservation, error) {
	root, rel, _, err := generatedConfigDestination(path)
	if err != nil {
		return ConfigOwnershipObservation{}, err
	}
	if _, err := os.Stat(root); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ConfigOwnershipObservation{}, nil
		}
		return ConfigOwnershipObservation{}, fmt.Errorf("inspect generated config root %s: %w", root, err)
	}
	content, revision, err := safefile.ReadWithin(root, rel)
	if err != nil {
		return ConfigOwnershipObservation{}, fmt.Errorf("observe generated config %s: %w", path, err)
	}
	if !revision.Exists() {
		return ConfigOwnershipObservation{}, nil
	}
	digest := sha256.Sum256(content)
	return ConfigOwnershipObservation{
		Exists:  true,
		Managed: hasGeneratedConfigHeader(content),
		Digest:  hex.EncodeToString(digest[:]),
	}, nil
}

type replaceGeneratedConfigFunc func(root, rel string, data []byte, mode os.FileMode) error

// writeToolConfig atomically replaces a generated tool config below a trusted
// per-user root. HOME (including its resolved location when HOME itself is a
// symlink) is preferred so symlinked descendants such as ~/.config are refused.
// An absolute XDG_CONFIG_HOME outside both HOME identities is also an explicit
// trusted root, which supports applications that honor an external XDG tree.
// Missing descendant directories are created 0700 by safefile; existing
// directory modes are preserved. Generated files are always written 0600.
func writeToolConfig(path string, content []byte) error {
	_, err := writeToolConfigTracked(path, content)
	return err
}

func writeToolConfigTracked(path string, content []byte) (MutationEvidence, error) {
	if !hasGeneratedConfigHeader(content) {
		return MutationEvidence{}, fmt.Errorf("%w: replacement for %s has no recognized ownership header", ErrUnmanagedConfig, path)
	}

	var evidence MutationEvidence
	err := withToolConfigLock(path, func(root, rel string) error {
		existing, revision, err := readToolConfig(root, rel)
		if err != nil {
			return err
		}
		if revision.Exists() && !hasGeneratedConfigHeader(existing) {
			return fmt.Errorf("%w: %s", ErrUnmanagedConfig, path)
		}
		committed, err := replaceToolConfigAtRevisionTracked(root, rel, revision, content)
		if err == nil {
			evidence = MutationEvidence{Path: path, Revision: committed}
		}
		return err
	})
	return evidence, err
}

// writeToolConfigAtRevisionTracked is the install-plan variant of the whole-
// file writer. The plan's private revision is authoritative: ownership and
// same-content checks happen only after the locked live target has been proven
// to be the exact namespace object accepted by the user.
func writeToolConfigAtRevisionTracked(path string, content []byte, accepted safefile.Revision) (MutationEvidence, error) {
	parents, err := compatibilityToolConfigParents(path)
	if err != nil {
		return MutationEvidence{}, err
	}
	return writeToolConfigAtAuthorityTracked(path, content, accepted, parents, operation.DefaultLocker)
}

func compatibilityToolConfigParents(path string) (*safefile.ParentChain, error) {
	root, rel, createRoot, err := generatedConfigDestination(path)
	if err != nil || createRoot {
		return nil, fmt.Errorf("resolve compatibility config authority for %s: %w", path, err)
	}
	parents, err := safefile.CaptureParentChainWithin(root, rel)
	if err != nil {
		return nil, err
	}
	return parents, nil
}

// writeToolConfigAtAuthorityTracked is the production install-plan writer. It
// binds both the accepted leaf revision and the complete root-to-parent
// namespace chain at the commit boundary.
func writeToolConfigAtAuthorityTracked(path string, content []byte, accepted safefile.Revision, parents *safefile.ParentChain, locker operation.Locker) (MutationEvidence, error) {
	if !accepted.Tracked() {
		return MutationEvidence{}, fmt.Errorf("%w: accepted revision for %s is untracked", safefile.ErrRevisionChanged, path)
	}
	if parents != nil && !parents.Tracked() {
		return MutationEvidence{}, fmt.Errorf("%w: accepted parent chain for %s is untracked", safefile.ErrParentChanged, path)
	}
	if !parents.Tracked() || locker == nil {
		return MutationEvidence{}, fmt.Errorf("%w: accepted writer authority for %s is incomplete", safefile.ErrParentChanged, path)
	}
	if !hasGeneratedConfigHeader(content) {
		return MutationEvidence{}, fmt.Errorf("%w: replacement for %s has no recognized ownership header", ErrUnmanagedConfig, path)
	}

	var evidence MutationEvidence
	err := withToolConfigLockAuthorized(path, locker, func(root, rel string) error {
		var existing []byte
		var current safefile.Revision
		var err error
		if parents != nil {
			existing, current, err = safefile.ReadWithinAuthorized(root, rel, parents)
		} else {
			existing, current, err = readToolConfig(root, rel)
		}
		if err != nil {
			return err
		}
		if current != accepted {
			return fmt.Errorf("%w: generated config %s changed after plan acceptance", safefile.ErrRevisionChanged, rel)
		}
		if current.Exists() && !hasGeneratedConfigHeader(existing) {
			return fmt.Errorf("%w: %s", ErrUnmanagedConfig, path)
		}
		if current.Exists() && bytes.Equal(existing, content) {
			evidence = MutationEvidence{Path: path, Revision: current, Parents: parents}
			return nil
		}
		var committed safefile.Revision
		if parents != nil {
			committed, err = replaceToolConfigAtRevisionNoCreateAuthorizedTracked(root, rel, accepted, parents, content)
		} else {
			committed, err = replaceToolConfigAtRevisionNoCreateTracked(root, rel, accepted, content)
		}
		if err == nil {
			evidence = MutationEvidence{Path: path, Revision: committed, Parents: parents}
		}
		return err
	})
	return evidence, err
}

func preflightToolConfigAtAuthority(path string, accepted safefile.Revision, parents *safefile.ParentChain, locker operation.Locker, allowLegacy func([]byte) bool) error {
	if !accepted.Tracked() {
		return fmt.Errorf("%w: accepted revision for %s is untracked", safefile.ErrRevisionChanged, path)
	}
	if !parents.Tracked() || locker == nil {
		return fmt.Errorf("%w: preflight authority for %s is incomplete", safefile.ErrParentChanged, path)
	}
	return withToolConfigLockAuthorized(path, locker, func(root, rel string) error {
		existing, current, err := safefile.ReadWithinAuthorized(root, rel, parents)
		if err != nil {
			return err
		}
		if current != accepted {
			return fmt.Errorf("%w: generated config %s changed after plan acceptance", safefile.ErrRevisionChanged, rel)
		}
		if current.Exists() && !hasGeneratedConfigHeader(existing) && (allowLegacy == nil || !allowLegacy(existing)) {
			return fmt.Errorf("%w: %s", ErrUnmanagedConfig, path)
		}
		return nil
	})
}

func hasGeneratedConfigHeader(content []byte) bool {
	for _, header := range generatedConfigHeaders {
		if bytes.HasPrefix(content, header) {
			return true
		}
	}
	return false
}

func writeGeneratedConfig(path string, content []byte, mode os.FileMode) error {
	return writeGeneratedConfigWith(path, content, mode, safefile.ReplaceWithin)
}

func writeGeneratedConfigWith(path string, content []byte, mode os.FileMode, replace replaceGeneratedConfigFunc) error {
	root, rel, err := prepareGeneratedConfigDestination(path)
	if err != nil {
		return err
	}
	if err := replace(root, rel, content, mode); err != nil {
		return fmt.Errorf("failed to replace generated config %s: %w", path, err)
	}
	return nil
}

// withToolConfigLock serializes a read-modify-write operation on path with
// other cooperating dotfiles processes. The lock lives below dotfiles' private
// operational-state root, so locking never creates an unplanned sidecar beside
// user configuration. Target parents are created only after acquiring it.
func withToolConfigLock(path string, mutate func(root, rel string) error) (returnErr error) {
	return withToolConfigLockPolicy(path, true, operation.DefaultLocker, mutate)
}

func withToolConfigLockNoCreate(path string, mutate func(root, rel string) error) (returnErr error) {
	return withToolConfigLockPolicy(path, false, operation.DefaultLocker, mutate)
}

func withToolConfigLockAuthorized(path string, locker operation.Locker, mutate func(root, rel string) error) (returnErr error) {
	if locker == nil {
		return fmt.Errorf("accepted tool config locker is unavailable")
	}
	return withToolConfigLockPolicy(path, false, locker, mutate)
}

func withToolConfigLockPolicy(path string, createParents bool, locker operation.Locker, mutate func(root, rel string) error) (returnErr error) {
	release, err := locker("tool-config", path)
	if err != nil {
		return fmt.Errorf("failed to lock generated config %s: %w", path, err)
	}
	defer func() {
		if releaseErr := release(); releaseErr != nil {
			returnErr = errors.Join(returnErr, fmt.Errorf("failed to unlock generated config %s: %w", path, releaseErr))
		}
	}()

	var root, rel string
	if createParents {
		root, rel, err = prepareGeneratedConfigDestination(path)
		if err == nil {
			parent := filepath.ToSlash(filepath.Dir(rel))
			if parent != "." {
				err = safefile.EnsureDirectoryWithin(root, parent, 0700)
			}
		}
	} else {
		var createRoot bool
		root, rel, createRoot, err = generatedConfigDestination(path)
		if err == nil && createRoot {
			err = fmt.Errorf("accepted config root does not exist: %s", root)
		}
	}
	if err != nil {
		return fmt.Errorf("prepare generated config parent for %s: %w", path, err)
	}
	return mutate(root, rel)
}

func readToolConfig(root, rel string) ([]byte, safefile.Revision, error) {
	content, revision, err := safefile.ReadWithin(root, rel)
	if err != nil {
		return nil, safefile.Revision{}, fmt.Errorf("failed to read generated config %s: %w", rel, err)
	}
	return content, revision, nil
}

func verifyToolConfigRevision(root, rel string, expected safefile.Revision) error {
	_, current, err := readToolConfig(root, rel)
	if err != nil {
		return err
	}
	if current != expected {
		return fmt.Errorf("%w: generated config %s changed before replacement", safefile.ErrRevisionChanged, rel)
	}
	return nil
}

func replaceToolConfigAtRevision(root, rel string, expected safefile.Revision, content []byte) error {
	_, err := replaceToolConfigAtRevisionTracked(root, rel, expected, content)
	return err
}

func replaceToolConfigAtRevisionTracked(root, rel string, expected safefile.Revision, content []byte) (safefile.Revision, error) {
	if err := verifyToolConfigRevision(root, rel, expected); err != nil {
		return safefile.Revision{}, err
	}
	revision, err := safefile.ReplaceWithinRevisionTracked(root, rel, expected, content, 0600)
	if err != nil {
		return safefile.Revision{}, fmt.Errorf("failed to replace generated config %s: %w", rel, err)
	}
	return revision, nil
}

func replaceToolConfigAtRevisionNoCreateTracked(root, rel string, expected safefile.Revision, content []byte) (safefile.Revision, error) {
	if err := verifyToolConfigRevision(root, rel, expected); err != nil {
		return safefile.Revision{}, err
	}
	revision, err := safefile.ReplaceWithinRevisionNoCreateTracked(root, rel, expected, content, 0600)
	if err != nil {
		return safefile.Revision{}, fmt.Errorf("failed to replace generated config %s: %w", rel, err)
	}
	return revision, nil
}

func replaceToolConfigAtRevisionNoCreateAuthorizedTracked(root, rel string, expected safefile.Revision, parents *safefile.ParentChain, content []byte) (safefile.Revision, error) {
	if _, current, err := safefile.ReadWithinAuthorized(root, rel, parents); err != nil {
		return safefile.Revision{}, err
	} else if current != expected {
		return safefile.Revision{}, fmt.Errorf("%w: generated config %s changed before replacement", safefile.ErrRevisionChanged, rel)
	}
	revision, err := safefile.ReplaceWithinRevisionNoCreateAuthorizedTracked(root, rel, expected, parents, content, 0600)
	if err != nil {
		return revision, fmt.Errorf("failed to replace generated config %s: %w", rel, err)
	}
	return revision, nil
}

func prepareGeneratedConfigDestination(path string) (root, rel string, err error) {
	root, rel, createRoot, err := generatedConfigDestination(path)
	if err != nil {
		return "", "", err
	}
	if createRoot {
		// XDG_CONFIG_HOME is explicitly trusted in its entirety, just as HOME is.
		// It may not exist yet, so establish that anchor before safefile performs
		// descriptor-relative traversal of every untrusted descendant.
		if err := os.MkdirAll(root, 0700); err != nil {
			return "", "", fmt.Errorf("failed to create trusted XDG config root %s: %w", root, err)
		}
	}
	return root, rel, nil
}

func generatedConfigDestination(path string) (root, rel string, createRoot bool, err error) {
	cleanPath := filepath.Clean(path)
	if !filepath.IsAbs(cleanPath) {
		return "", "", false, fmt.Errorf("%w: %s", errGeneratedConfigOutsideTrustedRoots, path)
	}

	if home, homeErr := os.UserHomeDir(); homeErr == nil && filepath.IsAbs(home) {
		// Check the lexical HOME first so HOME itself may legitimately be a
		// symlink. Then check its resolved identity before considering XDG as a
		// separate trusted anchor. Without the second check, an XDG path spelling
		// the real location of a symlinked HOME could turn ~/.config from an
		// untrusted descendant into a trusted root and bypass the symlink guard.
		homeRoots := []string{filepath.Clean(home)}
		if resolved, resolveErr := filepath.EvalSymlinks(home); resolveErr == nil && filepath.IsAbs(resolved) {
			resolved = filepath.Clean(resolved)
			if resolved != homeRoots[0] {
				homeRoots = append(homeRoots, resolved)
			}
		}
		for _, homeRoot := range homeRoots {
			if relative, ok := relativePathBelow(homeRoot, cleanPath); ok {
				return homeRoot, relative, false, nil
			}
		}
		// macOS commonly exposes the same directory through /var and
		// /private/var. More generally, the destination may spell the resolved
		// HOME through a filesystem alias that is not lexically comparable. Find
		// an actual (non-symlink) ancestor with HOME's directory identity and use
		// that spelling as the trusted root. Descendant symlinks remain below the
		// anchor and are therefore refused by safefile.
		if homeRoot, relative, ok := relativePathBelowHomeIdentity(home, cleanPath); ok {
			return homeRoot, relative, false, nil
		}
	}

	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" && filepath.IsAbs(xdg) {
		if relative, ok := relativePathBelow(xdg, cleanPath); ok {
			return filepath.Clean(xdg), relative, true, nil
		}
	}

	return "", "", false, fmt.Errorf("%w: %s", errGeneratedConfigOutsideTrustedRoots, path)
}

func relativePathBelowHomeIdentity(home, path string) (root, rel string, ok bool) {
	homeInfo, err := os.Stat(home)
	if err != nil || !homeInfo.IsDir() {
		return "", "", false
	}
	for candidate := filepath.Dir(path); ; candidate = filepath.Dir(candidate) {
		candidateLstat, lstatErr := os.Lstat(candidate)
		if lstatErr == nil && candidateLstat.IsDir() && candidateLstat.Mode()&os.ModeSymlink == 0 {
			candidateInfo, statErr := os.Stat(candidate)
			if statErr == nil && os.SameFile(homeInfo, candidateInfo) {
				if relative, below := relativePathBelow(candidate, path); below {
					return filepath.Clean(candidate), relative, true
				}
			}
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			break
		}
	}
	return "", "", false
}

func relativePathBelow(root, path string) (string, bool) {
	cleanRoot := filepath.Clean(root)
	rel, err := filepath.Rel(cleanRoot, path)
	if err != nil || rel == "." || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}
