package backup

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/safefile"
)

type TargetKind string

const (
	TargetFile      TargetKind = "file"
	TargetDirectory TargetKind = "directory"
)

// Target is one exact mutation destination represented in a plan-derived
// rollback point. Missing targets are recorded explicitly so restore can
// remove paths created by the operation.
type Target struct {
	RelPath string
	Kind    TargetKind
}

// PlanResult is the committed rollback backup plus exact authority for its
// originally created root. Directory is a final recursive snapshot captured
// after the manifest commit; Parents and Directory together let rollback prove
// that the selected backup root has not been replaced.
type PlanResult struct {
	Count     int
	Directory *safefile.DirectorySnapshot
	Parents   *safefile.ParentChain
	anchor    string
	rel       string
	sources   map[string]planSource
	targets   []Target
}

// planSource is immutable descriptor authority for one persisted source below
// the plan backup root. backup.go consumes these private records so restore
// reads are exact-CAS authorized rather than merely bracketed by validation.
type planSource struct {
	target    Target
	file      safefile.Revision
	directory *safefile.DirectorySnapshot
	parents   *safefile.ParentChain
}

type planSourceExpectation struct {
	target Target
	digest [32]byte
	mode   os.FileMode
}

// ValidatePlanRoot proves that a tracked plan backup still has the exact root
// identity and complete recursive contents committed by CreatePlanTracked.
func ValidatePlanRoot(result PlanResult) error {
	if result.Directory == nil || !result.Parents.Tracked() || result.anchor == "" || result.rel == "" {
		return fmt.Errorf("%w: plan backup authority is incomplete", safefile.ErrDirectoryChanged)
	}
	current, err := safefile.SnapshotDirectoryWithinBudget(result.anchor, result.rel, backupSnapshotBudget)
	if err != nil {
		return err
	}
	if !safefile.SameDirectoryRootState(current, result.Directory) || current.Digest() != result.Directory.Digest() {
		return fmt.Errorf("%w: plan backup root or contents changed", safefile.ErrDirectoryChanged)
	}
	checkRel := filepath.ToSlash(filepath.Join(filepath.FromSlash(result.rel), ".validate-authority"))
	if _, err := safefile.ExtendParentChainWithinDirectory(result.anchor, checkRel, result.rel, result.Parents, result.Directory); err != nil {
		return err
	}
	return nil
}

const manifestV2Header = `# dotfiles-backup-manifest v2
# preserves: file bytes, directory structure, and POSIX owner/group/other rwx bits
# ownership-policy: captured nodes must match the process effective uid:gid; ownership is not serialized
# not-preserved: ACLs, extended attributes, file flags, hard-link topology, and timestamps
`

const (
	planManifestRecords = "# records: absolute-original|backup|yes-or-no|file-or-directory|octal-mode\n"
	flatManifestRecords = "# records: home-relative-path<TAB>octal-mode; payload uses legacy flat-name encoding\n"
)

type backupLocation struct {
	path         string
	anchor       string
	rel          string
	rootParents  *safefile.ParentChain
	rootSnapshot *safefile.DirectorySnapshot
	directories  map[string]backupDirectoryAuthority
}

type backupDirectoryAuthority struct {
	snapshot *safefile.DirectorySnapshot
	parents  *safefile.ParentChain
}

// joinFailedRootCleanup removes only the root created and owned by this
// backupLocation. Cleanup first snapshots the live recursive tree, then proves
// both its original root identity and accepted parent chain before authorizing
// exact-snapshot removal. A replaced or grafted root is never path-deleted.
func (location backupLocation) joinFailedRootCleanup(creationErr error) error {
	cleanupErr := location.removeFailedRoot()
	if cleanupErr == nil {
		return creationErr
	}
	return errors.Join(creationErr, fmt.Errorf("clean failed backup root: %w", cleanupErr))
}

