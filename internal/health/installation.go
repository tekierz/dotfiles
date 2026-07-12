package health

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	CurrentInstallationSchemaVersion = 1
	MaxInstallationTools             = 1024
	maxEvidenceItems                 = 256
	maxDirectAlternatives            = 64
	maxIdentifiers                   = 64
	maxFieldBytes                    = 256
)

type Presence string

const (
	PresencePresent Presence = "present"
	PresencePartial Presence = "partial"
	PresenceMissing Presence = "missing"
	PresenceUnknown Presence = "unknown"
)

type Installability string

const (
	InstallabilitySupported   Installability = "supported"
	InstallabilityUnsupported Installability = "unsupported"
	InstallabilityUnknown     Installability = "unknown"
)

type PackageState string

const (
	PackagePresent       PackageState = "present"
	PackagePartial       PackageState = "partial"
	PackageMissing       PackageState = "missing"
	PackageUnknown       PackageState = "unknown"
	PackageNotApplicable PackageState = "not_applicable"
)

type ComponentState string

const (
	ComponentPresent       ComponentState = "present"
	ComponentMissing       ComponentState = "missing"
	ComponentUnknown       ComponentState = "unknown"
	ComponentNotApplicable ComponentState = "not_applicable"
)

type PackageNamespace string

const (
	PackageNamespaceCask    PackageNamespace = "cask"
	PackageNamespaceFormula PackageNamespace = "formula"
	PackageNamespaceSystem  PackageNamespace = "system"
)

type DirectSourceKind string

const (
	DirectSourceAppBundle DirectSourceKind = "app_bundle"
	DirectSourceBinary    DirectSourceKind = "binary"
	DirectSourceFlatpak   DirectSourceKind = "flatpak"
	DirectSourceLegacy    DirectSourceKind = "legacy_detector"
)

type DiagnosticCode string

const (
	DiagnosticPackageBatchFailed DiagnosticCode = "package_batch_failed"
	DiagnosticFallbackTimeout    DiagnosticCode = "fallback_timeout"
	DiagnosticCancelled          DiagnosticCode = "cancelled"
	DiagnosticProbeFailed        DiagnosticCode = "probe_failed"
	DiagnosticProbeTimeout       DiagnosticCode = "probe_timeout"
)

type PackageNamespaceFacet struct {
	Namespace          PackageNamespace
	State              PackageState
	ExpectedReceipts   []string
	ObservedReceipts   []string
	MissingReceipts    []string
	UnresolvedReceipts []string
	Complete           bool
	DiagnosticCode     DiagnosticCode
	DiagnosticSummary  string
}

type PackageFacet struct {
	State              PackageState
	Provider           string
	ExpectedReceipts   []string
	ObservedReceipts   []string
	MissingReceipts    []string
	UnresolvedReceipts []string
	Authoritative      bool
	Complete           bool
	Namespaces         []PackageNamespaceFacet
	DiagnosticCode     DiagnosticCode
	DiagnosticSummary  string
}

type DirectAlternative struct {
	Kind              DirectSourceKind
	Identifiers       []string
	State             ComponentState
	DiagnosticCode    DiagnosticCode
	DiagnosticSummary string
}

type DirectFacet struct {
	State             ComponentState
	Authoritative     bool
	Alternatives      []DirectAlternative
	DiagnosticCode    DiagnosticCode
	DiagnosticSummary string
}

type InstallationObservationSpec struct {
	ToolID              string
	Installability      Installability
	InstallRecipeDigest string
	Package             PackageFacet
	Direct              DirectFacet
}

type InstallationObservation struct {
	toolID              string
	installability      Installability
	installRecipeDigest string
	presence            Presence
	packageFacet        PackageFacet
	directFacet         DirectFacet
}

