package pkg

import "testing"

func TestUpdateProviderDiscoveryAndResolution(t *testing.T) {
	original := allManagers
	t.Cleanup(func() { allManagers = original })
	manager := func(name string, records ...Package) *MockPackageManager {
		m := NewMockPackageManager()
		m.ManagerName = name
		m.OutdatedPkgs = records
		return m
	}
	record := Package{Name: "shared", InstalledBy: "same-display"}
	brew := manager("brew", record, record)
	apt := manager("apt", record)
	paru := manager("paru", Package{Name: "official", InstalledBy: "pacman"}, Package{Name: "foreign", InstalledBy: "aur"})
	allManagers = func() []PackageManager { return []PackageManager{brew, apt, paru} }
	updates, err := CheckAllUpdates()
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) != 4 {
		t.Fatalf("dedup erased distinct execution authority: %+v", updates)
	}
	counts := map[ExecutionProvider]int{}
	for _, p := range updates {
		counts[p.ExecutionProvider()]++
		if ManagerForExecutionProvider(p.ExecutionProvider()) == nil {
			t.Fatalf("unresolved discovery: %+v", p)
		}
	}
	if counts[ExecutionProviderBrew] != 1 || counts[ExecutionProviderAPT] != 1 || counts[ExecutionProviderParu] != 2 {
		t.Fatalf("providers: %v", counts)
	}
	for _, p := range updates {
		if p.Name == "foreign" && (p.InstalledBy != "aur" || p.ExecutionProvider() != ExecutionProviderParu) {
			t.Fatalf("AUR routing: %+v", p)
		}
	}
	if ManagerForExecutionProvider(ExecutionProviderPacman) != nil || ManagerForExecutionProvider("") != nil {
		t.Fatal("missing authority resolved")
	}
	allManagers = func() []PackageManager { return []PackageManager{brew, manager("brew")} }
	if ManagerForExecutionProvider(ExecutionProviderBrew) != nil {
		t.Fatal("ambiguous provider resolved")
	}
}
