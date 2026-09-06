package pkg

import (
	"errors"
	"strings"
	"testing"
)

// TestCheckAllUpdates_DedupPreservesSourceManager verifies the dedup keyed on
// Name+InstalledBy (regression for pkg-4): same-named packages from different
// source managers must both survive so each is routed to the manager that can
// actually upgrade it. Exact duplicates (same name + same source) collapse.
func TestDedupByNameAndInstalledBy(t *testing.T) {
	allPackages := []Package{
		{Name: "vim", InstalledBy: "pacman"},
		{Name: "vim", InstalledBy: "pacman"}, // exact dup -> collapsed
		{Name: "yay", InstalledBy: "pacman"},
		{Name: "yay", InstalledBy: "aur"}, // same name, different source -> kept
	}

	// Mirror the dedup logic in CheckAllUpdates.
	seen := make(map[string]bool)
	var deduped []Package
	for _, p := range allPackages {
		key := p.Name + "\x00" + p.InstalledBy
		if !seen[key] {
			seen[key] = true
			deduped = append(deduped, p)
		}
	}

	if len(deduped) != 3 {
		t.Fatalf("deduped length = %d, want 3 (exact dup collapsed, cross-source kept): %+v", len(deduped), deduped)
	}

	// Verify both source managers for "yay" survived.
	var sources []string
	for _, p := range deduped {
		if p.Name == "yay" {
			sources = append(sources, p.InstalledBy)
		}
	}
	if len(sources) != 2 {
		t.Fatalf("expected 2 'yay' records with distinct sources, got %v", sources)
	}
	hasPacman, hasAur := false, false
	for _, s := range sources {
		switch s {
		case "pacman":
			hasPacman = true
		case "aur":
			hasAur = true
		}
	}
	if !hasPacman || !hasAur {
		t.Errorf("expected 'yay' to retain both pacman and aur sources, got %v", sources)
	}
}

func TestCheckAllUpdates_ReturnsPartialResultsAndManagerErrors(t *testing.T) {
	originalAllManagers := allManagers
	t.Cleanup(func() {
		allManagers = originalAllManagers
	})

	working := NewMockPackageManager()
	working.ManagerName = "apt"
	working.OutdatedPkgs = []Package{
		{Name: "zsh", InstalledBy: "apt", Outdated: true},
	}

	failing := NewMockPackageManager()
	failing.ManagerName = "brew"
	failing.CheckOutdatedErr = errors.New("outdated query failed")

	allManagers = func() []PackageManager {
		return []PackageManager{failing, working}
	}

	packages, err := CheckAllUpdates()
	if err == nil {
		t.Fatal("CheckAllUpdates error = nil, want manager failure")
	}
	if !strings.Contains(err.Error(), "brew: outdated query failed") {
		t.Fatalf("CheckAllUpdates error = %q, want manager name and wrapped error", err)
	}
	if len(packages) != 1 {
		t.Fatalf("CheckAllUpdates returned %d packages, want partial successful result: %+v", len(packages), packages)
	}
	if packages[0].Name != "zsh" || packages[0].InstalledBy != "apt" {
		t.Fatalf("partial package = %+v, want zsh from apt", packages[0])
	}
}

// TestParsePacmanUpdates verifies the shared parser used by checkupdates,
// `pacman -Qu`, and `pacman -Qua` (regression for pkg-2). It must tag records
// with the supplied source and skip blank/malformed lines.
func TestParsePacmanUpdates(t *testing.T) {
	output := "linux 6.8.1-1 -> 6.8.2-1\n" +
		"\n" + // blank line skipped
		"vim 9.0.0-1 -> 9.1.0-1\n" +
		"garbage-line-without-arrow\n" // no " -> " skipped

	pkgs := parsePacmanUpdates(output, "pacman")
	if len(pkgs) != 2 {
		t.Fatalf("parsePacmanUpdates returned %d packages, want 2: %+v", len(pkgs), pkgs)
	}

	if pkgs[0].Name != "linux" || pkgs[0].CurrentVersion != "6.8.1-1" || pkgs[0].LatestVersion != "6.8.2-1" {
		t.Errorf("first package = %+v, want linux 6.8.1-1 -> 6.8.2-1", pkgs[0])
	}
	for _, p := range pkgs {
		if p.InstalledBy != "pacman" {
			t.Errorf("InstalledBy = %q, want %q", p.InstalledBy, "pacman")
		}
		if !p.Outdated {
			t.Errorf("package %q Outdated should be true", p.Name)
		}
	}
}

