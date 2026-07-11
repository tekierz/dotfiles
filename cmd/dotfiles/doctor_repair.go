package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tekierz/dotfiles/internal/safefile"
)

const (
	doctorRepairSchemaVersion = 1
	dotfilesModulePath        = "github.com/tekierz/dotfiles"
	dotfilesCommandPath       = "github.com/tekierz/dotfiles/cmd/dotfiles"
	doctorRepairCandidateRel  = ".local/bin/dotfiles"
)

var errDoctorRepairStateChanged = errors.New("doctor repair candidate changed after preview")

type doctorRepairPlan struct {
	SchemaVersion     int    `json:"schema_version"`
	Action            string `json:"action"`
	Eligible          bool   `json:"eligible"`
	DecisionCode      string `json:"decision_code"`
	Summary           string `json:"summary"`
	Candidate         string `json:"candidate"`
	CandidateDigest   string `json:"candidate_digest,omitempty"`
	ModulePath        string `json:"module_path,omitempty"`
	CommandPath       string `json:"command_path,omitempty"`
	ModuleVersion     string `json:"module_version,omitempty"`
	RunningExecutable string `json:"running_executable"`
	HomebrewManaged   string `json:"homebrew_managed,omitempty"`
	QuarantinePath    string `json:"quarantine_path,omitempty"`
	ManifestPath      string `json:"manifest_path,omitempty"`
	PlanHash          string `json:"plan_hash"`

	home              string
	candidateRevision safefile.Revision
	candidateBytes    []byte
	quarantineRel     string
	manifestRel       string
}

type doctorRepairResult struct {
	SchemaVersion   int    `json:"schema_version"`
	PlanHash        string `json:"plan_hash"`
	Status          string `json:"status"`
	Candidate       string `json:"candidate"`
	QuarantinePath  string `json:"quarantine_path,omitempty"`
	ManifestPath    string `json:"manifest_path,omitempty"`
	Reversible      bool   `json:"reversible"`
	RestoreGuidance string `json:"restore_guidance,omitempty"`
}

type doctorRepairManifest struct {
	SchemaVersion   int    `json:"schema_version"`
	PlanHash        string `json:"plan_hash"`
	OriginalPath    string `json:"original_path"`
	QuarantinePath  string `json:"quarantine_path"`
	CandidateDigest string `json:"candidate_digest"`
	ModulePath      string `json:"module_path"`
	CommandPath     string `json:"command_path"`
	ModuleVersion   string `json:"module_version,omitempty"`
	OriginalMode    string `json:"original_mode"`
	RestoreGuidance string `json:"restore_guidance"`
}

type doctorRepairDependencies struct {
	executable            func() (string, error)
	userHomeDir           func() (string, error)
	homebrewProbe         func(context.Context, string) (brewPath, prefix string, err error)
	beforeApply           func() error
	beforeQuarantineWrite func() error
	beforeManifestWrite   func() error
	inspectBuild          func([]byte) (commandPath, modulePath, moduleVersion string, err error)
}

func systemDoctorRepairDependencies() doctorRepairDependencies {
	return doctorRepairDependencies{
		executable:    os.Executable,
		userHomeDir:   os.UserHomeDir,
		homebrewProbe: probeHomebrewPrefix,
		inspectBuild:  readDoctorRepairBuild,
	}
}