func (location backupLocation) removeFailedRoot() error {
	if location.anchor == "" || location.rel == "" || location.rootSnapshot == nil || !location.rootParents.Tracked() {
		return fmt.Errorf("%w: failed backup root authority is incomplete", safefile.ErrDirectoryChanged)
	}
	current, err := safefile.SnapshotDirectoryWithinBudget(location.anchor, location.rel, backupSnapshotBudget)
	if err != nil {
		return fmt.Errorf("snapshot failed backup root: %w", err)
	}
	if !safefile.SameDirectoryRootState(current, location.rootSnapshot) {
		return fmt.Errorf("%w: failed backup root identity changed", safefile.ErrDirectoryChanged)
	}
	bound, err := safefile.BindParentChainWithin(location.anchor, location.rel, location.rootParents, nil)
	if err != nil {
		return fmt.Errorf("bind failed backup root parent chain: %w", err)
	}
	if !safefile.SameParentChain(bound, location.rootParents) {
		return fmt.Errorf("%w: failed backup root parent chain changed", safefile.ErrParentChanged)
	}
	if err := safefile.RemoveDirectoryWithinSnapshotAuthorized(location.anchor, location.rel, current, location.rootParents); err != nil {
		return fmt.Errorf("remove exact failed backup root: %w", err)
	}
	return nil
}

var backupCreateTestHooks struct {
	afterRootPrefix    func(prefix string) error
	afterManifestWrite func(path string) error
}

// CreatePlan creates a fail-closed rollback point for an exact action-plan
// scope. Sources are read descriptor-relatively below HOME; files keep their
// exact modes, directories use opaque recursive snapshots, and absent targets
// are written to the manifest as existed=no. A persisted manifest becomes an
// authoritative rollback point only after final manifest/source/root digest
// validation succeeds. Failed-root cleanup is identity-bound and a failed call
// never returns PlanResult authority, including when hostile replacement makes
// cleanup refuse the live name.
func CreatePlan(home, backupDir string, targets []Target) (int, error) {
	result, err := CreatePlanTracked(home, backupDir, targets)
	return result.Count, err
}

// CreatePlanTracked creates the same rollback point as CreatePlan and returns
// exact final backup-root evidence for poisoning-resistant rollback reads.
func CreatePlanTracked(home, backupDir string, targets []Target) (PlanResult, error) {
	return createPlanTracked(home, backupDir, targets, nil)
}

// CreatePlanTrackedWithState creates a plan backup only at one exact absent
// child of the reviewed operational backups namespace.
func CreatePlanTrackedWithState(home, backupDir string, targets []Target, state *operation.StateAuthority) (PlanResult, error) {
	name := filepath.Base(filepath.Clean(backupDir))
	root, rel, path, parents, err := operation.StateChildTargetAuthority(state, "backups", name)
	if err != nil {
		return PlanResult{}, err
	}
	if filepath.Clean(path) != filepath.Clean(backupDir) {
		return PlanResult{}, fmt.Errorf("plan backup path does not match accepted state authority")
	}
	prepare := func(_, _ string) (backupLocation, error) {
		evidence, err := safefile.EnsureShallowDirectoryWithinParentChainTracked(root, rel, nil, parents, 0o700)
		if err != nil {
			return backupLocation{}, fmt.Errorf("create accepted plan backup root: %w", err)
		}
		return backupLocation{path: path, anchor: root, rel: rel, rootParents: parents, rootSnapshot: evidence, directories: make(map[string]backupDirectoryAuthority)}, nil
	}
	return createPlanTracked(home, backupDir, targets, prepare)
}

