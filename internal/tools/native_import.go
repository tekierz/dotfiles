package tools

import (
	"errors"
	"fmt"
	"os"
)

// ConfigValueScope identifies who owns the bytes that supplied an imported
// value. Native values remain user-owned; managed values came from an exact
// dotfiles ownership boundary.
type ConfigValueScope string

const (
	ConfigValueNative  ConfigValueScope = "native"
	ConfigValueManaged ConfigValueScope = "dotfiles-managed"
)

// ConfigFieldProvenance explains exactly where one recognized current value
// came from. Field maps use stable field IDs declared by each importer.
type ConfigFieldProvenance struct {
	Path  string
	Line  int
	Key   string
	Scope ConfigValueScope
}

// ConfigImportSource describes one observed config source. Active is distinct
// from Exists because an orphaned product-generated Git include file may exist
// without being referenced by the user's native .gitconfig.
type ConfigImportSource struct {
	Path    string
	Exists  bool
	Active  bool
	Managed bool
}

func readNativeConfig(path string) ([]byte, bool, error) {
	root, rel, _, err := generatedConfigDestination(path)
	if err != nil {
		return nil, false, err
	}
	if _, err := os.Stat(root); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("inspect config root for %s: %w", path, err)
	}
	content, revision, err := readToolConfig(root, rel)
	if err != nil {
		return nil, false, fmt.Errorf("read native config %s: %w", path, err)
	}
	return content, revision.Exists(), nil
}
