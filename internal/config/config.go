package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/tekierz/dotfiles/internal/safefile"
)

// ErrNoConfigDir is returned when the config directory cannot be determined
// (e.g. neither XDG_CONFIG_HOME nor HOME is set and os.UserHomeDir fails).
// It guards against silently building relative paths from an empty base.
var ErrNoConfigDir = errors.New("cannot determine config directory: HOME and XDG_CONFIG_HOME are unset")

// ErrGlobalConfigConflict reports that global.json no longer matches the
// source revision from which a GlobalConfig was loaded. Callers must reload,
// reconcile their changes, and save again rather than overwriting newer data.
var ErrGlobalConfigConflict = errors.New("global config changed since it was loaded")

// ErrGlobalConfigLockUnsupported reports that this operating system cannot
// provide the descriptor-anchored inter-process lock required for safe saves.
var ErrGlobalConfigLockUnsupported = errors.New("global config locking is unsupported on this platform")

var errConfigPathOutsideTrustedRoots = errors.New("config path is outside HOME and XDG_CONFIG_HOME")

// GlobalConfigCommittedError reports a failure discovered after global.json
// was atomically replaced. Callers must not blindly retry because the requested
// bytes may already have been committed.
type GlobalConfigCommittedError struct {
	Operation string
	Err       error
}

func (e *GlobalConfigCommittedError) Error() string {
	return fmt.Sprintf("global config committed; %s failed: %v", e.Operation, e.Err)
}

func (e *GlobalConfigCommittedError) Unwrap() error { return e.Err }

func (e *GlobalConfigCommittedError) Committed() bool { return true }

// CurrentGlobalConfigSchemaVersion is the schema written to global.json.
// Files without a schema_version predate versioning and are treated as schema
// 0. Loading schema 0 starts from current defaults and overlays every JSON key,
// so newly introduced settings receive safe defaults while explicit false and
// zero values remain intact. Future versions are rejected rather than silently
// interpreted with older semantics.
const CurrentGlobalConfigSchemaVersion = 1

// writeFileAtomic writes data below a trusted filesystem anchor. Safefile owns
// descriptor-relative traversal, no-follow enforcement, staging, rename, and
// directory durability; callers never resolve untrusted descendants by path.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	root, rel, err := anchoredFilePath(path)
	if err != nil {
		return err
	}
	return safefile.ReplaceWithin(root, rel, data, perm)
}

// anchoredFilePath splits an absolute destination into one trusted, existing
// root and a relative descendant path. HOME wins over an XDG alias within the
// same filesystem identity so ~/.config remains an untrusted descendant and a
// symlink there cannot be promoted into a new trusted root. An external absolute
// XDG_CONFIG_HOME remains explicitly trusted. Safefile owns all traversal below
// the chosen anchor.
func anchoredFilePath(path string) (string, string, error) {
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) {
		return "", "", fmt.Errorf("%w: destination must be absolute: %q", errConfigPathOutsideTrustedRoots, path)
	}

	if home, err := os.UserHomeDir(); err == nil && filepath.IsAbs(home) {
		homeRoots := []string{filepath.Clean(home)}
		if resolved, resolveErr := filepath.EvalSymlinks(home); resolveErr == nil && filepath.IsAbs(resolved) {
			resolved = filepath.Clean(resolved)
			if resolved != homeRoots[0] {
				homeRoots = append(homeRoots, resolved)
			}
		}
		for _, root := range homeRoots {
			if rel, inside := relativeDescendant(root, clean); inside {
				return root, rel, nil
			}
		}
		if root, rel, ok := relativePathBelowHomeIdentity(home, clean); ok {
			return root, rel, nil
		}
	}

	if xdg := os.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(xdg) {
		xdg = filepath.Clean(xdg)
		if _, inside := relativeDescendant(xdg, clean); inside {
			info, statErr := os.Stat(xdg)
			switch {
			case statErr == nil && info.IsDir():
				rel, _ := relativeDescendant(xdg, clean)
				return xdg, rel, nil
			case statErr == nil:
				return "", "", fmt.Errorf("trusted XDG config root %q is not a directory", xdg)
			case !errors.Is(statErr, os.ErrNotExist):
				return "", "", fmt.Errorf("inspect trusted XDG config root %q: %w", xdg, statErr)
			}

			// A not-yet-created external XDG root has no descriptor to anchor.
			// Anchor at its nearest existing ancestor and let safefile create and
			// verify every missing descendant directory.
			root := filepath.Dir(xdg)
			for {
				if ancestorInfo, ancestorErr := os.Stat(root); ancestorErr == nil {
					if !ancestorInfo.IsDir() {
						return "", "", fmt.Errorf("XDG config ancestor %q is not a directory", root)
					}
					rel, below := relativeDescendant(root, clean)
					if !below {
						return "", "", fmt.Errorf("%w: %q is not below XDG ancestor %q", errConfigPathOutsideTrustedRoots, clean, root)
					}
					return root, rel, nil
				}
				parent := filepath.Dir(root)
				if parent == root {
					break
				}
				root = parent
			}
		}
	}

	return "", "", fmt.Errorf("%w: %q", errConfigPathOutsideTrustedRoots, path)
}

