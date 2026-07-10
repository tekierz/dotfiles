package main

import (
	"context"
	"debug/buildinfo"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	doctorSchemaVersion = 1
	doctorFormula       = "tekierz/tap/dotfiles"
)

type doctorReport struct {
	SchemaVersion     int                `json:"schema_version"`
	RunningExecutable doctorExecutable   `json:"running_executable"`
	PATHMatches       []doctorExecutable `json:"path_matches"`
	Homebrew          doctorHomebrew     `json:"homebrew"`
	LegacyBinaries    []doctorExecutable `json:"legacy_binaries"`
	Findings          []doctorFinding    `json:"findings"`
}

type doctorExecutable struct {
	Name            string `json:"name"`
	Path            string `json:"path"`
	ResolvedPath    string `json:"resolved_path,omitempty"`
	SymlinkTarget   string `json:"symlink_target,omitempty"`
	PATHIndex       int    `json:"path_index,omitempty"`
	SelectedByPATH  bool   `json:"selected_by_path,omitempty"`
	SameAsRunning   bool   `json:"same_as_running,omitempty"`
	Executable      bool   `json:"executable"`
	Mode            string `json:"mode,omitempty"`
	VersionHint     string `json:"version_hint"`
	VersionSource   string `json:"version_source"`
	ModulePath      string `json:"module_path,omitempty"`
	ModuleVersion   string `json:"module_version,omitempty"`
	GoVersion       string `json:"go_version,omitempty"`
	VCSRevision     string `json:"vcs_revision,omitempty"`
	VCSModified     bool   `json:"vcs_modified,omitempty"`
	OwnershipHint   string `json:"ownership_hint"`
	InspectionError string `json:"inspection_error,omitempty"`
}

type doctorHomebrew struct {
	Formula            string `json:"formula"`
	BrewExecutable     string `json:"brew_executable,omitempty"`
	Installed          bool   `json:"installed"`
	Prefix             string `json:"prefix,omitempty"`
	ManagedExecutable  string `json:"managed_executable,omitempty"`
	ResolvedExecutable string `json:"resolved_executable,omitempty"`
	VersionHint        string `json:"version_hint,omitempty"`
	ProbeError         string `json:"probe_error,omitempty"`
}

type doctorFinding struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Summary  string `json:"summary"`
	Path     string `json:"path,omitempty"`
}

type doctorDependencies struct {
	executable        func() (string, error)
	userHomeDir       func() (string, error)
	getenv            func(string) string
	homebrewProbe     func(context.Context, string) (brewPath, prefix string, err error)
	legacyDirectories []string
}

func systemDoctorDependencies() doctorDependencies {
	return doctorDependencies{
		executable:        os.Executable,
		userHomeDir:       os.UserHomeDir,
		getenv:            os.Getenv,
		homebrewProbe:     probeHomebrewPrefix,
		legacyDirectories: []string{"/usr/local/bin", "/opt/homebrew/bin"},
	}
}

