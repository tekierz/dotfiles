package ui

import (
	"testing"

	"github.com/tekierz/dotfiles/internal/pkg"
)

// testZshVersion is a sample installed zsh version reused across cache tests.
const testZshVersion = "5.9"

// mockCaskManager embeds the shared mock and adds cask enumeration so it
// satisfies the caskLister interface used by batchInstalledPackages.
type mockCaskManager struct {
	*pkg.MockPackageManager
	casks   []string
	caskErr error
}

func (m *mockCaskManager) ListInstalledCasks() ([]string, error) {
	if m.caskErr != nil {
		return nil, m.caskErr
	}
	return m.casks, nil
}

func TestBatchInstalledPackages_NilManager(t *testing.T) {
	if got := batchInstalledPackages(nil); got != nil {
		t.Fatalf("expected nil for nil manager, got %v", got)
	}
}

func TestBatchInstalledPackages_FormulaeOnly(t *testing.T) {
	mgr := pkg.NewMockPackageManager()
	mgr.InstalledPkgs["zsh"] = testZshVersion

	got := batchInstalledPackages(mgr)
	if !got["zsh"] {
		t.Errorf("expected formula zsh to be present in batched set")
	}
	if got["sunshine"] {
		t.Errorf("did not expect cask token for non-cask manager")
	}
}

func TestBatchInstalledPackages_MergesCasks(t *testing.T) {
	base := pkg.NewMockPackageManager()
	base.InstalledPkgs["zsh"] = testZshVersion
	mgr := &mockCaskManager{
		MockPackageManager: base,
		casks:              []string{"sunshine", "zen-browser"},
	}

	got := batchInstalledPackages(mgr)
	if !got["zsh"] {
		t.Errorf("expected formula zsh to be present")
	}
	if !got["sunshine"] {
		t.Errorf("expected cask token sunshine to be merged into batched set")
	}
	if !got["zen-browser"] {
		t.Errorf("expected cask token zen-browser to be merged into batched set")
	}
}

func TestBatchInstalledPackages_CaskErrorIgnored(t *testing.T) {
	base := pkg.NewMockPackageManager()
	base.InstalledPkgs["zsh"] = testZshVersion
	mgr := &mockCaskManager{
		MockPackageManager: base,
		caskErr:            errCaskList,
	}

	got := batchInstalledPackages(mgr)
	if !got["zsh"] {
		t.Errorf("formulae should still resolve when cask listing fails")
	}
	if got["sunshine"] {
		t.Errorf("no casks expected when cask listing errors")
	}
}

var errCaskList = &caskListError{}

type caskListError struct{}

func (*caskListError) Error() string { return "cask list failed" }