// ensureConfiguredXDGRoot establishes a missing explicitly configured XDG
// root through a descriptor-anchored ancestor before a global-config lock is
// selected. Without this step, first creation can change anchoredFilePath from
// the nearest existing ancestor to XDG_CONFIG_HOME while a writer still holds
// a lock at the old anchor, allowing a second process to use a different lock.
func ensureConfiguredXDGRoot(path string) error {
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if !filepath.IsAbs(xdg) {
		return nil
	}
	xdg = filepath.Clean(xdg)
	if _, inside := relativeDescendant(xdg, filepath.Clean(path)); !inside {
		return nil
	}

	info, err := os.Stat(xdg)
	switch {
	case err == nil && info.IsDir():
		return nil
	case err == nil:
		return fmt.Errorf("trusted XDG config root %q is not a directory", xdg)
	case !errors.Is(err, os.ErrNotExist):
		return fmt.Errorf("inspect trusted XDG config root %q: %w", xdg, err)
	}

	root, _, err := anchoredFilePath(path)
	if err != nil {
		return err
	}
	if filepath.Clean(root) == xdg {
		// Another actor created the directory after the initial stat. The anchor
		// resolver already proved that it is now a directory.
		return nil
	}
	rel, inside := relativeDescendant(root, xdg)
	if !inside {
		return fmt.Errorf("%w: XDG root %q is not below trusted ancestor %q", errConfigPathOutsideTrustedRoots, xdg, root)
	}
	if err := safefile.EnsureDirectoryWithin(root, rel, 0o700); err != nil {
		if errors.Is(err, safefile.ErrUnsupported) {
			return errors.Join(ErrGlobalConfigLockUnsupported, err)
		}
		return fmt.Errorf("establish trusted XDG config root %q: %w", xdg, err)
	}
	return nil
}

