package pkg

import (
	"strings"
	"testing"
)

// BenchmarkParseDpkgOutput exercises the production receipt parser.
func BenchmarkParseDpkgOutput(b *testing.B) {
	output := "curl\tinstall ok installed\t1.0\nlibc6:arm64\thold ok installed\t2.40\nbroken\tinstall ok unpacked\t1\n"
	for i := 0; i < b.N; i++ {
		_ = parseAptInstalledReceipts(output)
	}
}

func BenchmarkListInstalledMock(b *testing.B) {
	output := strings.Repeat("package-name\tinstall ok installed\t1.0\n", 1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = parseAptInstalledReceipts(output)
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