func createPlanTracked(home, backupDir string, targets []Target, prepare func(home, backupDir string) (backupLocation, error)) (PlanResult, error) {
	fail := func(count int, err error) (PlanResult, error) {
		return PlanResult{Count: count}, err
	}
	if len(targets) == 0 {
		return fail(0, fmt.Errorf("plan backup scope is empty"))
	}
	if len(targets) > maxBackupManifestEntries {
		return fail(0, fmt.Errorf("plan backup scope exceeds %d entries", maxBackupManifestEntries))
	}
	if !filepath.IsAbs(home) || !filepath.IsAbs(backupDir) {
		return fail(0, fmt.Errorf("home and backup directory must be absolute"))
	}
	plannedTargets, err := normalizePlanTargets(home, backupDir, targets)
	if err != nil {
		return fail(0, err)
	}
	if prepare == nil {
		prepare = prepareBackupDirectory
	}
	location, err := prepare(home, backupDir)
	if err != nil {
		return fail(0, err)
	}
	failPrepared := func(count int, creationErr error) (PlanResult, error) {
		return PlanResult{Count: count}, location.joinFailedRootCleanup(creationErr)
	}

	manifest := make([]string, 0, len(targets))
	persisted := make([]planSourceExpectation, 0, len(targets))
	captured := 0
	snapshotFiles := 0
	var payloadBytes int64
	for _, target := range plannedTargets {
		rel := target.RelPath

		original := filepath.Join(home, filepath.FromSlash(rel))
		backupPath := filepath.Join(location.path, filepath.FromSlash(rel))
		if strings.ContainsRune(original, '|') || strings.ContainsRune(backupPath, '|') {
			return failPrepared(captured, fmt.Errorf("backup paths containing '|' are unsupported"))
		}
		switch target.Kind {
		case TargetFile:
			if snapshotFiles >= maxBackupSnapshotFiles {
				return failPrepared(captured, fmt.Errorf("backup snapshot exceeds %d files", maxBackupSnapshotFiles))
			}
			limit, limitErr := boundedBackupFileLimit(payloadBytes)
			if limitErr != nil {
				return failPrepared(captured, limitErr)
			}
			data, revision, _, err := safefile.ObserveFileWithinLimit(home, rel, limit)
			if err != nil {
				return failPrepared(captured, fmt.Errorf("read plan backup target %s: %w", rel, err))
			}
			if !revision.Exists() {
				manifest = append(manifest, fmt.Sprintf("%s||no|file|600", original))
				continue
			}
			if err := location.writeFile(rel, data, revision.Permissions()); err != nil {
				return failPrepared(captured, fmt.Errorf("store plan backup file %s: %w", rel, err))
			}
			manifest = append(manifest, fmt.Sprintf("%s|%s|yes|file|%o", original, backupPath, revision.Permissions()))
			persisted = append(persisted, planSourceExpectation{
				target: Target{RelPath: rel, Kind: TargetFile},
				digest: revision.Digest(),
				mode:   revision.Permissions(),
			})
			payloadBytes += int64(len(data))
			snapshotFiles++
			captured++
		case TargetDirectory:
			remaining, limitErr := remainingBackupBytes(payloadBytes)
			if limitErr != nil {
				return failPrepared(captured, limitErr)
			}
			snapshot, _, err := safefile.ObserveDirectoryWithinBudget(home, rel, safefile.SnapshotBudget{
				MaxFiles: maxBackupSnapshotFiles - snapshotFiles, MaxFileBytes: maxBackupFileBytes, MaxTotalBytes: remaining,
			})
			if errors.Is(err, os.ErrNotExist) {
				manifest = append(manifest, fmt.Sprintf("%s||no|directory|700", original))
				continue
			}
			if err != nil {
				return failPrepared(captured, fmt.Errorf("snapshot plan backup directory %s: %w", rel, err))
			}
			if err := location.writeDirectory(rel, snapshot); err != nil {
				return failPrepared(captured, fmt.Errorf("store plan backup directory %s: %w", rel, err))
			}
			manifest = append(manifest, fmt.Sprintf("%s|%s|yes|directory|%o", original, backupPath, snapshot.Permissions()))
			persisted = append(persisted, planSourceExpectation{
				target: Target{RelPath: rel, Kind: TargetDirectory},
				digest: snapshot.Digest(),
				mode:   snapshot.Permissions(),
			})
			files, size := snapshot.RecursiveFileStats()
			snapshotFiles += files
			payloadBytes += size
			captured++
		}
	}

	manifestBytes, err := generatedManifestSize(manifest, len(manifestV2Header)+len(planManifestRecords))
	if err != nil {
		return failPrepared(captured, err)
	}
	if manifestBytes > maxBackupTotalBytes-payloadBytes {
		return failPrepared(captured, fmt.Errorf("backup payload exceeds %d total bytes", maxBackupTotalBytes))
	}
	data := []byte(manifestV2Header + planManifestRecords + strings.Join(manifest, "\n") + "\n")
	if err := location.writeFile(ManifestName, data, 0o600); err != nil {
		return failPrepared(captured, fmt.Errorf("commit plan backup manifest: %w", err))
	}
	if hook := backupCreateTestHooks.afterManifestWrite; hook != nil {
		if err := hook(filepath.Join(location.path, ManifestName)); err != nil {
			return failPrepared(captured, fmt.Errorf("after plan manifest commit: %w", err))
		}
	}
	final, err := location.finalSnapshot()
	if err != nil {
		return failPrepared(captured, fmt.Errorf("capture final plan backup authority: %w", err))
	}
	sources, err := location.capturePlanSources(persisted, data)
	if err != nil {
		return failPrepared(captured, fmt.Errorf("capture plan backup source authority: %w", err))
	}
	result := PlanResult{
		Count:     captured,
		Directory: final,
		Parents:   location.rootParents,
		anchor:    location.anchor,
		rel:       location.rel,
		sources:   sources,
		targets:   plannedTargets,
	}
	if err := ValidatePlanRoot(result); err != nil {
		return failPrepared(captured, fmt.Errorf("validate final plan backup authority: %w", err))
	}
	return result, nil
}