func newDoctorCommand(collect func(context.Context) (doctorReport, error)) *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose executable provenance and PATH collisions",
		Long: `Inspect the running dotfiles executable, every executable named dotfiles on
PATH, Homebrew formula ownership, static version/build hints, and stale legacy
dotfiles-tui or dotfiles-setup binaries.

Doctor is read-only: it does not execute discovered dotfiles binaries and never
deletes or modifies files. Use --json for deterministic structured output.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			report, err := collect(cmd.Context())
			if err != nil {
				return err
			}
			if jsonOutput {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				return encoder.Encode(report)
			}
			return writeDoctorHuman(cmd.OutOrStdout(), report)
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Print deterministic JSON output")
	return cmd
}

var doctorCmd = newDoctorCommand(func(ctx context.Context) (doctorReport, error) {
	return collectDoctorReport(ctx, systemDoctorDependencies(), version)
})

func collectDoctorReport(ctx context.Context, deps doctorDependencies, buildVersion string) (doctorReport, error) {
	report := doctorReport{
		SchemaVersion:  doctorSchemaVersion,
		PATHMatches:    []doctorExecutable{},
		LegacyBinaries: []doctorExecutable{},
		Findings:       []doctorFinding{},
		Homebrew: doctorHomebrew{
			Formula: doctorFormula,
		},
	}

	runningPath, err := deps.executable()
	if err != nil {
		return report, fmt.Errorf("locate running executable: %w", err)
	}
	runningPath, err = filepath.Abs(runningPath)
	if err != nil {
		return report, fmt.Errorf("normalize running executable: %w", err)
	}
	runningPath = filepath.Clean(runningPath)
	report.RunningExecutable = inspectDoctorExecutable(runningPath, "dotfiles", 0)
	report.RunningExecutable.SameAsRunning = true
	report.RunningExecutable.VersionHint = buildVersion
	report.RunningExecutable.VersionSource = "running build"
	report.RunningExecutable.OwnershipHint = "running executable; ownership not yet verified"

	brewCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	brewPath, brewPrefix, brewErr := deps.homebrewProbe(brewCtx, doctorFormula)
	report.Homebrew.BrewExecutable = brewPath
	if brewErr != nil {
		report.Homebrew.ProbeError = brewErr.Error()
	} else {
		report.Homebrew.Installed = true
		report.Homebrew.Prefix = filepath.Clean(brewPrefix)
		report.Homebrew.ManagedExecutable = filepath.Join(report.Homebrew.Prefix, "bin", "dotfiles")
		if resolved, resolveErr := filepath.EvalSymlinks(report.Homebrew.ManagedExecutable); resolveErr == nil {
			report.Homebrew.ResolvedExecutable = filepath.Clean(resolved)
			report.Homebrew.VersionHint = homebrewVersionHint(report.Homebrew.ResolvedExecutable)
		}
	}
	if report.Homebrew.Installed && sameFile(runningPath, report.Homebrew.ManagedExecutable) {
		report.RunningExecutable.OwnershipHint = "Homebrew formula " + report.Homebrew.Formula
	}

	home := homeDirectory(deps)
	pathDirs := normalizedPATHDirectories(deps.getenv("PATH"))
	for pathIndex, directory := range pathDirs {
		for _, candidate := range executableNameCandidates(directory, "dotfiles", deps.getenv("PATHEXT")) {
			if !isExecutableFile(candidate) {
				continue
			}
			match := inspectDoctorExecutable(candidate, "dotfiles", pathIndex+1)
			match.SelectedByPATH = len(report.PATHMatches) == 0
			match.SameAsRunning = sameFile(candidate, runningPath)
			classifyDoctorOwnership(&match, runningPath, report.Homebrew, home)
			if match.SameAsRunning {
				match.VersionHint = buildVersion
				match.VersionSource = "running build"
			}
			report.PATHMatches = append(report.PATHMatches, match)
			break
		}
	}
	report.PATHMatches = uniqueDoctorExecutables(report.PATHMatches)
	if len(report.PATHMatches) > 0 {
		for i := range report.PATHMatches {
			report.PATHMatches[i].SelectedByPATH = i == 0
		}
	}

	legacyDirs := append([]string(nil), pathDirs...)
	if home != "" {
		legacyDirs = append(legacyDirs, filepath.Join(home, ".local", "bin"))
	}
	legacyDirs = append(legacyDirs, deps.legacyDirectories...)
	legacyDirs = uniqueStrings(legacyDirs)
	for _, directory := range legacyDirs {
		for _, legacyName := range []string{"dotfiles-tui", "dotfiles-setup"} {
			for _, candidate := range executableNameCandidates(directory, legacyName, deps.getenv("PATHEXT")) {
				if !isExecutableFile(candidate) {
					continue
				}
				legacy := inspectDoctorExecutable(candidate, legacyName, 0)
				legacy.SameAsRunning = sameFile(candidate, runningPath)
				legacy.OwnershipHint = "legacy-name candidate; ownership not verified"
				report.LegacyBinaries = append(report.LegacyBinaries, legacy)
				break
			}
		}
	}
	report.LegacyBinaries = uniqueDoctorExecutables(report.LegacyBinaries)
	sort.SliceStable(report.LegacyBinaries, func(i, j int) bool {
		return report.LegacyBinaries[i].Path < report.LegacyBinaries[j].Path
	})

	report.Findings = doctorFindings(report)
	return report, nil
}

func probeHomebrewPrefix(ctx context.Context, formula string) (string, string, error) {
	brewPath, err := exec.LookPath("brew")
	if err != nil {
		return "", "", errors.New("brew was not found on PATH")
	}
	command := exec.CommandContext(ctx, brewPath, "--prefix", formula)
	command.Env = append(os.Environ(), "HOMEBREW_NO_AUTO_UPDATE=1")
	output, err := command.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			detail = err.Error()
		}
		return brewPath, "", fmt.Errorf("brew prefix query failed: %s", detail)
	}
	prefix := strings.TrimSpace(string(output))
	if prefix == "" || !filepath.IsAbs(prefix) {
		return brewPath, "", fmt.Errorf("brew returned invalid formula prefix %q", prefix)
	}
	return brewPath, prefix, nil
}

func normalizedPATHDirectories(pathEnv string) []string {
	parts := filepath.SplitList(pathEnv)
	directories := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			part = "."
		}
		absolute, err := filepath.Abs(part)
		if err != nil {
			continue
		}
		directories = append(directories, filepath.Clean(absolute))
	}
	return uniqueStrings(directories)
}

func executableNameCandidates(directory, name, pathExt string) []string {
	if runtime.GOOS != "windows" || filepath.Ext(name) != "" {
		return []string{filepath.Join(directory, name)}
	}
	extensions := filepath.SplitList(pathExt)
	if len(extensions) == 0 {
		extensions = []string{".exe", ".com", ".bat", ".cmd"}
	}
	candidates := make([]string, 0, len(extensions)+1)
	for _, extension := range extensions {
		candidates = append(candidates, filepath.Join(directory, name+strings.ToLower(extension)))
	}
	candidates = append(candidates, filepath.Join(directory, name))
	return candidates
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	return runtime.GOOS == "windows" || info.Mode().Perm()&0o111 != 0
}

func inspectDoctorExecutable(path, name string, pathIndex int) doctorExecutable {
	path = filepath.Clean(path)
	result := doctorExecutable{
		Name:          name,
		Path:          path,
		PATHIndex:     pathIndex,
		VersionHint:   "unknown (candidate was not executed)",
		VersionSource: "unavailable",
		OwnershipHint: "ownership not verified",
	}

	lstat, err := os.Lstat(path)
	if err != nil {
		result.InspectionError = err.Error()
		return result
	}
	if lstat.Mode()&os.ModeSymlink != 0 {
		if target, readErr := os.Readlink(path); readErr == nil {
			result.SymlinkTarget = target
		}
	}
	if resolved, resolveErr := filepath.EvalSymlinks(path); resolveErr == nil {
		result.ResolvedPath = filepath.Clean(resolved)
	}
	info, err := os.Stat(path)
	if err != nil {
		result.InspectionError = err.Error()
		return result
	}
	result.Mode = info.Mode().String()
	result.Executable = info.Mode().IsRegular() && (runtime.GOOS == "windows" || info.Mode().Perm()&0o111 != 0)

	metadataPath := result.ResolvedPath
	if metadataPath == "" {
		metadataPath = path
	}
	if build, buildErr := buildinfo.ReadFile(metadataPath); buildErr == nil {
		result.ModulePath = build.Path
		result.ModuleVersion = build.Main.Version
		result.GoVersion = build.GoVersion
		for _, setting := range build.Settings {
			switch setting.Key {
			case "vcs.revision":
				result.VCSRevision = setting.Value
			case "vcs.modified":
				result.VCSModified = setting.Value == "true"
			}
		}
		switch {
		case build.Main.Version != "" && build.Main.Version != "(devel)":
			result.VersionHint = build.Main.Version
			result.VersionSource = "Go module metadata"
		case result.VCSRevision != "":
			result.VersionHint = shortRevision(result.VCSRevision)
			if result.VCSModified {
				result.VersionHint += "-dirty"
			}
			result.VersionSource = "Go VCS metadata"
		default:
			result.VersionHint = "development Go build"
			result.VersionSource = "Go build metadata"
		}
		return result
	}

	if scriptVersion, scriptErr := readStaticScriptVersion(metadataPath); scriptErr == nil && scriptVersion != "" {
		result.VersionHint = scriptVersion
		result.VersionSource = "static script VERSION assignment"
	}
	return result
}

func readStaticScriptVersion(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, 64*1024))
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "readonly ")
		if !strings.HasPrefix(line, "VERSION=") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, "VERSION="))
		if len(value) >= 2 && ((value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"')) {
			value = value[1 : len(value)-1]
		}
		if value != "" && !strings.ContainsAny(value, "$`\\") {
			return value, nil
		}
	}
	return "", nil
}