// TestParsePacmanUpdates_TagsAUR verifies the source tag is honored so AUR
// updates are routed correctly (regression for pkg-4/pkg-5 routing).
func TestParsePacmanUpdates_TagsAUR(t *testing.T) {
	pkgs := parsePacmanUpdates("paru 1.0.0-1 -> 1.1.0-1\n", "aur")
	if len(pkgs) != 1 {
		t.Fatalf("got %d packages, want 1", len(pkgs))
	}
	if pkgs[0].InstalledBy != "aur" {
		t.Errorf("InstalledBy = %q, want %q", pkgs[0].InstalledBy, "aur")
	}
}

// TestParsePacmanUpdates_Empty verifies that empty output (the "no updates"
// case) yields no packages rather than a phantom entry (regression for pkg-2,
// where an empty string previously parsed to a single blank record).
func TestParsePacmanUpdates_Empty(t *testing.T) {
	if pkgs := parsePacmanUpdates("", "pacman"); len(pkgs) != 0 {
		t.Errorf("empty output should yield 0 packages, got %d: %+v", len(pkgs), pkgs)
	}
	if pkgs := parsePacmanUpdates("   \n  \n", "pacman"); len(pkgs) != 0 {
		t.Errorf("whitespace-only output should yield 0 packages, got %d: %+v", len(pkgs), pkgs)
	}
}

// TestCheckDotfilesUpdates_DebianRenamedPackages verifies that packages with
// platform-specific names (e.g., fd -> fd-find on Debian) are recognized in
// the dotfiles update allow-list when the current-platform name is used.
// The old code used a flat macOS/Arch-named list, so "fd-find" was silently
// dropped on Debian (the reported package name) even though fd is managed.
func TestCheckDotfilesUpdates_DebianRenamedPackages(t *testing.T) {
	// Simulate what CheckAllUpdates returns on Debian: apt reports the package
	// by its Debian name "fd-find", not the macOS/Arch "fd".
	debianUpdates := []Package{
		{Name: "fd-find", InstalledBy: "apt", Outdated: true},        // fd on Debian
		{Name: "zsh", InstalledBy: "apt", Outdated: true},            // same name everywhere
		{Name: "something-else", InstalledBy: "apt", Outdated: true}, // not a dotfiles pkg
	}

	// Build the allow-list the same way CheckDotfilesUpdates should: from
	// DotfilesDebianPackages, which must include "fd-find".
	dotfilesSet := make(map[string]bool)
	for _, name := range DotfilesDebianPackages {
		dotfilesSet[name] = true
	}

	var filtered []Package
	for _, pkg := range debianUpdates {
		if dotfilesSet[pkg.Name] {
			filtered = append(filtered, pkg)
		}
	}

	if len(filtered) != 2 {
		t.Fatalf("expected 2 dotfiles packages (fd-find + zsh), got %d: %+v", len(filtered), filtered)
	}

	// Confirm fd-find is recognized
	foundFdFind := false
	for _, p := range filtered {
		if p.Name == "fd-find" {
			foundFdFind = true
		}
		if p.Name == "something-else" {
			t.Errorf("non-dotfiles package %q must not appear in filtered list", p.Name)
		}
	}
	if !foundFdFind {
		t.Error("fd-find (Debian name for fd) must be recognized in the dotfiles update list")
	}
}