// Create backs up each of the given files (paths relative to home) into
// backupDir, writing a manifest that records the original path and mode of
// every captured file. Sources and destinations are observed and copied below
// descriptor anchors without following symlinks. Every source must be a stable
// regular file owned by the process effective uid:gid. backupDir is created if
// as a new, uniquely absent directory below process-owned, non-group/world-
// writable ancestors.
//
// It returns the number of files actually captured. Unlike the old inline
// loops, it does NOT silently report success when nothing was backed up:
//   - if zero files were captured (none existed / all unreadable) it returns
//     an error and writes no manifest, so callers cannot claim a rollback
//     point that does not exist (C4/C5);
//   - if the manifest write fails it returns that error.
//
// Missing candidates are skipped. Any other observation or persistence error
// aborts the backup so callers cannot mistake a partial capture for a rollback
// point.
func Create(home, backupDir string, files []string) (int, error) {
	if len(files) == 0 {
		return 0, fmt.Errorf("backup candidate list is empty")
	}
	if len(files) > maxBackupManifestEntries {
		return 0, fmt.Errorf("backup candidate list exceeds %d entries", maxBackupManifestEntries)
	}
	if !filepath.IsAbs(home) || !filepath.IsAbs(backupDir) {
		return 0, fmt.Errorf("home and backup directory must be absolute")
	}
	candidates, err := normalizeFlatCandidates(home, files)
	if err != nil {
		return 0, err
	}
	location, err := prepareBackupDirectory(home, backupDir)
	if err != nil {
		return 0, err
	}
	failPrepared := func(count int, creationErr error) (int, error) {
		return count, location.joinFailedRootCleanup(creationErr)
	}

	var manifest []string
	count := 0
	var payloadBytes int64
	for _, candidate := range candidates {
		rel := candidate.rel
		stored := candidate.stored

		limit, limitErr := boundedBackupFileLimit(payloadBytes)
		if limitErr != nil {
			return failPrepared(count, limitErr)
		}
		data, revision, _, err := safefile.ObserveFileWithinLimit(home, rel, limit)
		if err != nil {
			return failPrepared(count, fmt.Errorf("observe backup candidate %s: %w", rel, err))
		}
		if !revision.Exists() {
			continue
		}

		// Flat storage name (human-readable); the manifest is the
		// authoritative source for the original path on restore.
		if err := location.writeFile(stored, data, 0o600); err != nil {
			return failPrepared(count, fmt.Errorf("store backup candidate %s: %w", rel, err))
		}

		// Record the original path and mode so restore can reconstruct both
		// exactly (the underscore encoding is lossy).
		manifest = append(manifest, ManifestLine(rel, revision.Permissions()))
		payloadBytes += int64(len(data))
		count++
	}

	if count == 0 {
		return failPrepared(0, fmt.Errorf("no files were backed up (none of %d candidate files were present)", len(files)))
	}

	manifestBytes, err := generatedManifestSize(manifest, len(manifestV2Header)+len(flatManifestRecords))
	if err != nil {
		return failPrepared(count, err)
	}
	if manifestBytes > maxBackupTotalBytes-payloadBytes {
		return failPrepared(count, fmt.Errorf("backup payload exceeds %d total bytes", maxBackupTotalBytes))
	}
	data := []byte(manifestV2Header + flatManifestRecords + strings.Join(manifest, "\n") + "\n")
	if err := location.writeFile(ManifestName, data, 0o600); err != nil {
		return failPrepared(count, fmt.Errorf("write backup manifest: %w", err))
	}

	return count, nil
}