func relativeDescendant(root, path string) (string, bool) {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", false
	}
	return filepath.ToSlash(rel), true
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
				if relative, below := relativeDescendant(candidate, path); below {
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

func globalConfigLockRel(targetRel string) string {
	// The lock lives in the same already-openable trusted anchor as global.json.
	// Hashing the complete relative target keeps distinct configs below one root
	// from sharing a lock and avoids exposing descendant path separators in the
	// lock name. Symlink spellings of one trusted root still open the same inode.
	return fmt.Sprintf(".dotfiles-global-config-%x.lock", sha256.Sum256([]byte(targetRel)))
}

func acquireGlobalConfigLock(root, rel string) (func() error, error) {
	release, err := safefile.AcquireLockWithin(root, rel, 0600)
	if errors.Is(err, safefile.ErrUnsupported) {
		return nil, errors.Join(ErrGlobalConfigLockUnsupported, err)
	}
	return release, err
}

// GlobalConfig holds global dotfiles settings
type GlobalConfig struct {
	SchemaVersion     int    `json:"schema_version,omitempty"`
	Theme             string `json:"theme"`
	NavStyle          string `json:"nav_style"`
	ActiveUser        string `json:"active_user,omitempty"`
	DisableAnimations bool   `json:"disable_animations,omitempty"`

	// Backup settings
	AutoBackup       bool `json:"auto_backup"`         // Create backup before install/config changes
	BackupMaxCount   int  `json:"backup_max_count"`    // Max number of backups to keep (0 = unlimited)
	BackupMaxAgeDays int  `json:"backup_max_age_days"` // Delete backups older than this (0 = keep forever)

	// unknownFields retains additive keys this binary does not yet model. This
	// lets a current-schema config survive a read-modify-write by an older binary
	// without losing settings owned by a newer component. Future schema versions
	// are still rejected before decode; this is preservation, not interpretation.
	unknownFields map[string]json.RawMessage

	// sourceRevision is populated by LoadGlobalConfig and refreshed after a
	// successful SaveGlobalConfig. It is deliberately not serialized. A zero
	// revision identifies a programmatically constructed config, which may create
	// a missing file but may not replace an existing file without first loading it.
	sourceRevision safefile.Revision
}

var globalConfigSaveMu sync.Mutex

// globalConfigAfterWriteHook is used only by package tests to deterministically
// model a non-cooperating process replacing global.json after our rename but
// before post-commit verification.
var globalConfigAfterWriteHook func(path string) error

var globalConfigKnownFields = map[string]struct{}{
	"schema_version":      {},
	"theme":               {},
	"nav_style":           {},
	"active_user":         {},
	"disable_animations":  {},
	"auto_backup":         {},
	"backup_max_count":    {},
	"backup_max_age_days": {},
}

func canonicalGlobalConfigKey(key string) (string, bool) {
	for canonical := range globalConfigKnownFields {
		if strings.EqualFold(key, canonical) {
			return canonical, true
		}
	}
	return "", false
}

// DefaultGlobalConfig returns default global settings
func DefaultGlobalConfig() *GlobalConfig {
	return &GlobalConfig{
		SchemaVersion:    CurrentGlobalConfigSchemaVersion,
		Theme:            "catppuccin-mocha",
		NavStyle:         "emacs",
		AutoBackup:       true, // Auto-backup enabled by default
		BackupMaxCount:   10,   // Keep last 10 backups
		BackupMaxAgeDays: 30,   // Delete backups older than 30 days
	}
}

// ConfigDir returns the dotfiles config directory path
// Returns empty string if HOME is not set and XDG_CONFIG_HOME is not available
func ConfigDir() string {
	// The XDG base-directory specification requires an absolute path. Ignore a
	// relative value instead of letting configuration reads/writes escape through
	// the process working directory.
	if xdg := os.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(xdg) {
		return filepath.Clean(filepath.Join(xdg, "dotfiles"))
	}
	home := os.Getenv("HOME")
	if home == "" {
		// Fallback: try to get home directory from os.UserHomeDir
		var err error
		home, err = os.UserHomeDir()
		if err != nil || home == "" {
			return ""
		}
	}
	return filepath.Clean(filepath.Join(home, ".config", "dotfiles"))
}

// ToolsDir returns the per-tool config directory path
func ToolsDir() string {
	return filepath.Join(ConfigDir(), "tools")
}

// LoadToolConfig loads a tool config from JSON file, returning defaults if not found
func LoadToolConfig[T any](toolName string, defaultFn func() *T) (*T, error) {
	if ConfigDir() == "" {
		return nil, ErrNoConfigDir
	}
	if err := validateToolConfigName(toolName); err != nil {
		return nil, err
	}
	path := filepath.Join(ToolsDir(), toolName+".json")

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		// Return defaults if file doesn't exist
		return defaultFn(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}

	// Start from the intended defaults so keys absent from an older/partial JSON
	// file keep their default value instead of decoding to the Go zero value.
	// json.Unmarshal only overwrites keys that are actually present in the file.
	cfg := defaultFn()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}

	return cfg, nil
}

// SaveToolConfig saves a tool config to JSON file
func SaveToolConfig[T any](toolName string, cfg *T) error {
	if ConfigDir() == "" {
		return ErrNoConfigDir
	}
	if err := validateToolConfigName(toolName); err != nil {
		return err
	}
	path := filepath.Join(ToolsDir(), toolName+".json")

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := writeFileAtomic(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}

	return nil
}

func validateToolConfigName(name string) error {
	if name == "" {
		return errors.New("tool config name cannot be empty")
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return fmt.Errorf("invalid tool config name %q", name)
	}
	return nil
}

// readGlobalConfigRevision opens and reads path through one descriptor so the
// bytes and file identity belong to the same source even if another process
// replaces the pathname concurrently.
func readGlobalConfigRevision(root, rel string) ([]byte, safefile.Revision, error) {
	return safefile.ReadWithin(root, rel)
}