// ValidateInstallationToolIDs validates collector input before any external
// observation begins. It intentionally shares the same identifier and count
// contract as immutable snapshots.
func ValidateInstallationToolIDs(ids []string) error {
	if len(ids) > MaxInstallationTools {
		return fmt.Errorf("too many tools: %d", len(ids))
	}
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if err := validateIdentifier("tool ID", id); err != nil {
			return err
		}
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("duplicate tool ID %q", id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

func NewInstallationObservation(spec InstallationObservationSpec) (InstallationObservation, error) {
	if err := validateIdentifier("tool ID", spec.ToolID); err != nil {
		return InstallationObservation{}, err
	}
	if !validInstallability(spec.Installability) {
		return InstallationObservation{}, fmt.Errorf("invalid installability %q", spec.Installability)
	}
	if spec.Installability == InstallabilitySupported {
		if !validLowerHexDigest(spec.InstallRecipeDigest) {
			return InstallationObservation{}, errors.New("supported installation requires a valid recipe digest")
		}
	} else if spec.InstallRecipeDigest != "" {
		return InstallationObservation{}, errors.New("non-supported installation cannot carry a recipe digest")
	}
	packageFacet, err := normalizePackageFacet(spec.Package)
	if err != nil {
		return InstallationObservation{}, fmt.Errorf("package facet: %w", err)
	}
	directFacet, err := normalizeDirectFacet(spec.Direct)
	if err != nil {
		return InstallationObservation{}, fmt.Errorf("direct facet: %w", err)
	}
	return InstallationObservation{
		toolID: spec.ToolID, installability: spec.Installability, installRecipeDigest: spec.InstallRecipeDigest,
		presence:     derivePresence(packageFacet, directFacet),
		packageFacet: packageFacet, directFacet: directFacet,
	}, nil
}

func (o InstallationObservation) ToolID() string                 { return o.toolID }
func (o InstallationObservation) Installability() Installability { return o.installability }
func (o InstallationObservation) InstallRecipeDigest() string    { return o.installRecipeDigest }
func (o InstallationObservation) Presence() Presence             { return o.presence }
func (o InstallationObservation) Package() PackageFacet          { return clonePackageFacet(o.packageFacet) }
func (o InstallationObservation) Direct() DirectFacet            { return cloneDirectFacet(o.directFacet) }

type InstallationSnapshotSpec struct {
	Generation uint64
	Platform   string
	Manager    string
	Tools      []InstallationObservation
}

type InstallationSnapshot struct {
	schemaVersion int
	generation    uint64
	platform      string
	manager       string
	tools         []InstallationObservation
	byID          map[string]InstallationObservation
	canonical     []byte
	digest        string
}

type canonicalInstallationSnapshot struct {
	SchemaVersion int                                `json:"schema_version"`
	Generation    uint64                             `json:"generation"`
	Platform      string                             `json:"platform"`
	Manager       string                             `json:"manager"`
	Tools         []canonicalInstallationObservation `json:"tools"`
}
type canonicalInstallationObservation struct {
	ToolID              string                `json:"tool_id"`
	Installability      Installability        `json:"installability"`
	InstallRecipeDigest string                `json:"install_recipe_digest"`
	Presence            Presence              `json:"presence"`
	Package             canonicalPackageFacet `json:"package"`
	Direct              canonicalDirectFacet  `json:"direct"`
}
type canonicalPackageFacet struct {
	State              PackageState                     `json:"state"`
	Provider           string                           `json:"provider"`
	ExpectedReceipts   []string                         `json:"expected_receipts"`
	ObservedReceipts   []string                         `json:"observed_receipts"`
	MissingReceipts    []string                         `json:"missing_receipts"`
	UnresolvedReceipts []string                         `json:"unresolved_receipts"`
	Authoritative      bool                             `json:"authoritative"`
	Complete           bool                             `json:"complete"`
	Namespaces         []canonicalPackageNamespaceFacet `json:"namespaces"`
	DiagnosticCode     DiagnosticCode                   `json:"diagnostic_code"`
	DiagnosticSummary  string                           `json:"diagnostic_summary"`
}
type canonicalPackageNamespaceFacet struct {
	Namespace          PackageNamespace `json:"namespace"`
	State              PackageState     `json:"state"`
	ExpectedReceipts   []string         `json:"expected_receipts"`
	ObservedReceipts   []string         `json:"observed_receipts"`
	MissingReceipts    []string         `json:"missing_receipts"`
	UnresolvedReceipts []string         `json:"unresolved_receipts"`
	Complete           bool             `json:"complete"`
	DiagnosticCode     DiagnosticCode   `json:"diagnostic_code"`
	DiagnosticSummary  string           `json:"diagnostic_summary"`
}
type canonicalDirectFacet struct {
	State             ComponentState               `json:"state"`
	Authoritative     bool                         `json:"authoritative"`
	Alternatives      []canonicalDirectAlternative `json:"alternatives"`
	DiagnosticCode    DiagnosticCode               `json:"diagnostic_code"`
	DiagnosticSummary string                       `json:"diagnostic_summary"`
}
type canonicalDirectAlternative struct {
	Kind              DirectSourceKind `json:"kind"`
	Identifiers       []string         `json:"identifiers"`
	State             ComponentState   `json:"state"`
	DiagnosticCode    DiagnosticCode   `json:"diagnostic_code"`
	DiagnosticSummary string           `json:"diagnostic_summary"`
}

func canonicalSnapshotFrom(schema int, generation uint64, platform, manager string, tools []InstallationObservation) canonicalInstallationSnapshot {
	document := canonicalInstallationSnapshot{SchemaVersion: schema, Generation: generation, Platform: platform, Manager: manager, Tools: make([]canonicalInstallationObservation, len(tools))}
	for index, observation := range tools {
		packageFacet, directFacet := observation.packageFacet, observation.directFacet
		canonicalPackage := canonicalPackageFacet{State: packageFacet.State, Provider: packageFacet.Provider, ExpectedReceipts: nonNilStrings(packageFacet.ExpectedReceipts), ObservedReceipts: nonNilStrings(packageFacet.ObservedReceipts), MissingReceipts: nonNilStrings(packageFacet.MissingReceipts), UnresolvedReceipts: nonNilStrings(packageFacet.UnresolvedReceipts), Authoritative: packageFacet.Authoritative, Complete: packageFacet.Complete, Namespaces: make([]canonicalPackageNamespaceFacet, len(packageFacet.Namespaces)), DiagnosticCode: packageFacet.DiagnosticCode, DiagnosticSummary: packageFacet.DiagnosticSummary}
		for namespaceIndex, namespace := range packageFacet.Namespaces {
			canonicalPackage.Namespaces[namespaceIndex] = canonicalPackageNamespaceFacet{Namespace: namespace.Namespace, State: namespace.State, ExpectedReceipts: nonNilStrings(namespace.ExpectedReceipts), ObservedReceipts: nonNilStrings(namespace.ObservedReceipts), MissingReceipts: nonNilStrings(namespace.MissingReceipts), UnresolvedReceipts: nonNilStrings(namespace.UnresolvedReceipts), Complete: namespace.Complete, DiagnosticCode: namespace.DiagnosticCode, DiagnosticSummary: namespace.DiagnosticSummary}
		}
		canonicalDirect := canonicalDirectFacet{State: directFacet.State, Authoritative: directFacet.Authoritative, Alternatives: make([]canonicalDirectAlternative, len(directFacet.Alternatives)), DiagnosticCode: directFacet.DiagnosticCode, DiagnosticSummary: directFacet.DiagnosticSummary}
		for alternativeIndex, alternative := range directFacet.Alternatives {
			canonicalDirect.Alternatives[alternativeIndex] = canonicalDirectAlternative{Kind: alternative.Kind, Identifiers: nonNilStrings(alternative.Identifiers), State: alternative.State, DiagnosticCode: alternative.DiagnosticCode, DiagnosticSummary: alternative.DiagnosticSummary}
		}
		document.Tools[index] = canonicalInstallationObservation{ToolID: observation.toolID, Installability: observation.installability, InstallRecipeDigest: observation.installRecipeDigest, Presence: observation.presence, Package: canonicalPackage, Direct: canonicalDirect}
	}
	return document
}

func nonNilStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	return slices.Clone(values)
}

func NewInstallationSnapshot(spec InstallationSnapshotSpec) (InstallationSnapshot, error) {
	if spec.Generation == 0 {
		return InstallationSnapshot{}, errors.New("generation must be positive")
	}
	if err := validateIdentifier("platform", spec.Platform); err != nil {
		return InstallationSnapshot{}, err
	}
	if spec.Manager != "" {
		if err := validateIdentifier("manager", spec.Manager); err != nil {
			return InstallationSnapshot{}, err
		}
	}
	if len(spec.Tools) > MaxInstallationTools {
		return InstallationSnapshot{}, fmt.Errorf("too many tools: %d", len(spec.Tools))
	}
	tools := slices.Clone(spec.Tools)
	sort.Slice(tools, func(i, j int) bool { return tools[i].toolID < tools[j].toolID })
	byID := make(map[string]InstallationObservation, len(tools))
	for _, observation := range tools {
		if observation.toolID == "" {
			return InstallationSnapshot{}, errors.New("snapshot contains zero observation")
		}
		if _, exists := byID[observation.toolID]; exists {
			return InstallationSnapshot{}, fmt.Errorf("duplicate tool ID %q", observation.toolID)
		}
		byID[observation.toolID] = observation
	}
	canonical, err := json.Marshal(canonicalSnapshotFrom(CurrentInstallationSchemaVersion, spec.Generation, spec.Platform, spec.Manager, tools))
	if err != nil {
		return InstallationSnapshot{}, fmt.Errorf("marshal canonical installation snapshot: %w", err)
	}
	hash := sha256.Sum256(canonical)
	return InstallationSnapshot{schemaVersion: CurrentInstallationSchemaVersion, generation: spec.Generation, platform: spec.Platform, manager: spec.Manager, tools: tools, byID: byID, canonical: canonical, digest: hex.EncodeToString(hash[:])}, nil
}

func (s InstallationSnapshot) SchemaVersion() int     { return s.schemaVersion }
func (s InstallationSnapshot) Generation() uint64     { return s.generation }
func (s InstallationSnapshot) Platform() string       { return s.platform }
func (s InstallationSnapshot) Manager() string        { return s.manager }
func (s InstallationSnapshot) Digest() string         { return s.digest }
func (s InstallationSnapshot) CanonicalBytes() []byte { return slices.Clone(s.canonical) }
func (s InstallationSnapshot) Tools() []InstallationObservation {
	return slices.Clone(s.tools)
}
func (s InstallationSnapshot) Tool(id string) (InstallationObservation, bool) {
	observation, ok := s.byID[id]
	return observation, ok
}

func normalizePackageFacet(facet PackageFacet) (PackageFacet, error) {
	if !validPackageState(facet.State) {
		return PackageFacet{}, fmt.Errorf("invalid state %q", facet.State)
	}
	if facet.Provider != "" {
		if err := validateIdentifier("provider", facet.Provider); err != nil {
			return PackageFacet{}, err
		}
	}
	normalized := clonePackageFacet(facet)
	var err error
	normalized.ExpectedReceipts, err = normalizeIdentifiers("expected receipts", facet.ExpectedReceipts, maxEvidenceItems)
	if err != nil {
		return PackageFacet{}, err
	}
	normalized.ObservedReceipts, err = normalizeIdentifiers("observed receipts", facet.ObservedReceipts, maxEvidenceItems)
	if err != nil {
		return PackageFacet{}, err
	}
	normalized.MissingReceipts, err = normalizeIdentifiers("missing receipts", facet.MissingReceipts, maxEvidenceItems)
	if err != nil {
		return PackageFacet{}, err
	}
	normalized.UnresolvedReceipts, err = normalizeIdentifiers("unresolved receipts", facet.UnresolvedReceipts, maxEvidenceItems)
	if err != nil {
		return PackageFacet{}, err
	}
	normalized.UnresolvedReceipts = fillImplicitUnresolved(normalized.State, normalized.Complete, normalized.ExpectedReceipts, normalized.ObservedReceipts, normalized.MissingReceipts, normalized.UnresolvedReceipts)
	if err := validateReceiptPartition(normalized.State, normalized.ExpectedReceipts, normalized.ObservedReceipts, normalized.MissingReceipts, normalized.UnresolvedReceipts, normalized.Complete); err != nil {
		return PackageFacet{}, err
	}
	if err := normalizeDiagnostic(&normalized.DiagnosticCode, &normalized.DiagnosticSummary); err != nil {
		return PackageFacet{}, err
	}
	seenNamespaces := make(map[PackageNamespace]struct{}, len(facet.Namespaces))
	normalized.Namespaces = make([]PackageNamespaceFacet, len(facet.Namespaces))
	for index, namespace := range facet.Namespaces {
		if namespace.Namespace != PackageNamespaceFormula && namespace.Namespace != PackageNamespaceCask && namespace.Namespace != PackageNamespaceSystem {
			return PackageFacet{}, fmt.Errorf("invalid package namespace %q", namespace.Namespace)
		}
		if _, exists := seenNamespaces[namespace.Namespace]; exists {
			return PackageFacet{}, fmt.Errorf("duplicate package namespace %q", namespace.Namespace)
		}
		seenNamespaces[namespace.Namespace] = struct{}{}
		normalizedNamespace, err := normalizeNamespaceFacet(namespace)
		if err != nil {
			return PackageFacet{}, err
		}
		normalized.Namespaces[index] = normalizedNamespace
	}
	sort.Slice(normalized.Namespaces, func(i, j int) bool { return normalized.Namespaces[i].Namespace < normalized.Namespaces[j].Namespace })
	if len(normalized.Namespaces) > 0 {
		if err := validateNamespaceAggregate(normalized); err != nil {
			return PackageFacet{}, err
		}
	}
	return normalized, nil
}

func validateNamespaceAggregate(facet PackageFacet) error {
	union := make(map[string]struct{})
	for _, namespace := range facet.Namespaces {
		for _, receipt := range namespace.ExpectedReceipts {
			union[receipt] = struct{}{}
		}
	}
	expected := make([]string, 0, len(union))
	for receipt := range union {
		expected = append(expected, receipt)
	}
	sort.Strings(expected)
	if !slices.Equal(expected, facet.ExpectedReceipts) {
		return errors.New("namespace expected receipts disagree with aggregate")
	}
	var observed, missing, unresolved []string
	for _, receipt := range expected {
		positive, allNegative := false, true
		for _, namespace := range facet.Namespaces {
			if !slices.Contains(namespace.ExpectedReceipts, receipt) {
				continue
			}
			if slices.Contains(namespace.ObservedReceipts, receipt) {
				positive = true
			}
			if !slices.Contains(namespace.MissingReceipts, receipt) {
				allNegative = false
			}
		}
		switch {
		case positive:
			observed = append(observed, receipt)
		case allNegative:
			missing = append(missing, receipt)
		default:
			unresolved = append(unresolved, receipt)
		}
	}
	state := packageStateFromParts(expected, observed, missing, unresolved)
	if !slices.Equal(observed, facet.ObservedReceipts) || !slices.Equal(missing, facet.MissingReceipts) || !slices.Equal(unresolved, facet.UnresolvedReceipts) || facet.State != state || facet.Complete != (len(unresolved) == 0) {
		return errors.New("namespace evidence disagrees with aggregate")
	}
	return nil
}

func packageStateFromParts(expected, observed, missing, unresolved []string) PackageState {
	switch {
	case len(expected) == 0:
		return PackageNotApplicable
	case len(observed) == len(expected):
		return PackagePresent
	case len(missing) == len(expected):
		return PackageMissing
	case len(unresolved) > 0 && len(observed) == 0:
		return PackageUnknown
	case len(observed) > 0 || len(missing) > 0:
		return PackagePartial
	default:
		return PackageUnknown
	}
}

func normalizeNamespaceFacet(facet PackageNamespaceFacet) (PackageNamespaceFacet, error) {
	normalized := facet
	var err error
	normalized.ExpectedReceipts, err = normalizeIdentifiers("namespace expected receipts", facet.ExpectedReceipts, maxEvidenceItems)
	if err != nil {
		return PackageNamespaceFacet{}, err
	}
	normalized.ObservedReceipts, err = normalizeIdentifiers("namespace observed receipts", facet.ObservedReceipts, maxEvidenceItems)
	if err != nil {
		return PackageNamespaceFacet{}, err
	}
	normalized.MissingReceipts, err = normalizeIdentifiers("namespace missing receipts", facet.MissingReceipts, maxEvidenceItems)
	if err != nil {
		return PackageNamespaceFacet{}, err
	}
	normalized.UnresolvedReceipts, err = normalizeIdentifiers("namespace unresolved receipts", facet.UnresolvedReceipts, maxEvidenceItems)
	if err != nil {
		return PackageNamespaceFacet{}, err
	}
	normalized.UnresolvedReceipts = fillImplicitUnresolved(normalized.State, normalized.Complete, normalized.ExpectedReceipts, normalized.ObservedReceipts, normalized.MissingReceipts, normalized.UnresolvedReceipts)
	if err := validateReceiptPartition(normalized.State, normalized.ExpectedReceipts, normalized.ObservedReceipts, normalized.MissingReceipts, normalized.UnresolvedReceipts, normalized.Complete); err != nil {
		return PackageNamespaceFacet{}, err
	}
	if err := normalizeDiagnostic(&normalized.DiagnosticCode, &normalized.DiagnosticSummary); err != nil {
		return PackageNamespaceFacet{}, err
	}
	return normalized, nil
}

func validateReceiptPartition(state PackageState, expected, observed, missing, unresolved []string, complete bool) error {
	if !validPackageState(state) {
		return fmt.Errorf("invalid package state %q", state)
	}
	sets := [][]string{observed, missing, unresolved}
	seen := make(map[string]struct{})
	for _, values := range sets {
		for _, value := range values {
			if _, duplicate := seen[value]; duplicate {
				return fmt.Errorf("receipt %q appears in conflicting sets", value)
			}
			seen[value] = struct{}{}
		}
	}
	if len(expected) != len(seen) {
		return errors.New("receipt partition does not cover expected receipts")
	}
	for _, value := range expected {
		if _, ok := seen[value]; !ok {
			return fmt.Errorf("expected receipt %q is unclassified", value)
		}
	}
	if state != PackageNotApplicable && complete != (len(unresolved) == 0) {
		return errors.New("package completeness disagrees with unresolved receipts")
	}
	want := packageStateFromParts(expected, observed, missing, unresolved)
	if state != want {
		return fmt.Errorf("package state %q disagrees with receipt evidence %q", state, want)
	}
	return nil
}

func fillImplicitUnresolved(state PackageState, complete bool, expected, observed, missing, unresolved []string) []string {
	if complete || state != PackageUnknown {
		return unresolved
	}
	classified := make(map[string]struct{}, len(observed)+len(missing)+len(unresolved))
	for _, values := range [][]string{observed, missing, unresolved} {
		for _, value := range values {
			classified[value] = struct{}{}
		}
	}
	filled := slices.Clone(unresolved)
	for _, value := range expected {
		if _, ok := classified[value]; !ok {
			filled = append(filled, value)
		}
	}
	sort.Strings(filled)
	return filled
}

func normalizeDirectFacet(facet DirectFacet) (DirectFacet, error) {
	if !validComponentState(facet.State) {
		return DirectFacet{}, fmt.Errorf("invalid state %q", facet.State)
	}
	if len(facet.Alternatives) > maxDirectAlternatives {
		return DirectFacet{}, fmt.Errorf("too many direct alternatives: %d", len(facet.Alternatives))
	}
	normalized := facet
	if err := normalizeDiagnostic(&normalized.DiagnosticCode, &normalized.DiagnosticSummary); err != nil {
		return DirectFacet{}, err
	}
	normalized.Alternatives = make([]DirectAlternative, len(facet.Alternatives))
	seen := make(map[string]struct{}, len(facet.Alternatives))
	for index, alternative := range facet.Alternatives {
		if !validDirectKind(alternative.Kind) || !validComponentState(alternative.State) {
			return DirectFacet{}, errors.New("invalid direct alternative")
		}
		identifiers, err := normalizeIdentifiers("direct identifiers", alternative.Identifiers, maxIdentifiers)
		if err != nil {
			return DirectFacet{}, err
		}
		if len(identifiers) == 0 {
			return DirectFacet{}, errors.New("direct alternative requires at least one identifier")
		}
		key := string(alternative.Kind) + "\x00" + strings.Join(identifiers, "\x00")
		if _, duplicate := seen[key]; duplicate {
			return DirectFacet{}, errors.New("duplicate direct alternative")
		}
		seen[key] = struct{}{}
		alternative.Identifiers = identifiers
		if err := normalizeDiagnostic(&alternative.DiagnosticCode, &alternative.DiagnosticSummary); err != nil {
			return DirectFacet{}, err
		}
		normalized.Alternatives[index] = alternative
	}
	sort.Slice(normalized.Alternatives, func(i, j int) bool {
		if normalized.Alternatives[i].Kind != normalized.Alternatives[j].Kind {
			return normalized.Alternatives[i].Kind < normalized.Alternatives[j].Kind
		}
		return strings.Join(normalized.Alternatives[i].Identifiers, "\x00") < strings.Join(normalized.Alternatives[j].Identifiers, "\x00")
	})
	want := aggregateDirectState(normalized.Alternatives)
	if len(normalized.Alternatives) == 0 {
		want = normalized.State
	}
	if normalized.State != want {
		return DirectFacet{}, fmt.Errorf("direct state %q disagrees with alternatives %q", normalized.State, want)
	}
	return normalized, nil
}

func aggregateDirectState(alternatives []DirectAlternative) ComponentState {
	if len(alternatives) == 0 {
		return ComponentNotApplicable
	}
	hasUnknown, hasMissing := false, false
	for _, alternative := range alternatives {
		switch alternative.State {
		case ComponentPresent:
			return ComponentPresent
		case ComponentUnknown:
			hasUnknown = true
		case ComponentMissing:
			hasMissing = true
		case ComponentNotApplicable:
			// This alternative contributes no installation evidence.
		}
	}
	if hasUnknown {
		return ComponentUnknown
	}
	if hasMissing {
		return ComponentMissing
	}
	return ComponentNotApplicable
}

func derivePresence(packages PackageFacet, direct DirectFacet) Presence {
	if direct.State == ComponentPresent {
		return PresencePresent
	}
	if packages.State == PackagePartial {
		return PresencePartial
	}
	if packages.State == PackagePresent {
		if direct.Authoritative && (direct.State == ComponentMissing || direct.State == ComponentUnknown) {
			return PresencePartial
		}
		if packages.Authoritative {
			return PresencePresent
		}
		return PresencePartial
	}
	if packages.State == PackageMissing {
		if direct.Authoritative && direct.State == ComponentMissing {
			return PresenceMissing
		}
		if packages.Authoritative && (direct.State == ComponentMissing || direct.State == ComponentNotApplicable) {
			return PresenceMissing
		}
		return PresenceUnknown
	}
	if packages.State == PackageNotApplicable && direct.Authoritative && direct.State == ComponentMissing {
		return PresenceMissing
	}
	return PresenceUnknown
}

func normalizeIdentifiers(label string, values []string, limit int) ([]string, error) {
	if len(values) > limit {
		return nil, fmt.Errorf("too many %s: %d", label, len(values))
	}
	normalized := slices.Clone(values)
	sort.Strings(normalized)
	for index, value := range normalized {
		if err := validateIdentifier(label, value); err != nil {
			return nil, err
		}
		if index > 0 && value == normalized[index-1] {
			return nil, fmt.Errorf("duplicate %s %q", label, value)
		}
	}
	return normalized, nil
}

func validateIdentifier(label, value string) error {
	if value == "" || len(value) > maxFieldBytes || !utf8.ValidString(value) {
		return fmt.Errorf("invalid %s", label)
	}
	for _, r := range value {
		if unicode.IsControl(r) || isBidiControl(r) {
			return fmt.Errorf("invalid %s", label)
		}
	}
	return nil
}

func validLowerHexDigest(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func normalizeDiagnostic(code *DiagnosticCode, summary *string) error {
	if *code != "" && !validDiagnosticCode(*code) {
		return fmt.Errorf("invalid diagnostic code %q", *code)
	}
	*summary = sanitizeDiagnostic(*summary)
	return nil
}

func sanitizeDiagnostic(value string) string {
	var builder strings.Builder
	for index := 0; index < len(value); {
		r, size := utf8.DecodeRuneInString(value[index:])
		if r == utf8.RuneError && size == 1 {
			index++
			continue
		}
		if r == 0x1b {
			index += size
			if index < len(value) && value[index] == '[' {
				index++
				for index < len(value) {
					b := value[index]
					index++
					if b >= 0x40 && b <= 0x7e {
						break
					}
				}
			}
			continue
		}
		index += size
		if unicode.IsControl(r) || isBidiControl(r) {
			continue
		}
		if builder.Len()+size > maxFieldBytes {
			break
		}
		builder.WriteRune(r)
	}
	return builder.String()
}

func isBidiControl(r rune) bool {
	return (r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069) || r == 0x200e || r == 0x200f || r == 0x061c
}

func clonePackageFacet(facet PackageFacet) PackageFacet {
	cloned := facet
	cloned.ExpectedReceipts = slices.Clone(facet.ExpectedReceipts)
	cloned.ObservedReceipts = slices.Clone(facet.ObservedReceipts)
	cloned.MissingReceipts = slices.Clone(facet.MissingReceipts)
	cloned.UnresolvedReceipts = slices.Clone(facet.UnresolvedReceipts)
	cloned.Namespaces = make([]PackageNamespaceFacet, len(facet.Namespaces))
	for index, namespace := range facet.Namespaces {
		cloned.Namespaces[index] = namespace
		cloned.Namespaces[index].ExpectedReceipts = slices.Clone(namespace.ExpectedReceipts)
		cloned.Namespaces[index].ObservedReceipts = slices.Clone(namespace.ObservedReceipts)
		cloned.Namespaces[index].MissingReceipts = slices.Clone(namespace.MissingReceipts)
		cloned.Namespaces[index].UnresolvedReceipts = slices.Clone(namespace.UnresolvedReceipts)
	}
	return cloned
}

func cloneDirectFacet(facet DirectFacet) DirectFacet {
	cloned := facet
	cloned.Alternatives = make([]DirectAlternative, len(facet.Alternatives))
	for index, alternative := range facet.Alternatives {
		cloned.Alternatives[index] = alternative
		cloned.Alternatives[index].Identifiers = slices.Clone(alternative.Identifiers)
	}
	return cloned
}

func validInstallability(value Installability) bool {
	return value == InstallabilitySupported || value == InstallabilityUnsupported || value == InstallabilityUnknown
}
func validPackageState(value PackageState) bool {
	return value == PackagePresent || value == PackagePartial || value == PackageMissing || value == PackageUnknown || value == PackageNotApplicable
}
func validComponentState(value ComponentState) bool {
	return value == ComponentPresent || value == ComponentMissing || value == ComponentUnknown || value == ComponentNotApplicable
}
func validDirectKind(value DirectSourceKind) bool {
	return value == DirectSourceBinary || value == DirectSourceAppBundle || value == DirectSourceFlatpak || value == DirectSourceLegacy
}
func validDiagnosticCode(value DiagnosticCode) bool {
	return value == DiagnosticPackageBatchFailed || value == DiagnosticFallbackTimeout || value == DiagnosticCancelled || value == DiagnosticProbeFailed || value == DiagnosticProbeTimeout
}