type flatCandidate struct {
	rel    string
	stored string
}

func normalizeFlatCandidates(home string, files []string) ([]flatCandidate, error) {
	candidates := make([]flatCandidate, 0, len(files))
	seenSources := make(map[string]struct{}, len(files))
	seenDestinations := make(map[string]string, len(files))
	for _, relPath := range files {
		rel, err := validateFileTarget(home, relPath)
		if err != nil {
			return nil, err
		}
		if _, duplicate := seenSources[rel]; duplicate {
			return nil, fmt.Errorf("duplicate backup candidate %q", rel)
		}
		seenSources[rel] = struct{}{}
		stored := EncodeName(rel)
		if stored == ManifestName {
			return nil, fmt.Errorf("backup candidate %q collides with reserved manifest name", rel)
		}
		if previous, collision := seenDestinations[stored]; collision {
			return nil, fmt.Errorf("backup candidates %q and %q collide in legacy flat storage", previous, rel)
		}
		seenDestinations[stored] = rel
		candidates = append(candidates, flatCandidate{rel: rel, stored: stored})
	}
	return candidates, nil
}

func normalizePlanTargets(home, backupDir string, targets []Target) ([]Target, error) {
	seen := make(map[string]struct{}, len(targets))
	normalized := make([]Target, 0, len(targets))
	cleanBackup := filepath.Clean(backupDir)
	for _, target := range targets {
		rel := filepath.ToSlash(filepath.Clean(filepath.FromSlash(target.RelPath)))
		if rel == "." || rel == "" || filepath.IsAbs(rel) || !IsRestorePathSafe(home, filepath.FromSlash(rel)) || strings.ContainsAny(rel, "|\r\n\x00") {
			return nil, fmt.Errorf("invalid plan backup target %q", target.RelPath)
		}
		if target.Kind != TargetFile && target.Kind != TargetDirectory {
			return nil, fmt.Errorf("invalid plan backup target kind %q for %s", target.Kind, rel)
		}
		if _, duplicate := seen[rel]; duplicate {
			return nil, fmt.Errorf("duplicate plan backup target %q", rel)
		}
		seen[rel] = struct{}{}
		original := filepath.Join(home, filepath.FromSlash(rel))
		stored := filepath.Join(cleanBackup, filepath.FromSlash(rel))
		if strings.ContainsRune(original, '|') || strings.ContainsRune(stored, '|') {
			return nil, fmt.Errorf("backup paths containing '|' are unsupported")
		}
		normalized = append(normalized, Target{RelPath: rel, Kind: target.Kind})
	}
	return normalized, nil
}