func classifyDoctorOwnership(item *doctorExecutable, runningPath string, brew doctorHomebrew, home string) {
	switch {
	case brew.Installed && sameFile(item.Path, brew.ManagedExecutable):
		item.OwnershipHint = "Homebrew formula " + brew.Formula
		if sameFile(item.Path, runningPath) {
			item.OwnershipHint += "; same as running executable"
		}
	case brew.Installed && item.ResolvedPath != "" && pathWithin(brew.Prefix, item.ResolvedPath):
		item.OwnershipHint = "inside Homebrew formula prefix " + brew.Formula
	case sameFile(item.Path, runningPath):
		item.OwnershipHint = "running executable; ownership not yet verified"
	case home != "" && pathWithin(filepath.Join(home, ".local", "bin"), item.Path):
		item.OwnershipHint = "user-local path; ownership not verified"
	default:
		item.OwnershipHint = "PATH candidate; ownership not verified"
	}
}

func doctorFindings(report doctorReport) []doctorFinding {
	findings := make([]doctorFinding, 0)
	switch {
	case len(report.PATHMatches) == 0:
		findings = append(findings, doctorFinding{
			Severity: "warning",
			Code:     "path-missing",
			Summary:  "no executable named dotfiles was found on PATH",
		})
	case !report.PATHMatches[0].SameAsRunning:
		findings = append(findings, doctorFinding{
			Severity: "warning",
			Code:     "path-shadowing",
			Summary:  "the first dotfiles on PATH is not the running executable",
			Path:     report.PATHMatches[0].Path,
		})
	}
	if len(report.PATHMatches) > 1 {
		findings = append(findings, doctorFinding{
			Severity: "warning",
			Code:     "multiple-path-matches",
			Summary:  fmt.Sprintf("%d distinct dotfiles executables are reachable through PATH", len(report.PATHMatches)),
		})
	}
	runningOnPATH := false
	for _, match := range report.PATHMatches {
		if match.SameAsRunning {
			runningOnPATH = true
		}
		if !match.SameAsRunning && knownDifferentVersion(match.VersionHint, report.RunningExecutable.VersionHint) {
			findings = append(findings, doctorFinding{
				Severity: "warning",
				Code:     "version-mismatch",
				Summary:  fmt.Sprintf("PATH candidate version %s differs from running version %s", match.VersionHint, report.RunningExecutable.VersionHint),
				Path:     match.Path,
			})
		}
		if !match.SameAsRunning && match.ModulePath != "" && match.ModulePath == report.RunningExecutable.ModulePath && match.GoVersion != "" && report.RunningExecutable.GoVersion != "" && match.GoVersion != report.RunningExecutable.GoVersion {
			findings = append(findings, doctorFinding{
				Severity: "info",
				Code:     "build-toolchain-mismatch",
				Summary:  fmt.Sprintf("PATH candidate uses %s while the running build uses %s", match.GoVersion, report.RunningExecutable.GoVersion),
				Path:     match.Path,
			})
		}
	}
	if !runningOnPATH {
		findings = append(findings, doctorFinding{
			Severity: "info",
			Code:     "running-not-on-path",
			Summary:  "the running executable is not reachable through the current PATH",
			Path:     report.RunningExecutable.Path,
		})
	}
	if report.Homebrew.Installed && !sameFile(report.RunningExecutable.Path, report.Homebrew.ManagedExecutable) {
		findings = append(findings, doctorFinding{
			Severity: "warning",
			Code:     "homebrew-not-running",
			Summary:  "Homebrew manages a different dotfiles executable than the one currently running",
			Path:     report.Homebrew.ManagedExecutable,
		})
	}
	for _, legacy := range report.LegacyBinaries {
		findings = append(findings, doctorFinding{
			Severity: "warning",
			Code:     "legacy-binary",
			Summary:  "stale legacy executable name is still present; review ownership manually",
			Path:     legacy.Path,
		})
	}
	if report.Homebrew.ProbeError != "" {
		findings = append(findings, doctorFinding{
			Severity: "info",
			Code:     "homebrew-unavailable",
			Summary:  report.Homebrew.ProbeError,
		})
	}
	return findings
}