// decodeJSONObject rejects duplicate keys instead of accepting encoding/json's
// last-key-wins behavior. The latter is unsafe for schema metadata because two
// textual schema_version entries can otherwise describe one ambiguous file.
func decodeJSONObject(data []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	start, ok := token.(json.Delim)
	if !ok || start != '{' {
		return nil, fmt.Errorf("expected JSON object")
	}

	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyToken.(string)
		if !ok {
			return nil, fmt.Errorf("expected object key")
		}
		if _, duplicate := fields[key]; duplicate {
			return nil, fmt.Errorf("duplicate global config key %q", key)
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		fields[key] = value
	}
	end, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if delim, ok := end.(json.Delim); !ok || delim != '}' {
		return nil, fmt.Errorf("expected end of JSON object")
	}
	if token, err := decoder.Token(); err != io.EOF {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("unexpected trailing JSON token %v", token)
	}
	return fields, nil
}

func validateGlobalConfigFields(fields map[string]json.RawMessage) error {
	for key, value := range fields {
		if canonical, collision := canonicalGlobalConfigKey(key); collision && key != canonical {
			return fmt.Errorf("noncanonical global config key %q; use %q", key, canonical)
		}
		if _, known := globalConfigKnownFields[key]; known && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("global config key %q must not be null", key)
		}
	}
	return nil
}

func validateGlobalConfig(cfg *GlobalConfig) error {
	if !IsValidTheme(cfg.Theme) {
		return fmt.Errorf("invalid global config theme %q", cfg.Theme)
	}
	if !IsValidNavStyle(cfg.NavStyle) {
		return fmt.Errorf("invalid global config nav_style %q", cfg.NavStyle)
	}
	if cfg.ActiveUser != "" {
		if err := ValidateUsername(cfg.ActiveUser); err != nil {
			return fmt.Errorf("invalid global config active_user: %w", err)
		}
	}
	if cfg.BackupMaxCount < 0 {
		return fmt.Errorf("invalid global config backup_max_count %d: must be nonnegative", cfg.BackupMaxCount)
	}
	if cfg.BackupMaxAgeDays < 0 {
		return fmt.Errorf("invalid global config backup_max_age_days %d: must be nonnegative", cfg.BackupMaxAgeDays)
	}
	return nil
}

// LoadGlobalConfig loads global config from settings file. The returned value
// carries an opaque source revision used by SaveGlobalConfig for conflict-safe
// read-modify-write. A missing file is also tracked, so a file created after
// this load cannot be overwritten by the stale default value.
func LoadGlobalConfig() (*GlobalConfig, error) {
	if ConfigDir() == "" {
		return nil, ErrNoConfigDir
	}
	path := filepath.Join(ConfigDir(), "global.json")
	root, rel, err := anchoredFilePath(path)
	if err != nil {
		return nil, fmt.Errorf("resolve global config path: %w", err)
	}

	data, revision, err := readGlobalConfigRevision(root, rel)
	if err != nil {
		return nil, fmt.Errorf("failed to read global config: %w", err)
	}
	if !revision.Exists() {
		cfg := DefaultGlobalConfig()
		cfg.sourceRevision = revision
		return cfg, nil
	}

	// Inspect the on-disk schema independently from the default-filled target.
	// Otherwise an absent legacy schema_version would be indistinguishable from
	// the current version supplied by DefaultGlobalConfig.
	fields, err := decodeJSONObject(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse global config: %w", err)
	}
	if err := validateGlobalConfigFields(fields); err != nil {
		return nil, fmt.Errorf("failed to parse global config: %w", err)
	}

	onDiskVersion := 0
	if rawVersion, ok := fields["schema_version"]; ok {
		var parsedVersion *int
		if err := json.Unmarshal(rawVersion, &parsedVersion); err != nil {
			return nil, fmt.Errorf("failed to parse global config schema_version: %w", err)
		}
		if parsedVersion == nil {
			return nil, fmt.Errorf("failed to parse global config schema_version: value must be a non-null integer")
		}
		onDiskVersion = *parsedVersion
	}
	if onDiskVersion < 0 || onDiskVersion > CurrentGlobalConfigSchemaVersion {
		return nil, fmt.Errorf("unsupported global config schema_version %d (supported: 0-%d)", onDiskVersion, CurrentGlobalConfigSchemaVersion)
	}

	// Schema 0 migration and forward-compatible additions use the same
	// presence-aware operation: defaults first, then overlay keys that exist in
	// JSON. json.Unmarshal overwrites explicit false and zero values, but leaves
	// defaults in place for fields absent from older/partial files.
	cfg := DefaultGlobalConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse global config: %w", err)
	}
	cfg.SchemaVersion = CurrentGlobalConfigSchemaVersion
	if err := validateGlobalConfig(cfg); err != nil {
		return nil, fmt.Errorf("failed to parse global config: %w", err)
	}
	for key, value := range fields {
		if _, known := globalConfigKnownFields[key]; known {
			continue
		}
		if cfg.unknownFields == nil {
			cfg.unknownFields = make(map[string]json.RawMessage)
		}
		cfg.unknownFields[key] = append(json.RawMessage(nil), value...)
	}
	cfg.sourceRevision = revision

	return cfg, nil
}