func prepareBackupDirectory(home, backupDir string) (backupLocation, error) {
	if !filepath.IsAbs(home) || !filepath.IsAbs(backupDir) {
		return backupLocation{}, fmt.Errorf("home and backup directory must be absolute")
	}
	cleanBackup := filepath.Clean(backupDir)
	backupParent := filepath.Dir(cleanBackup)
	backupName := filepath.Base(cleanBackup)
	if backupName == "." || backupParent == cleanBackup {
		return backupLocation{}, fmt.Errorf("invalid backup directory %q", backupDir)
	}
	location := backupLocation{
		path:        cleanBackup,
		anchor:      backupParent,
		rel:         backupName,
		directories: make(map[string]backupDirectoryAuthority),
	}
	if rel, relErr := filepath.Rel(home, cleanBackup); relErr == nil && IsRestorePathSafe(home, rel) {
		// HOME remains the trust boundary for production backups. Promoting an
		// existing descendant would let a symlink redirect persisted recovery
		// data outside HOME.
		location.anchor = home
		location.rel = filepath.ToSlash(rel)
	} else {
		// Explicit external locations use their nearest existing ancestor as a
		// caller-supplied trust boundary. Every descendant is still traversed
		// descriptor-relatively and must be owned by the effective user.
		for {
			info, statErr := os.Lstat(location.anchor)
			if statErr == nil {
				if info.Mode()&os.ModeSymlink != 0 {
					return backupLocation{}, fmt.Errorf("%w: backup ancestor %s", safefile.ErrSymlink, location.anchor)
				}
				if !info.IsDir() {
					return backupLocation{}, fmt.Errorf("backup ancestor is not a directory: %s", location.anchor)
				}
				break
			}
			if !errors.Is(statErr, os.ErrNotExist) {
				return backupLocation{}, fmt.Errorf("inspect backup ancestor %s: %w", location.anchor, statErr)
			}
			parent := filepath.Dir(location.anchor)
			if parent == location.anchor {
				return backupLocation{}, fmt.Errorf("no existing backup ancestor for %s", backupDir)
			}
			location.rel = filepath.ToSlash(filepath.Join(filepath.Base(location.anchor), filepath.FromSlash(location.rel)))
			location.anchor = parent
		}
	}
	components := strings.Split(filepath.ToSlash(location.rel), "/")
	prefixes := make([]string, 0, len(components))
	missing := make(map[string]bool, len(components))
	for index := range components {
		prefix := strings.Join(components[:index+1], "/")
		prefixes = append(prefixes, prefix)
		_, statErr := os.Lstat(filepath.Join(location.anchor, filepath.FromSlash(prefix)))
		switch {
		case errors.Is(statErr, os.ErrNotExist):
			missing[prefix] = true
		case statErr != nil:
			return backupLocation{}, fmt.Errorf("inspect backup namespace prefix %s: %w", prefix, statErr)
		}
	}
	if !missing[location.rel] {
		return backupLocation{}, fmt.Errorf("backup directory already exists: %s", cleanBackup)
	}
	acceptedFull, err := safefile.CaptureParentChainWithin(location.anchor, location.rel)
	if err != nil {
		return backupLocation{}, fmt.Errorf("capture intended backup namespace: %w", err)
	}
	created := make(map[string]*safefile.DirectorySnapshot)
	for _, prefix := range prefixes {
		if !missing[prefix] {
			continue
		}
		parents, err := safefile.BindParentChainPrefixWithin(location.anchor, location.rel, prefix, acceptedFull, created)
		if err != nil {
			return backupLocation{}, fmt.Errorf("bind backup namespace prefix %s: %w", prefix, err)
		}
		evidence, err := safefile.EnsureShallowDirectoryWithinParentChainTracked(location.anchor, prefix, nil, parents, 0o700)
		if err != nil {
			return backupLocation{}, fmt.Errorf("create accepted backup namespace prefix %s: %w", prefix, err)
		}
		created[prefix] = evidence
		if prefix == location.rel {
			location.rootParents = parents
			location.rootSnapshot = evidence
		}
		if hook := backupCreateTestHooks.afterRootPrefix; hook != nil {
			if err := hook(prefix); err != nil {
				prepareErr := fmt.Errorf("after creating backup namespace prefix %s: %w", prefix, err)
				if prefix == location.rel && location.rootSnapshot != nil {
					prepareErr = location.joinFailedRootCleanup(prepareErr)
				}
				return backupLocation{}, prepareErr
			}
		}
	}
	if location.rootSnapshot == nil || !location.rootParents.Tracked() {
		return backupLocation{}, fmt.Errorf("backup directory creation produced no exact authority")
	}
	return location, nil
}

func validateFileTarget(home, relPath string) (string, error) {
	rel := filepath.ToSlash(filepath.Clean(filepath.FromSlash(relPath)))
	if rel == "." || rel == "" || filepath.IsAbs(rel) || !IsRestorePathSafe(home, filepath.FromSlash(rel)) || strings.ContainsAny(rel, "\t\r\n\x00") {
		return "", fmt.Errorf("invalid backup candidate %q", relPath)
	}
	return rel, nil
}

