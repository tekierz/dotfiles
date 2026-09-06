package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/tekierz/dotfiles/internal/safefile"
)

func TestDoctorRepairPlanAcceptsOnlyProvenStaleCandidateWithoutHomebrew(t *testing.T) {
	fixture := newDoctorRepairFixture(t)
	plan := fixture.plan(t)
	if !plan.Eligible || plan.Action != "quarantine" || plan.DecisionCode != "stale-user-local-candidate" {
		t.Fatalf("plan = %+v", plan)
	}
	if plan.ModulePath != dotfilesModulePath || plan.CommandPath != dotfilesCommandPath || len(plan.PlanHash) != 64 || plan.QuarantinePath == "" {
		t.Fatalf("plan proof = %+v", plan)
	}
	second := fixture.plan(t)
	if second.PlanHash != plan.PlanHash {
		t.Fatalf("plan hashes differ: %q != %q", plan.PlanHash, second.PlanHash)
	}
}

func TestDoctorRepairRefusesUnknownCandidate(t *testing.T) {
	fixture := newDoctorRepairFixture(t)
	if err := os.WriteFile(fixture.candidate, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	plan := fixture.plan(t)
	if plan.Eligible || plan.DecisionCode != "ownership-unproven" {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestDoctorRepairRefusesDifferentCommandFromSameModule(t *testing.T) {
	fixture := newDoctorRepairFixture(t)
	fixture.deps.inspectBuild = func([]byte) (string, string, string, error) {
		return "github.com/tekierz/dotfiles/cmd/other", dotfilesModulePath, "v1.0.0", nil
	}
	plan := fixture.plan(t)
	if plan.Eligible || plan.DecisionCode != "ownership-unproven" {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestDoctorRepairRefusesRunningCandidate(t *testing.T) {
	fixture := newDoctorRepairFixture(t)
	if err := os.Remove(fixture.candidate); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(fixture.running, fixture.candidate); err != nil {
		t.Fatal(err)
	}
	plan := fixture.plan(t)
	if plan.Eligible || plan.DecisionCode != "candidate-is-running" {
		t.Fatalf("plan = %+v", plan)
	}
	if _, err := os.Stat(fixture.candidate); err != nil {
		t.Fatalf("running candidate was changed: %v", err)
	}
}

func TestDoctorRepairRefusesHomebrewOwnedCandidate(t *testing.T) {
	fixture := newDoctorRepairFixture(t)
	brewPrefix := filepath.Join(t.TempDir(), "opt", "dotfiles")
	managed := filepath.Join(brewPrefix, "bin", "dotfiles")
	if err := os.MkdirAll(filepath.Dir(managed), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(fixture.candidate, managed); err != nil {
		t.Fatal(err)
	}
	fixture.deps.homebrewProbe = func(context.Context, string) (string, string, error) {
		return "/opt/homebrew/bin/brew", brewPrefix, nil
	}
	plan := fixture.plan(t)
	if plan.Eligible || plan.DecisionCode != "candidate-homebrew-owned" {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestDoctorRepairRefusesOtherwiseUnownedHardlink(t *testing.T) {
	fixture := newDoctorRepairFixture(t)
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Link(fixture.candidate, alias); err != nil {
		t.Fatal(err)
	}
	plan := fixture.plan(t)
	if plan.Eligible || plan.DecisionCode != "candidate-hardlinked" {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestDoctorRepairRefusesSymlinkSwapAfterPreview(t *testing.T) {
	fixture := newDoctorRepairFixture(t)
	plan := fixture.plan(t)
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("outside"), 0o700); err != nil {
		t.Fatal(err)
	}
	fixture.deps.beforeApply = func() error {
		if err := os.Remove(fixture.candidate); err != nil {
			return err
		}
		return os.Symlink(outside, fixture.candidate)
	}
	if _, err := applyDoctorRepairPlan(plan, fixture.deps); !errors.Is(err, safefile.ErrSymlink) {
		t.Fatalf("apply error = %v, want ErrSymlink", err)
	}
	if got, err := os.ReadFile(outside); err != nil || string(got) != "outside" {
		t.Fatalf("outside target changed: %q, %v", got, err)
	}
}

func TestDoctorRepairRefusesContentChangeAfterPreview(t *testing.T) {
	fixture := newDoctorRepairFixture(t)
	plan := fixture.plan(t)
	fixture.deps.beforeApply = func() error {
		return os.WriteFile(fixture.candidate, append(append([]byte(nil), fixture.staleBytes...), 'x'), 0o700)
	}
	if _, err := applyDoctorRepairPlan(plan, fixture.deps); !errors.Is(err, errDoctorRepairStateChanged) {
		t.Fatalf("apply error = %v, want state changed", err)
	}
	if _, err := os.Stat(fixture.candidate); err != nil {
		t.Fatalf("changed candidate was removed: %v", err)
	}
}

func TestDoctorRepairCancelLeavesCandidateUntouched(t *testing.T) {
	fixture := newDoctorRepairFixture(t)
	cmd := newDoctorRepairCommand(fixture.deps)
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetIn(strings.NewReader("no\n"))
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Repair cancelled; no files were changed.") {
		t.Fatalf("output = %s", output.String())
	}
	if got, err := os.ReadFile(fixture.candidate); err != nil || !bytes.Equal(got, fixture.staleBytes) {
		t.Fatalf("candidate changed on cancel: %v", err)
	}
}

func TestDoctorRepairSuccessfullyCreatesReversiblePrivateQuarantine(t *testing.T) {
	fixture := newDoctorRepairFixture(t)
	plan := fixture.plan(t)
	result, err := applyDoctorRepairPlan(plan, fixture.deps)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "quarantined" || !result.Reversible || result.RestoreGuidance == "" {
		t.Fatalf("result = %+v", result)
	}
	if _, err := os.Lstat(fixture.candidate); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("candidate still exists: %v", err)
	}
	quarantined, err := os.ReadFile(plan.QuarantinePath)
	if err != nil || !bytes.Equal(quarantined, fixture.staleBytes) {
		t.Fatalf("quarantined bytes mismatch: %v", err)
	}
	if info, err := os.Stat(plan.QuarantinePath); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("quarantine mode = %v, err = %v", infoMode(info), err)
	}
	manifestData, err := os.ReadFile(plan.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest doctorRepairManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.PlanHash != plan.PlanHash || manifest.OriginalMode != "0700" || manifest.RestoreGuidance == "" || manifest.CandidateDigest != plan.CandidateDigest || manifest.CommandPath != dotfilesCommandPath {
		t.Fatalf("manifest = %+v", manifest)
	}
	if info, err := os.Stat(plan.ManifestPath); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("manifest mode = %v, err = %v", infoMode(info), err)
	}
}

func TestDoctorRepairRefusesManifestCollisionWithoutRemovingCandidate(t *testing.T) {
	fixture := newDoctorRepairFixture(t)
	plan := fixture.plan(t)
	if err := os.MkdirAll(filepath.Dir(plan.ManifestPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(plan.ManifestPath, []byte("different"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := applyDoctorRepairPlan(plan, fixture.deps); err == nil || !strings.Contains(err.Error(), "manifest already exists") {
		t.Fatalf("apply error = %v", err)
	}
	if _, err := os.Stat(fixture.candidate); err != nil {
		t.Fatalf("candidate removed despite manifest collision: %v", err)
	}
}

func TestDoctorRepairRefusesQuarantineCollisionAtCommitBoundary(t *testing.T) {
	fixture := newDoctorRepairFixture(t)
	plan := fixture.plan(t)
	fixture.deps.beforeQuarantineWrite = func() error {
		return os.WriteFile(plan.QuarantinePath, []byte("racing quarantine"), 0o600)
	}
	if _, err := applyDoctorRepairPlan(plan, fixture.deps); !errors.Is(err, safefile.ErrRevisionChanged) {
		t.Fatalf("apply error = %v, want ErrRevisionChanged", err)
	}
	if got, err := os.ReadFile(plan.QuarantinePath); err != nil || string(got) != "racing quarantine" {
		t.Fatalf("racing quarantine was overwritten: %q, %v", got, err)
	}
	if _, err := os.Stat(fixture.candidate); err != nil {
		t.Fatalf("candidate removed after quarantine collision: %v", err)
	}
}

func TestDoctorRepairRefusesManifestCollisionAtCommitBoundary(t *testing.T) {
	fixture := newDoctorRepairFixture(t)
	plan := fixture.plan(t)
	fixture.deps.beforeManifestWrite = func() error {
		return os.WriteFile(plan.ManifestPath, []byte("racing manifest"), 0o600)
	}
	if _, err := applyDoctorRepairPlan(plan, fixture.deps); !errors.Is(err, safefile.ErrRevisionChanged) {
		t.Fatalf("apply error = %v, want ErrRevisionChanged", err)
	}
	if got, err := os.ReadFile(plan.ManifestPath); err != nil || string(got) != "racing manifest" {
		t.Fatalf("racing manifest was overwritten: %q, %v", got, err)
	}
	if _, err := os.Stat(fixture.candidate); err != nil {
		t.Fatalf("candidate removed after manifest collision: %v", err)
	}
}

func TestDoctorRepairNoninteractiveRequiresFreshPlanHash(t *testing.T) {
	fixture := newDoctorRepairFixture(t)
	cmd := newDoctorRepairCommand(fixture.deps)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"--json", "--yes"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "requires --plan-hash") {
		t.Fatalf("missing hash error = %v", err)
	}
	if _, err := os.Stat(fixture.candidate); err != nil {
		t.Fatalf("candidate changed without plan hash: %v", err)
	}
}

func TestDoctorRepairNoninteractiveRejectsStalePlanHash(t *testing.T) {
	fixture := newDoctorRepairFixture(t)
	cmd := newDoctorRepairCommand(fixture.deps)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"--json", "--yes", "--plan-hash", strings.Repeat("0", 64)})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "does not match current state") {
		t.Fatalf("stale hash error = %v", err)
	}
	if _, err := os.Stat(fixture.candidate); err != nil {
		t.Fatalf("candidate changed with stale plan hash: %v", err)
	}
}

func TestDoctorRepairJSONPreviewIsDeterministicAndReadOnly(t *testing.T) {
	fixture := newDoctorRepairFixture(t)
	first := executeDoctorCommand(t, newDoctorRepairCommand(fixture.deps), "--json")
	second := executeDoctorCommand(t, newDoctorRepairCommand(fixture.deps), "--json")
	if first != second {
		t.Fatalf("preview changed without a state change:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
	var plan doctorRepairPlan
	if err := json.Unmarshal([]byte(first), &plan); err != nil {
		t.Fatal(err)
	}
	if !plan.Eligible || plan.PlanHash == "" {
		t.Fatalf("preview = %+v", plan)
	}
	if _, err := os.Stat(fixture.candidate); err != nil {
		t.Fatalf("JSON preview changed candidate: %v", err)
	}
}

func TestDoctorRepairBoundsHomebrewProbe(t *testing.T) {
	fixture := newDoctorRepairFixture(t)
	fixture.deps.homebrewProbe = func(ctx context.Context, _ string) (string, string, error) {
		<-ctx.Done()
		return "", "", ctx.Err()
	}
	started := time.Now()
	plan := fixture.plan(t)
	if !plan.Eligible {
		t.Fatalf("timed-out Homebrew probe should behave as unavailable: %+v", plan)
	}
	if elapsed := time.Since(started); elapsed > 4*time.Second {
		t.Fatalf("Homebrew probe was not bounded: %v", elapsed)
	}
}

type doctorRepairFixture struct {
	home       string
	running    string
	candidate  string
	staleBytes []byte
	deps       doctorRepairDependencies
}

func newDoctorRepairFixture(t *testing.T) doctorRepairFixture {
	t.Helper()
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("descriptor-anchored repair is supported on Darwin and Linux")
	}
	running, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	running, err = filepath.Abs(running)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(running)
	if err != nil {
		t.Fatal(err)
	}
	stale := append(append([]byte(nil), data...), []byte("doctor-repair-stale-fixture")...)
	home := t.TempDir()
	candidate := filepath.Join(home, filepath.FromSlash(doctorRepairCandidateRel))
	if err := os.MkdirAll(filepath.Dir(candidate), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(candidate, stale, 0o700); err != nil {
		t.Fatal(err)
	}
	deps := doctorRepairDependencies{
		executable:  func() (string, error) { return running, nil },
		userHomeDir: func() (string, error) { return home, nil },
		homebrewProbe: func(context.Context, string) (string, string, error) {
			return "", "", errors.New("brew unavailable")
		},
	}
	return doctorRepairFixture{home: home, running: running, candidate: candidate, staleBytes: stale, deps: deps}
}

func (fixture doctorRepairFixture) plan(t *testing.T) doctorRepairPlan {
	t.Helper()
	plan, err := buildDoctorRepairPlan(context.Background(), fixture.deps)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func infoMode(info os.FileInfo) os.FileMode {
	if info == nil {
		return 0
	}
	return info.Mode().Perm()
}