func newDoctorRepairCommand(deps doctorRepairDependencies) *cobra.Command {
	var jsonOutput bool
	var yes bool
	var acceptedPlanHash string
	cmd := &cobra.Command{
		Use:   "repair",
		Short: "Safely quarantine one proven stale user-local dotfiles binary",
		Long: `Preview and, after explicit confirmation, quarantine only the exact
~/.local/bin/dotfiles PATH entry when static Go build metadata proves it came
from github.com/tekierz/dotfiles and it differs from the running and
Homebrew-managed executables.

Unknown, symlinked, hardlinked, non-regular, current, and unproven files are
never changed. Legacy dotfiles-tui and dotfiles-setup names remain diagnostic
only. The quarantined bytes and a recovery manifest are retained privately.

For deterministic automation, first run with --json, then repeat with --yes
and --plan-hash set to that exact preview hash. --json never prompts.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			plan, err := buildDoctorRepairPlan(cmd.Context(), deps)
			if err != nil {
				return err
			}
			if !yes {
				if jsonOutput {
					return writeDoctorRepairJSON(cmd.OutOrStdout(), plan)
				}
				if err := writeDoctorRepairHuman(cmd.OutOrStdout(), plan); err != nil {
					return err
				}
				if !plan.Eligible {
					_, err = fmt.Fprintln(cmd.OutOrStdout(), "No repair is available; no files were changed.")
					return err
				}
				confirmed, err := confirmDoctorRepair(cmd.InOrStdin(), cmd.OutOrStdout())
				if err != nil {
					return err
				}
				if !confirmed {
					result := cancelledDoctorRepairResult(plan)
					return writeDoctorRepairResultHuman(cmd.OutOrStdout(), result)
				}
			} else {
				if acceptedPlanHash == "" {
					return errors.New("--yes requires --plan-hash from a fresh preview")
				}
				if acceptedPlanHash != plan.PlanHash {
					return fmt.Errorf("plan hash does not match current state; preview again before repair")
				}
				if !plan.Eligible {
					return fmt.Errorf("repair is blocked: %s", plan.DecisionCode)
				}
				if !jsonOutput {
					if err := writeDoctorRepairHuman(cmd.OutOrStdout(), plan); err != nil {
						return err
					}
				}
			}

			result, err := applyDoctorRepairPlan(plan, deps)
			if err != nil {
				return err
			}
			if jsonOutput {
				return writeDoctorRepairJSON(cmd.OutOrStdout(), struct {
					Plan   doctorRepairPlan   `json:"plan"`
					Result doctorRepairResult `json:"result"`
				}{Plan: plan, Result: result})
			}
			return writeDoctorRepairResultHuman(cmd.OutOrStdout(), result)
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Print deterministic structured output; never prompt")
	cmd.Flags().BoolVar(&yes, "yes", false, "Apply noninteractively; requires an exact --plan-hash")
	cmd.Flags().StringVar(&acceptedPlanHash, "plan-hash", "", "Bind noninteractive repair to an exact preview hash")
	return cmd
}

var doctorRepairCmd = newDoctorRepairCommand(systemDoctorRepairDependencies())

func init() {
	doctorCmd.AddCommand(doctorRepairCmd)
}

func buildDoctorRepairPlan(ctx context.Context, deps doctorRepairDependencies) (doctorRepairPlan, error) {
	home, err := deps.userHomeDir()
	if err != nil {
		return doctorRepairPlan{}, fmt.Errorf("determine absolute home directory: %w", err)
	}
	if !filepath.IsAbs(home) {
		return doctorRepairPlan{}, errors.New("determine absolute home directory: path is not absolute")
	}
	home = filepath.Clean(home)
	running, err := deps.executable()
	if err != nil {
		return doctorRepairPlan{}, fmt.Errorf("locate running executable: %w", err)
	}
	running, err = filepath.Abs(running)
	if err != nil {
		return doctorRepairPlan{}, fmt.Errorf("normalize running executable: %w", err)
	}
	running = filepath.Clean(running)
	candidate := filepath.Join(home, filepath.FromSlash(doctorRepairCandidateRel))
	plan := doctorRepairPlan{
		SchemaVersion:     doctorRepairSchemaVersion,
		Action:            "none",
		DecisionCode:      "candidate-missing",
		Summary:           "the historical user-local dotfiles candidate does not exist",
		Candidate:         candidate,
		RunningExecutable: running,
		home:              home,
	}

	data, revision, readErr := safefile.ReadWithin(home, doctorRepairCandidateRel)
	if readErr != nil {
		plan.DecisionCode = "candidate-unsafe"
		plan.Summary = "the candidate could not be observed as one anchored regular file"
		return finalizeDoctorRepairPlan(plan)
	}
	if !revision.Exists() {
		return finalizeDoctorRepairPlan(plan)
	}
	plan.candidateBytes = data
	plan.candidateRevision = revision
	plan.CandidateDigest = digestDoctorRepairBytes(data)
	if revision.Permissions()&0o111 == 0 {
		plan.DecisionCode = "candidate-not-executable"
		plan.Summary = "the candidate is not executable and is outside this repair's scope"
		return finalizeDoctorRepairPlan(plan)
	}
	inspectBuild := deps.inspectBuild
	if inspectBuild == nil {
		inspectBuild = readDoctorRepairBuild
	}
	commandPath, modulePath, moduleVersion, buildErr := inspectBuild(data)
	if buildErr != nil || modulePath != dotfilesModulePath || commandPath != dotfilesCommandPath {
		plan.DecisionCode = "ownership-unproven"
		plan.Summary = "static Go metadata does not prove the official dotfiles command and module paths"
		return finalizeDoctorRepairPlan(plan)
	}
	plan.CommandPath = commandPath
	plan.ModulePath = modulePath
	plan.ModuleVersion = moduleVersion

	if sameFile(candidate, running) {
		plan.DecisionCode = "candidate-is-running"
		plan.Summary = "the candidate is the currently running executable"
		return finalizeDoctorRepairPlan(plan)
	}
	runningDigest, digestErr := digestDoctorRepairFile(running)
	if digestErr != nil {
		plan.DecisionCode = "running-comparison-unavailable"
		plan.Summary = "the running executable could not be compared safely"
		return finalizeDoctorRepairPlan(plan)
	}
	if runningDigest == plan.CandidateDigest {
		plan.DecisionCode = "candidate-matches-running"
		plan.Summary = "the user-local candidate has the same bytes as the running executable"
		return finalizeDoctorRepairPlan(plan)
	}

	brewCtx, cancelBrew := context.WithTimeout(ctx, 3*time.Second)
	_, brewPrefix, brewErr := deps.homebrewProbe(brewCtx, doctorFormula)
	cancelBrew()
	if brewErr == nil {
		brewPrefix = filepath.Clean(brewPrefix)
		managed := filepath.Join(brewPrefix, "bin", "dotfiles")
		plan.HomebrewManaged = managed
		if pathWithin(brewPrefix, candidate) || sameFile(candidate, managed) {
			plan.DecisionCode = "candidate-homebrew-owned"
			plan.Summary = "the candidate is owned by the Homebrew-managed installation"
			return finalizeDoctorRepairPlan(plan)
		}
		managedDigest, managedErr := digestDoctorRepairFile(managed)
		if managedErr != nil {
			plan.DecisionCode = "homebrew-comparison-unavailable"
			plan.Summary = "the Homebrew-managed executable could not be compared safely"
			return finalizeDoctorRepairPlan(plan)
		}
		if managedDigest == plan.CandidateDigest {
			plan.DecisionCode = "candidate-matches-homebrew"
			plan.Summary = "the user-local candidate has the same bytes as the Homebrew-managed executable"
			return finalizeDoctorRepairPlan(plan)
		}
	}
	if revision.LinkCount() != 1 {
		plan.DecisionCode = "candidate-hardlinked"
		plan.Summary = "the candidate inode has another directory entry and ownership is not exclusive"
		return finalizeDoctorRepairPlan(plan)
	}

	digestPrefix := plan.CandidateDigest[:16]
	plan.quarantineRel = filepath.ToSlash(filepath.Join(".local", "state", "dotfiles", "quarantine", "stale-dotfiles-"+digestPrefix))
	plan.manifestRel = plan.quarantineRel + ".json"
	plan.QuarantinePath = filepath.Join(home, filepath.FromSlash(plan.quarantineRel))
	plan.ManifestPath = filepath.Join(home, filepath.FromSlash(plan.manifestRel))
	plan.Action = "quarantine"
	plan.Eligible = true
	plan.DecisionCode = "stale-user-local-candidate"
	plan.Summary = "ownership is proven and the stale user-local candidate can be quarantined reversibly"
	return finalizeDoctorRepairPlan(plan)
}

func finalizeDoctorRepairPlan(plan doctorRepairPlan) (doctorRepairPlan, error) {
	canonical := struct {
		SchemaVersion     int    `json:"schema_version"`
		Action            string `json:"action"`
		Eligible          bool   `json:"eligible"`
		DecisionCode      string `json:"decision_code"`
		Candidate         string `json:"candidate"`
		CandidateDigest   string `json:"candidate_digest,omitempty"`
		ModulePath        string `json:"module_path,omitempty"`
		CommandPath       string `json:"command_path,omitempty"`
		ModuleVersion     string `json:"module_version,omitempty"`
		RunningExecutable string `json:"running_executable"`
		HomebrewManaged   string `json:"homebrew_managed,omitempty"`
		QuarantinePath    string `json:"quarantine_path,omitempty"`
		ManifestPath      string `json:"manifest_path,omitempty"`
	}{
		SchemaVersion: plan.SchemaVersion, Action: plan.Action, Eligible: plan.Eligible,
		DecisionCode: plan.DecisionCode, Candidate: plan.Candidate, CandidateDigest: plan.CandidateDigest,
		ModulePath: plan.ModulePath, CommandPath: plan.CommandPath, ModuleVersion: plan.ModuleVersion, RunningExecutable: plan.RunningExecutable,
		HomebrewManaged: plan.HomebrewManaged, QuarantinePath: plan.QuarantinePath, ManifestPath: plan.ManifestPath,
	}
	data, err := json.Marshal(canonical)
	if err != nil {
		return doctorRepairPlan{}, fmt.Errorf("marshal repair plan: %w", err)
	}
	digest := sha256.Sum256(data)
	plan.PlanHash = hex.EncodeToString(digest[:])
	return plan, nil
}

func applyDoctorRepairPlan(plan doctorRepairPlan, deps doctorRepairDependencies) (doctorRepairResult, error) {
	if !plan.Eligible || plan.Action != "quarantine" || plan.PlanHash == "" {
		return doctorRepairResult{}, errors.New("doctor repair plan is not eligible for execution")
	}
	if deps.beforeApply != nil {
		if err := deps.beforeApply(); err != nil {
			return doctorRepairResult{}, err
		}
	}
	current, revision, err := safefile.ReadWithin(plan.home, doctorRepairCandidateRel)
	if err != nil {
		return doctorRepairResult{}, fmt.Errorf("revalidate repair candidate: %w", err)
	}
	if revision != plan.candidateRevision || !bytes.Equal(current, plan.candidateBytes) {
		return doctorRepairResult{}, errDoctorRepairStateChanged
	}

	quarantineDir := filepath.ToSlash(filepath.Dir(plan.quarantineRel))
	if err := safefile.EnsureDirectoryWithin(plan.home, quarantineDir, 0o700); err != nil {
		return doctorRepairResult{}, fmt.Errorf("create private quarantine: %w", err)
	}
	existing, quarantineRevision, err := safefile.ReadWithin(plan.home, plan.quarantineRel)
	if err != nil {
		return doctorRepairResult{}, fmt.Errorf("inspect quarantine target: %w", err)
	}
	if quarantineRevision.Exists() && !bytes.Equal(existing, plan.candidateBytes) {
		return doctorRepairResult{}, errors.New("quarantine target already exists with different content")
	}
	if deps.beforeQuarantineWrite != nil {
		if err := deps.beforeQuarantineWrite(); err != nil {
			return doctorRepairResult{}, err
		}
	}
	if err := safefile.ReplaceWithinRevision(plan.home, plan.quarantineRel, quarantineRevision, plan.candidateBytes, 0o600); err != nil {
		return doctorRepairResult{}, fmt.Errorf("preserve candidate in quarantine: %w", err)
	}

	manifest := doctorRepairManifest{
		SchemaVersion: doctorRepairSchemaVersion, PlanHash: plan.PlanHash,
		OriginalPath: "~/" + doctorRepairCandidateRel, QuarantinePath: "~/" + plan.quarantineRel,
		CandidateDigest: plan.CandidateDigest, ModulePath: plan.ModulePath, CommandPath: plan.CommandPath, ModuleVersion: plan.ModuleVersion,
		OriginalMode:    fmt.Sprintf("%04o", plan.candidateRevision.Permissions()),
		RestoreGuidance: "Restore only after verifying the original path is absent; copy the preserved binary back and apply original_mode.",
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return doctorRepairResult{}, fmt.Errorf("marshal quarantine manifest: %w", err)
	}
	manifestData = append(manifestData, '\n')
	existing, manifestRevision, err := safefile.ReadWithin(plan.home, plan.manifestRel)
	if err != nil {
		return doctorRepairResult{}, fmt.Errorf("inspect quarantine manifest: %w", err)
	}
	if manifestRevision.Exists() && !bytes.Equal(existing, manifestData) {
		return doctorRepairResult{}, errors.New("quarantine manifest already exists with different content")
	}
	if deps.beforeManifestWrite != nil {
		if err := deps.beforeManifestWrite(); err != nil {
			return doctorRepairResult{}, err
		}
	}
	if err := safefile.ReplaceWithinRevision(plan.home, plan.manifestRel, manifestRevision, manifestData, 0o600); err != nil {
		return doctorRepairResult{}, fmt.Errorf("write quarantine manifest: %w", err)
	}

	current, revision, err = safefile.ReadWithin(plan.home, doctorRepairCandidateRel)
	if err != nil {
		return doctorRepairResult{}, fmt.Errorf("final candidate revalidation: %w", err)
	}
	if revision != plan.candidateRevision || !bytes.Equal(current, plan.candidateBytes) {
		return doctorRepairResult{}, errDoctorRepairStateChanged
	}
	if err := safefile.RemoveWithinRevision(plan.home, doctorRepairCandidateRel, plan.candidateRevision); err != nil {
		return doctorRepairResult{}, fmt.Errorf("quarantine stale PATH entry: %w", err)
	}
	return doctorRepairResult{
		SchemaVersion: doctorRepairSchemaVersion, PlanHash: plan.PlanHash, Status: "quarantined",
		Candidate: plan.Candidate, QuarantinePath: plan.QuarantinePath, ManifestPath: plan.ManifestPath, Reversible: true,
		RestoreGuidance: manifest.RestoreGuidance,
	}, nil
}

func digestDoctorRepairFile(path string) (string, error) {
	// #nosec G304 -- callers pass only the already accepted running executable
	// or fixed Homebrew-managed path; candidate mutation uses anchored reads and
	// exact-revision CAS operations instead.
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return digestDoctorRepairBytes(data), nil
}

func readDoctorRepairBuild(data []byte) (string, string, string, error) {
	build, err := buildinfo.Read(bytes.NewReader(data))
	if err != nil {
		return "", "", "", err
	}
	return build.Path, build.Main.Path, build.Main.Version, nil
}

func digestDoctorRepairBytes(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func confirmDoctorRepair(reader io.Reader, writer io.Writer) (bool, error) {
	if _, err := fmt.Fprint(writer, "Type quarantine to apply this exact plan: "); err != nil {
		return false, err
	}
	scanner := bufio.NewScanner(reader)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return false, err
		}
		return false, nil
	}
	return strings.TrimSpace(scanner.Text()) == "quarantine", nil
}

func cancelledDoctorRepairResult(plan doctorRepairPlan) doctorRepairResult {
	return doctorRepairResult{
		SchemaVersion: doctorRepairSchemaVersion, PlanHash: plan.PlanHash, Status: "cancelled",
		Candidate: plan.Candidate, Reversible: false,
	}
}

func writeDoctorRepairHuman(writer io.Writer, plan doctorRepairPlan) error {
	_, err := fmt.Fprintf(writer, "Doctor Repair Plan\n==================\nAction: %s\nCandidate: %s\nDecision: %s\nSummary: %s\nPlan hash: %s\n", plan.Action, plan.Candidate, plan.DecisionCode, plan.Summary, plan.PlanHash)
	if err != nil {
		return err
	}
	if plan.Eligible {
		_, err = fmt.Fprintf(writer, "Quarantine: %s\nRecovery manifest: %s\n", plan.QuarantinePath, plan.ManifestPath)
	}
	return err
}

func writeDoctorRepairResultHuman(writer io.Writer, result doctorRepairResult) error {
	if result.Status == "cancelled" {
		_, err := fmt.Fprintln(writer, "Repair cancelled; no files were changed.")
		return err
	}
	_, err := fmt.Fprintf(writer, "Repair complete: stale PATH entry quarantined.\nPreserved binary: %s\nRecovery manifest: %s\nRestore guidance: %s\n", result.QuarantinePath, result.ManifestPath, result.RestoreGuidance)
	return err
}

func writeDoctorRepairJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