func (location backupLocation) fullRel(rel string) string {
	return filepath.ToSlash(filepath.Join(filepath.FromSlash(location.rel), filepath.FromSlash(rel)))
}

func (location backupLocation) ensureParent(rel string) error {
	parent := filepath.ToSlash(filepath.Dir(filepath.FromSlash(rel)))
	if parent == "." {
		return nil
	}
	components := strings.Split(parent, "/")
	for index := range components {
		prefix := strings.Join(components[:index+1], "/")
		if authority, exists := location.directories[prefix]; exists && authority.snapshot != nil {
			continue
		}
		fullRel := location.fullRel(prefix)
		parents, err := location.authority(fullRel)
		if err != nil {
			return err
		}
		evidence, err := safefile.EnsureShallowDirectoryWithinParentChainTracked(location.anchor, fullRel, nil, parents, 0o700)
		if err != nil {
			return fmt.Errorf("create backup storage parent %s: %w", prefix, err)
		}
		location.directories[prefix] = backupDirectoryAuthority{snapshot: evidence, parents: parents}
	}
	return nil
}

func (location backupLocation) authority(fullRel string) (*safefile.ParentChain, error) {
	rel, err := filepath.Rel(filepath.FromSlash(location.rel), filepath.FromSlash(fullRel))
	if err != nil {
		return nil, err
	}
	rel = filepath.ToSlash(rel)
	parent := filepath.ToSlash(filepath.Dir(filepath.FromSlash(rel)))
	for parent != "." && parent != "" {
		if base, exists := location.directories[parent]; exists && base.snapshot != nil && base.parents.Tracked() {
			baseRel := location.fullRel(parent)
			return safefile.ExtendParentChainWithinDirectory(location.anchor, fullRel, baseRel, base.parents, base.snapshot)
		}
		next := filepath.ToSlash(filepath.Dir(filepath.FromSlash(parent)))
		if next == parent {
			break
		}
		parent = next
	}
	return safefile.ExtendParentChainWithinDirectory(location.anchor, fullRel, location.rel, location.rootParents, location.rootSnapshot)
}

func (location backupLocation) finalSnapshot() (*safefile.DirectorySnapshot, error) {
	checkRel := location.fullRel(".final-authority-check")
	if _, err := location.authority(checkRel); err != nil {
		return nil, err
	}
	final, err := safefile.SnapshotDirectoryWithinBudget(location.anchor, location.rel, backupSnapshotBudget)
	if err != nil {
		return nil, err
	}
	// The recursive snapshot must describe the same originally created root,
	// not a replacement raced into the namespace during capture. Validate both
	// the creation evidence and final evidence against the live name.
	if _, err := location.authority(checkRel); err != nil {
		return nil, err
	}
	if _, err := safefile.ExtendParentChainWithinDirectory(location.anchor, checkRel, location.rel, location.rootParents, final); err != nil {
		return nil, err
	}
	return final, nil
}

func (location backupLocation) capturePlanSources(expected []planSourceExpectation, manifestData []byte) (map[string]planSource, error) {
	sources := make(map[string]planSource, len(expected)+1)
	manifest, err := location.capturePlanFile(ManifestName, Target{RelPath: ManifestName, Kind: TargetFile})
	if err != nil {
		return nil, fmt.Errorf("capture manifest authority: %w", err)
	}
	if !manifest.file.Exists() {
		return nil, fmt.Errorf("manifest disappeared after commit")
	}
	if manifest.file.Digest() != sha256.Sum256(manifestData) || manifest.file.Permissions() != 0o600 {
		return nil, fmt.Errorf("plan backup manifest differs from committed bytes or mode")
	}
	sources[ManifestName] = manifest

	for _, wanted := range expected {
		target := wanted.target
		rel := filepath.ToSlash(filepath.Clean(filepath.FromSlash(target.RelPath)))
		switch target.Kind {
		case TargetFile:
			source, err := location.capturePlanFile(rel, target)
			if err != nil {
				return nil, err
			}
			if source.file.Exists() {
				if source.file.Digest() != wanted.digest || source.file.Permissions() != wanted.mode {
					return nil, fmt.Errorf("copied plan backup file %s differs from observed source", rel)
				}
				sources[rel] = source
			} else {
				return nil, fmt.Errorf("copied plan backup file %s disappeared", rel)
			}
		case TargetDirectory:
			source, exists, err := location.capturePlanDirectory(rel, target)
			if err != nil {
				return nil, err
			}
			if exists {
				if source.directory.Digest() != wanted.digest || source.directory.Permissions() != wanted.mode {
					return nil, fmt.Errorf("copied plan backup directory %s differs from observed source", rel)
				}
				sources[rel] = source
			} else {
				return nil, fmt.Errorf("copied plan backup directory %s disappeared", rel)
			}
		default:
			return nil, fmt.Errorf("invalid source target kind %q", target.Kind)
		}
	}
	return sources, nil
}