func normalizeGlobalConfigForSave(cfg *GlobalConfig) (GlobalConfig, error) {
	if cfg == nil {
		return GlobalConfig{}, errors.New("failed to marshal global config: config is nil")
	}

	normalized := *cfg
	if normalized.SchemaVersion == 0 {
		// Backward compatibility for old in-memory struct literals: required enum
		// fields that are empty are treated like omitted legacy JSON and receive
		// defaults. Boolean and numeric zero values remain explicit because a Go
		// value has no JSON-key-presence information with which to infer omission.
		defaults := DefaultGlobalConfig()
		if normalized.Theme == "" {
			normalized.Theme = defaults.Theme
		}
		if normalized.NavStyle == "" {
			normalized.NavStyle = defaults.NavStyle
		}
		normalized.SchemaVersion = CurrentGlobalConfigSchemaVersion
	}
	if normalized.SchemaVersion != CurrentGlobalConfigSchemaVersion {
		return GlobalConfig{}, fmt.Errorf("unsupported global config schema_version %d (supported: %d)", normalized.SchemaVersion, CurrentGlobalConfigSchemaVersion)
	}
	if err := validateGlobalConfig(&normalized); err != nil {
		return GlobalConfig{}, err
	}
	return normalized, nil
}

func marshalGlobalConfig(cfg *GlobalConfig) ([]byte, error) {
	knownData, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal global config: %w", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(knownData, &fields); err != nil {
		return nil, fmt.Errorf("failed to marshal global config: %w", err)
	}
	for key, value := range cfg.unknownFields {
		if _, known := globalConfigKnownFields[key]; known {
			continue
		}
		if canonical, collision := canonicalGlobalConfigKey(key); collision && key != canonical {
			return nil, fmt.Errorf("failed to marshal global config: noncanonical key %q collides with %q", key, canonical)
		}
		fields[key] = value
	}
	data, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal global config: %w", err)
	}
	return data, nil
}

func revisionsMatch(expected, actual safefile.Revision) bool {
	return expected.Tracked() && actual.Tracked() && expected == actual
}

func checkGlobalConfigRevision(root, rel string, expected safefile.Revision) error {
	_, actual, err := readGlobalConfigRevision(root, rel)
	if err != nil {
		return fmt.Errorf("read current global config revision: %w", err)
	}
	if !expected.Tracked() {
		if actual.Exists() {
			return fmt.Errorf("%w: programmatic config must load existing global.json before replacing it", ErrGlobalConfigConflict)
		}
		return nil
	}
	if !revisionsMatch(expected, actual) {
		return fmt.Errorf("%w: on-disk revision no longer matches source", ErrGlobalConfigConflict)
	}
	return nil
}

// SaveGlobalConfig saves global config to settings file. Loaded configs use a
// hash + file-identity compare-and-save while all cooperating writers hold an
// inter-process lock. A programmatically constructed config remains compatible
// for first-file creation, but must LoadGlobalConfig before replacing a file.
func SaveGlobalConfig(cfg *GlobalConfig) error {
	return SaveGlobalConfigWithReservedRevision(cfg, nil)
}