func writeDoctorHuman(writer io.Writer, report doctorReport) error {
	if _, err := fmt.Fprintln(writer, "Dotfiles Doctor"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(writer, "==============="); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "Running executable: %s\n", report.RunningExecutable.Path); err != nil {
		return err
	}
	if report.RunningExecutable.ResolvedPath != "" && report.RunningExecutable.ResolvedPath != report.RunningExecutable.Path {
		if _, err := fmt.Fprintf(writer, "Resolved executable: %s\n", report.RunningExecutable.ResolvedPath); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(writer, "Running version: %s (%s)\n\n", report.RunningExecutable.VersionHint, report.RunningExecutable.VersionSource); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(writer, "PATH matches:"); err != nil {
		return err
	}
	if len(report.PATHMatches) == 0 {
		if _, err := fmt.Fprintln(writer, "  none"); err != nil {
			return err
		}
	}
	for _, match := range report.PATHMatches {
		selected := ""
		if match.SelectedByPATH {
			selected = " [selected by PATH]"
		}
		if _, err := fmt.Fprintf(writer, "  %d. %s%s\n     version: %s (%s)\n     ownership: %s\n", match.PATHIndex, match.Path, selected, match.VersionHint, match.VersionSource, match.OwnershipHint); err != nil {
			return err
		}
		if match.ResolvedPath != "" && match.ResolvedPath != match.Path {
			if _, err := fmt.Fprintf(writer, "     resolves to: %s\n", match.ResolvedPath); err != nil {
				return err
			}
		}
		if match.ModulePath != "" {
			if _, err := fmt.Fprintf(writer, "     build: %s %s; %s\n", match.ModulePath, match.ModuleVersion, match.GoVersion); err != nil {
				return err
			}
		}
	}

	if _, err := fmt.Fprintln(writer, "\nHomebrew:"); err != nil {
		return err
	}
	if report.Homebrew.Installed {
		if _, err := fmt.Fprintf(writer, "  formula: %s\n  managed executable: %s\n", report.Homebrew.Formula, report.Homebrew.ManagedExecutable); err != nil {
			return err
		}
		if report.Homebrew.VersionHint != "" {
			if _, err := fmt.Fprintf(writer, "  version hint: %s (from resolved Cellar path)\n", report.Homebrew.VersionHint); err != nil {
				return err
			}
		}
	} else if _, err := fmt.Fprintf(writer, "  unavailable: %s\n", report.Homebrew.ProbeError); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(writer, "\nLegacy executable candidates:"); err != nil {
		return err
	}
	if len(report.LegacyBinaries) == 0 {
		if _, err := fmt.Fprintln(writer, "  none"); err != nil {
			return err
		}
	}
	for _, legacy := range report.LegacyBinaries {
		if _, err := fmt.Fprintf(writer, "  %s (%s; %s)\n", legacy.Path, legacy.VersionHint, legacy.OwnershipHint); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintln(writer, "\nFindings:"); err != nil {
		return err
	}
	if len(report.Findings) == 0 {
		if _, err := fmt.Fprintln(writer, "  OK: no PATH collisions or stale legacy executable names were detected"); err != nil {
			return err
		}
	}
	for _, finding := range report.Findings {
		path := ""
		if finding.Path != "" {
			path = " (" + finding.Path + ")"
		}
		if _, err := fmt.Fprintf(writer, "  %s %s: %s%s\n", strings.ToUpper(finding.Severity), finding.Code, finding.Summary, path); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(writer, "\nRead-only check complete; no files were changed.")
	return err
}

func sameFile(first, second string) bool {
	if first == "" || second == "" {
		return false
	}
	firstInfo, firstErr := os.Stat(first)
	secondInfo, secondErr := os.Stat(second)
	return firstErr == nil && secondErr == nil && os.SameFile(firstInfo, secondInfo)
}

func pathWithin(root, candidate string) bool {
	if root == "" || candidate == "" {
		return false
	}
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator)) && !filepath.IsAbs(relative)
}

func homeDirectory(deps doctorDependencies) string {
	home, err := deps.userHomeDir()
	if err != nil || !filepath.IsAbs(home) {
		return ""
	}
	return filepath.Clean(home)
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = filepath.Clean(value)
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func uniqueDoctorExecutables(values []doctorExecutable) []doctorExecutable {
	seen := make(map[string]struct{}, len(values))
	result := make([]doctorExecutable, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value.Path]; exists {
			continue
		}
		seen[value.Path] = struct{}{}
		result = append(result, value)
	}
	return result
}

func shortRevision(revision string) string {
	if len(revision) <= 12 {
		return revision
	}
	return revision[:12]
}

func knownDifferentVersion(candidate, running string) bool {
	if !knownVersion(candidate) || !knownVersion(running) {
		return false
	}
	return candidate != running
}

func knownVersion(value string) bool {
	value = strings.TrimSpace(strings.ToLower(value))
	return value != "" && value != "dev" && value != "(devel)" && value != "development go build" && !strings.HasPrefix(value, "unknown ")
}

func homebrewVersionHint(resolvedExecutable string) string {
	parts := strings.Split(filepath.ToSlash(filepath.Clean(resolvedExecutable)), "/")
	for index := 0; index+2 < len(parts); index++ {
		if strings.EqualFold(parts[index], "Cellar") && strings.EqualFold(parts[index+1], "dotfiles") {
			return parts[index+2]
		}
	}
	return ""
}