func (location backupLocation) capturePlanFile(rel string, target Target) (planSource, error) {
	fullRel := location.fullRel(rel)
	parents, err := location.authority(fullRel)
	if err != nil {
		return planSource{}, err
	}
	limit := maxBackupFileBytes
	if rel == ManifestName {
		limit = maxBackupManifestBytes
	}
	_, revision, err := safefile.ReadWithinAuthorizedLimit(location.anchor, fullRel, parents, limit)
	if err != nil {
		return planSource{}, err
	}
	return planSource{target: target, file: revision, parents: parents}, nil
}

func (location backupLocation) capturePlanDirectory(rel string, target Target) (planSource, bool, error) {
	fullRel := location.fullRel(rel)
	parents, err := location.authority(fullRel)
	if err != nil {
		return planSource{}, false, err
	}
	snapshot, err := safefile.SnapshotDirectoryWithinBudget(location.anchor, fullRel, backupSnapshotBudget)
	if errors.Is(err, os.ErrNotExist) {
		return planSource{target: target, parents: parents}, false, nil
	}
	if err != nil {
		return planSource{}, false, err
	}
	after, err := location.authority(fullRel)
	if err != nil {
		return planSource{}, false, err
	}
	if !safefile.SameParentChain(parents, after) {
		return planSource{}, false, fmt.Errorf("%w: backup source parent changed during capture", safefile.ErrParentChanged)
	}
	return planSource{target: target, directory: snapshot, parents: parents}, true, nil
}

func (location backupLocation) writeFile(rel string, data []byte, mode os.FileMode) error {
	if err := location.ensureParent(rel); err != nil {
		return err
	}
	fullRel := location.fullRel(rel)
	parents, err := location.authority(fullRel)
	if err != nil {
		return err
	}
	_, revision, err := safefile.ReadWithinAuthorizedLimit(location.anchor, fullRel, parents, 0)
	if err != nil {
		return err
	}
	if revision.Exists() {
		return fmt.Errorf("backup destination already exists: %s", filepath.Join(location.path, filepath.FromSlash(rel)))
	}
	_, err = safefile.ReplaceWithinRevisionNoCreateAuthorizedTracked(location.anchor, fullRel, revision, parents, data, mode.Perm())
	return err
}

func (location backupLocation) writeDirectory(rel string, snapshot *safefile.DirectorySnapshot) error {
	if err := location.ensureParent(rel); err != nil {
		return err
	}
	fullRel := location.fullRel(rel)
	parents, err := location.authority(fullRel)
	if err != nil {
		return err
	}
	existing, err := safefile.SnapshotDirectoryWithinBudget(location.anchor, fullRel, backupSnapshotBudget)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if existing != nil || err == nil {
		return fmt.Errorf("backup destination already exists: %s", filepath.Join(location.path, filepath.FromSlash(rel)))
	}
	evidence, err := safefile.RestoreDirectoryWithinSnapshotNoCreateAuthorizedTracked(location.anchor, fullRel, snapshot, nil, parents)
	if err == nil {
		location.directories[filepath.ToSlash(rel)] = backupDirectoryAuthority{snapshot: evidence, parents: parents}
	}
	return err
}