// SaveGlobalConfigWithReservedRevision saves cfg after reserving its source
// revision under both the in-process and inter-process locks. beforeCommit, when
// non-nil, runs only after the revision is proven current and before global.json
// is replaced. This lets callers persist dependent state without mutating it
// first and only then discovering that their global config was stale.
//
// A beforeCommit failure leaves global.json unchanged. A later global.json
// write failure cannot roll back side effects performed by beforeCommit, so the
// callback should remain small and independently retryable.
func SaveGlobalConfigWithReservedRevision(cfg *GlobalConfig, beforeCommit func() error) (returnErr error) {
	// SaveGlobalConfig refreshes cfg.sourceRevision after a successful write.
	// Lock before reading any cfg field so concurrent saves of the same pointer do
	// not race with that refresh. This also protects unknownFields map reads made
	// while marshaling against another SaveGlobalConfig call on the same pointer.
	globalConfigSaveMu.Lock()
	defer globalConfigSaveMu.Unlock()

	normalized, err := normalizeGlobalConfigForSave(cfg)
	if err != nil {
		return err
	}
	data, err := marshalGlobalConfig(&normalized)
	if err != nil {
		return err
	}

	dir := ConfigDir()
	if dir == "" {
		return ErrNoConfigDir
	}
	path := filepath.Join(dir, "global.json")
	if err := ensureConfiguredXDGRoot(path); err != nil {
		return fmt.Errorf("prepare global config path: %w", err)
	}
	root, rel, err := anchoredFilePath(path)
	if err != nil {
		return fmt.Errorf("resolve global config path: %w", err)
	}
	lockRoot := root
	lockRel := globalConfigLockRel(rel)

	release, err := acquireGlobalConfigLock(lockRoot, lockRel)
	if err != nil {
		return fmt.Errorf("lock global config: %w", err)
	}
	didCommit := false
	defer func() {
		if err := release(); err != nil {
			releaseErr := fmt.Errorf("release global config lock: %w", err)
			if didCommit {
				returnErr = &GlobalConfigCommittedError{Operation: "release global config lock", Err: errors.Join(returnErr, releaseErr)}
				return
			}
			returnErr = errors.Join(returnErr, releaseErr)
		}
	}()

	if err := checkGlobalConfigRevision(root, rel, cfg.sourceRevision); err != nil {
		return err
	}
	if beforeCommit != nil {
		if err := beforeCommit(); err != nil {
			return fmt.Errorf("global config reserved callback failed: %w", err)
		}
		// The callback may be independently retryable, but it can be long enough
		// for a non-cooperating writer to replace global.json while our advisory
		// lock is held. Refuse to overwrite that edit. A portable optimistic CAS
		// still cannot make this final check and rename one indivisible operation.
		if err := checkGlobalConfigRevision(root, rel, cfg.sourceRevision); err != nil {
			return err
		}
	}

	if err := safefile.ReplaceWithin(root, rel, data, 0600); err != nil {
		var committed interface{ Committed() bool }
		if errors.As(err, &committed) && committed.Committed() {
			didCommit = true
			return &GlobalConfigCommittedError{Operation: "replace global.json", Err: err}
		}
		return fmt.Errorf("failed to write global config: %w", err)
	}
	didCommit = true
	if globalConfigAfterWriteHook != nil {
		if err := globalConfigAfterWriteHook(path); err != nil {
			return &GlobalConfigCommittedError{Operation: "post-write test hook", Err: err}
		}
	}
	committedData, revision, err := readGlobalConfigRevision(root, rel)
	if err != nil {
		return &GlobalConfigCommittedError{Operation: "read committed revision", Err: errors.Join(ErrGlobalConfigConflict, err)}
	}
	if !revision.Exists() || !bytes.Equal(committedData, data) || revision.Permissions() != 0600 {
		return &GlobalConfigCommittedError{
			Operation: "verify committed revision",
			Err:       fmt.Errorf("%w: global.json no longer contains the intended bytes and mode", ErrGlobalConfigConflict),
		}
	}
	cfg.sourceRevision = revision

	return nil
}

// AvailableThemes returns the list of available themes
var AvailableThemes = []string{
	"catppuccin-mocha",
	"catppuccin-latte",
	"catppuccin-frappe",
	"catppuccin-macchiato",
	"dracula",
	"gruvbox-dark",
	"gruvbox-light",
	"nord",
	"tokyo-night",
	"solarized-dark",
	"solarized-light",
	"monokai",
	"rose-pine",
	"everforest",
	"one-dark",
	"neon-seapunk",
}

// IsValidTheme checks if a theme name is valid
func IsValidTheme(theme string) bool {
	for _, t := range AvailableThemes {
		if t == theme {
			return true
		}
	}
	return false
}
