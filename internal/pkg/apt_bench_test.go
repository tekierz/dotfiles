package pkg

import (
	"strings"
	"testing"
)

// Mock dpkg output for benchmarking without requiring actual dpkg
const mockDpkgOutput = `accountsservice	install
acl	install
adduser	install
apt	install
base-files	install
bash	install
curl	install
git	install
neovim	install
tmux	install
vim	install
zsh	install`

// BenchmarkParseDpkgOutput benchmarks parsing dpkg --get-selections output
func BenchmarkParseDpkgOutput(b *testing.B) {
	output := mockDpkgOutput

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lines := strings.Split(strings.TrimSpace(output), "\n")
		var packages []Package
		for _, line := range lines {
			parts := strings.Fields(line)
			if len(parts) >= 2 && parts[1] == "install" {
				packages = append(packages, Package{
					Name:        parts[0],
					InstalledBy: "apt",
				})
			}
		}
		_ = packages
	}
}

// BenchmarkListInstalledMock benchmarks parsing with 1000 packages
func BenchmarkListInstalledMock(b *testing.B) {
	// Create mock output with 1000 packages
	var builder strings.Builder
	for i := 0; i < 1000; i++ {
		builder.WriteString("package-")
		builder.WriteString(strings.Repeat("x", 20))
		builder.WriteString("\tinstall\n")
	}
	output := builder.String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lines := strings.Split(strings.TrimSpace(output), "\n")
		packages := make([]Package, 0, len(lines))
		for _, line := range lines {
			parts := strings.Fields(line)
			if len(parts) >= 2 && parts[1] == "install" {
				packages = append(packages, Package{
					Name:        parts[0],
					InstalledBy: "apt",
				})
			}
		}
		_ = packages
	}
}

// BenchmarkParseUpgradableOutput benchmarks parsing apt list --upgradable output
func BenchmarkParseUpgradableOutput(b *testing.B) {
	var builder strings.Builder
	builder.WriteString("Listing... Up to date\n")
	for i := 0; i < 100; i++ {
		builder.WriteString("package-name/jammy-updates 2.0.0 amd64 [upgradable from: 1.0.0]\n")
	}
	output := builder.String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var packages []Package
		lines := strings.Split(output, "\n")
		for _, line := range lines {
			if line == "" || strings.HasPrefix(line, "Listing...") {
				continue
			}

			parts := strings.Fields(line)
			if len(parts) >= 4 {
				nameParts := strings.Split(parts[0], "/")
				name := nameParts[0]
				newVersion := parts[1]

				oldVersion := ""
				for j, p := range parts {
					if p == "from:" && j+1 < len(parts) {
						oldVersion = strings.TrimSuffix(parts[j+1], "]")
					}
				}

				packages = append(packages, Package{
					Name:           name,
					CurrentVersion: oldVersion,
					LatestVersion:  newVersion,
					Outdated:       true,
					InstalledBy:    "apt",
				})
			}
		}
		_ = packages
	}
}

// BenchmarkPackageAllocation benchmarks Package struct allocation
func BenchmarkPackageAllocation(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		packages := make([]Package, 0, 100)
		for j := 0; j < 100; j++ {
			packages = append(packages, Package{
				Name:           "test-package",
				CurrentVersion: "1.0.0",
				LatestVersion:  "2.0.0",
				Outdated:       true,
				InstalledBy:    "apt",
				Description:    "A test package",
			})
		}
		_ = packages
	}
}

// TestAptManager_Name tests the Name method
func TestAptManager_Name(t *testing.T) {
	mgr := &AptManager{}
	if mgr.Name() != "apt" {
		t.Errorf("Name() = %q, want %q", mgr.Name(), "apt")
	}
}

// TestAptManager_NeedsSudo tests sudo requirement
func TestAptManager_NeedsSudo(t *testing.T) {
	mgr := &AptManager{}
	if !mgr.NeedsSudo() {
		t.Error("NeedsSudo() should be true for apt")
	}
}
