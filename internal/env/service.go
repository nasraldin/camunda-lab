package env

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nasraldin/camunda-lab/internal/config"
	"gopkg.in/yaml.v3"
)

// ProfileSource identifies the deterministic origin of a resolved profile.
type ProfileSource string

const (
	ProfileSourceImplicit ProfileSource = "implicit"
	ProfileSourceProject  ProfileSource = "project"
	ProfileSourceGlobal   ProfileSource = "global"
)

// ErrorKind categorizes environment service failures for thin callers.
type ErrorKind string

const (
	ErrorMissing   ErrorKind = "missing"
	ErrorInvalid   ErrorKind = "invalid"
	ErrorConflict  ErrorKind = "conflict"
	ErrorMigration ErrorKind = "migration"
)

// Error is a typed environment service failure.
type Error struct {
	Kind      ErrorKind
	Operation string
	Name      string
	Source    ProfileSource
	Err       error
}

func (e *Error) Error() string {
	target := e.Name
	if target == "" {
		target = "environment state"
	}
	if e.Source != "" {
		target = string(e.Source) + " " + target
	}
	return fmt.Sprintf("%s %s: %v", e.Operation, target, e.Err)
}

func (e *Error) Unwrap() error { return e.Err }

// ResolveRequest requests an explicit profile by name or active-state
// resolution when Name is empty.
type ResolveRequest struct {
	Name        string
	ProjectRoot string
}

// Resolved includes both a profile and its deterministic source.
type Resolved struct {
	Profile Profile       `json:"profile"`
	Source  ProfileSource `json:"source"`
}

// Service owns all project/global profile and active-state operations.
type Service struct {
	labHome string
	hooks   serviceHooks
}

type serviceHooks struct {
	afterProfileDirOpen func()
	crashAfterPhase     func(string) error
}

// NewService creates an environment service rooted at labHome.
func NewService(labHome string) *Service {
	return &Service{labHome: filepath.Clean(labHome)}
}

// List returns the implicit lab and effective named profiles. Project profiles
// shadow same-named global profiles.
func (s *Service) List(projectRoot string) ([]Resolved, error) {
	root, err := canonicalProjectRoot(projectRoot, false)
	if err != nil {
		return nil, serviceError("list", ErrorInvalid, "", ProfileSourceProject, err)
	}
	var out []Resolved
	err = s.withStateLocks(root, func() error {
		if err := s.migrateLegacyLocked(); err != nil {
			return err
		}
		out = []Resolved{{Profile: DefaultLab(), Source: ProfileSourceImplicit}}
		seen := map[string]struct{}{"lab": {}}
		if root != "" {
			profiles, err := s.listProfiles(projectProfilesDir(root))
			if err != nil {
				return serviceError("list", ErrorInvalid, "", ProfileSourceProject, err)
			}
			sort.Slice(profiles, func(i, j int) bool { return profiles[i].Name < profiles[j].Name })
			for _, profile := range profiles {
				seen[profile.Name] = struct{}{}
				out = append(out, Resolved{Profile: profile, Source: ProfileSourceProject})
			}
		}
		profiles, err := s.listProfiles(s.globalProfilesDir())
		if err != nil {
			return serviceError("list", ErrorInvalid, "", ProfileSourceGlobal, err)
		}
		sort.Slice(profiles, func(i, j int) bool { return profiles[i].Name < profiles[j].Name })
		for _, profile := range profiles {
			if _, exists := seen[profile.Name]; exists {
				continue
			}
			out = append(out, Resolved{Profile: profile, Source: ProfileSourceGlobal})
		}
		if root != "" {
			active, err := s.resolveActiveWithoutRegistrationLocked(root)
			if err != nil {
				return err
			}
			if err := s.registerProjectLocked(root, active); err != nil {
				return err
			}
		}
		return nil
	})
	return out, err
}

// Resolve resolves an explicit name, or project active then global active then
// implicit lab when no name is supplied.
func (s *Service) Resolve(request ResolveRequest) (Resolved, error) {
	root, err := canonicalProjectRoot(request.ProjectRoot, false)
	if err != nil {
		return Resolved{}, serviceError("resolve", ErrorInvalid, request.Name, ProfileSourceProject, err)
	}
	var result Resolved
	err = s.withStateLocks(root, func() error {
		if err := s.migrateLegacyLocked(); err != nil {
			return err
		}
		name := request.Name
		if name == "" && root != "" {
			projectName, present, err := config.ReadScalarLocked(projectConfigPath(root), "environment")
			if err != nil {
				return serviceError("resolve", ErrorInvalid, "", ProfileSourceProject, err)
			}
			if present && projectName != "" {
				name = projectName
			}
		}
		if name == "" {
			globalName, _, err := config.ReadScalarLocked(s.globalConfigPath(), "activeEnv")
			if err != nil {
				return serviceError("resolve", ErrorInvalid, "", ProfileSourceGlobal, err)
			}
			name = globalName
		}
		if name == "" || name == "lab" {
			result = Resolved{Profile: DefaultLab(), Source: ProfileSourceImplicit}
			if root != "" {
				if err := s.registerProjectLocked(root, result); err != nil {
					return err
				}
			}
			return nil
		}
		resolved, err := s.resolveNamedLocked(name, root)
		if err != nil {
			return err
		}
		result = resolved
		if root != "" {
			if err := s.registerProjectLocked(root, result); err != nil {
				return err
			}
		}
		return nil
	})
	return result, err
}

// SaveGlobal creates a global profile.
func (s *Service) SaveGlobal(profile Profile) error {
	return s.withStateLocks("", func() error {
		if err := s.migrateLegacyLocked(); err != nil {
			return err
		}
		return s.saveNewProfile(s.globalProfilesDir(), profile, ProfileSourceGlobal)
	})
}

// SaveProject creates a project-local profile.
func (s *Service) SaveProject(projectRoot string, profile Profile) error {
	root, err := canonicalProjectRoot(projectRoot, true)
	if err != nil {
		return serviceError("save", ErrorInvalid, profile.Name, ProfileSourceProject, err)
	}
	return s.withStateLocks(root, func() error {
		if err := s.migrateLegacyLocked(); err != nil {
			return err
		}
		return s.saveNewProfile(projectProfilesDir(root), profile, ProfileSourceProject)
	})
}

// Use validates and selects name in project config when projectRoot is
// supplied, otherwise in global user config.
func (s *Service) Use(name, projectRoot string) (Resolved, error) {
	root, err := canonicalProjectRoot(projectRoot, projectRoot != "")
	if err != nil {
		return Resolved{}, serviceError("use", ErrorInvalid, name, ProfileSourceProject, err)
	}
	var resolved Resolved
	err = s.withStateLocks(root, func() error {
		if err := s.migrateLegacyLocked(); err != nil {
			return err
		}
		if name == "lab" {
			resolved = Resolved{Profile: DefaultLab(), Source: ProfileSourceImplicit}
		} else {
			var err error
			resolved, err = s.resolveNamedLocked(name, root)
			if err != nil {
				return err
			}
		}
		if root != "" {
			if err := config.UpdateScalarLocked(projectConfigPath(root), "environment", name, 0o644); err != nil {
				return serviceError("use", ErrorInvalid, name, ProfileSourceProject, err)
			}
			if err := s.registerProjectLocked(root, resolved); err != nil {
				return err
			}
		} else if err := config.UpdateScalarLocked(s.globalConfigPath(), "activeEnv", name, 0o600); err != nil {
			return serviceError("use", ErrorInvalid, name, ProfileSourceGlobal, err)
		}
		return nil
	})
	return resolved, err
}

// Remove deletes a profile from source and transactionally updates active
// references which would otherwise become dangling.
func (s *Service) Remove(name, projectRoot string, source ProfileSource) error {
	if source != ProfileSourceGlobal && source != ProfileSourceProject {
		return serviceError("remove", ErrorInvalid, name, source, fmt.Errorf("source must be project or global"))
	}
	root, err := canonicalProjectRoot(projectRoot, source == ProfileSourceProject)
	if err != nil {
		return serviceError("remove", ErrorInvalid, name, source, err)
	}
	return s.withStateLocks(root, func() error {
		if err := s.migrateLegacyLocked(); err != nil {
			return err
		}
		return s.removeLocked(name, root, source)
	})
}

func (s *Service) resolveNamedLocked(name, projectRoot string) (Resolved, error) {
	if err := ValidateName(name); err != nil {
		return Resolved{}, serviceError("resolve", ErrorInvalid, name, "", err)
	}
	if projectRoot != "" {
		profile, err := s.loadProfile(projectProfilesDir(projectRoot), name)
		if err == nil {
			return Resolved{Profile: profile, Source: ProfileSourceProject}, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return Resolved{}, serviceError("resolve", ErrorInvalid, name, ProfileSourceProject, err)
		}
	}
	profile, err := s.loadProfile(s.globalProfilesDir(), name)
	if err == nil {
		return Resolved{Profile: profile, Source: ProfileSourceGlobal}, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return Resolved{}, serviceError("resolve", ErrorMissing, name, "", err)
	}
	return Resolved{}, serviceError("resolve", ErrorInvalid, name, ProfileSourceGlobal, err)
}

func (s *Service) saveNewProfile(dir string, profile Profile, source ProfileSource) error {
	if err := profile.Validate(); err != nil {
		return serviceError("save", ErrorInvalid, profile.Name, source, err)
	}
	root, err := openProfileRoot(dir, true, s.hooks.afterProfileDirOpen)
	if err != nil {
		return serviceError("save", ErrorInvalid, profile.Name, source, err)
	}
	defer root.close()
	if err := root.save(profile, false); err != nil {
		if errors.Is(err, os.ErrExist) {
			return serviceError("save", ErrorConflict, profile.Name, source, err)
		}
		return serviceError("save", ErrorInvalid, profile.Name, source, err)
	}
	return nil
}

func (s *Service) removeLocked(name, projectRoot string, source ProfileSource) error {
	if err := ValidateName(name); err != nil {
		return serviceError("remove", ErrorInvalid, name, source, err)
	}
	if source == ProfileSourceGlobal {
		if err := s.globalRemovalConflictsLocked(name); err != nil {
			return err
		}
	}
	dir := s.globalProfilesDir()
	if source == ProfileSourceProject {
		dir = projectProfilesDir(projectRoot)
	}
	root, err := openProfileRoot(dir, false, s.hooks.afterProfileDirOpen)
	if err != nil {
		kind := ErrorInvalid
		if errors.Is(err, os.ErrNotExist) {
			kind = ErrorMissing
		}
		return serviceError("remove", kind, name, source, err)
	}
	defer root.close()
	if _, err := root.load(name); err != nil {
		kind := ErrorInvalid
		if errors.Is(err, os.ErrNotExist) {
			kind = ErrorMissing
		}
		return serviceError("remove", kind, name, source, err)
	}
	profileInfo, err := root.stat(name + ".yaml")
	if err != nil {
		return serviceError("remove", ErrorInvalid, name, source, err)
	}
	globalState, err := config.SnapshotLocked(s.globalConfigPath())
	if err != nil {
		return serviceError("remove", ErrorInvalid, name, ProfileSourceGlobal, err)
	}
	var projectState config.FileState
	if projectRoot != "" {
		projectState, err = config.SnapshotLocked(projectConfigPath(projectRoot))
		if err != nil {
			return serviceError("remove", ErrorInvalid, name, ProfileSourceProject, err)
		}
	}
	journal := removalJournal{
		Version:       1,
		Phase:         phaseIntent,
		Name:          name,
		Source:        source,
		ProjectRoot:   projectRoot,
		ProfileRoot:   dir,
		ProfileName:   name + ".yaml",
		TombstoneName: "." + name + ".removing-" + secureSuffix(),
		GlobalPrior:   journalState(globalState),
		ProjectPrior:  journalState(projectState),
	}
	if err := s.writeJournalLocked(journal); err != nil {
		return serviceError("remove", ErrorInvalid, name, source, err)
	}
	if err := s.crashAfter(phaseIntent); err != nil {
		return err
	}
	if err := root.renameExpected(journal.ProfileName, journal.TombstoneName, profileInfo); err != nil {
		_ = s.recoverJournalUsingRootLocked(root)
		return serviceError("remove", ErrorInvalid, name, source, err)
	}
	journal.Phase = phaseTombstoned
	if err := s.writeJournalLocked(journal); err != nil {
		_ = s.recoverJournalUsingRootLocked(root)
		return serviceError("remove", ErrorInvalid, name, source, err)
	}
	if err := s.crashAfter(phaseTombstoned); err != nil {
		return err
	}
	if source == ProfileSourceGlobal {
		active, _, err := config.ReadScalarLocked(s.globalConfigPath(), "activeEnv")
		if err != nil {
			_ = s.recoverJournalUsingRootLocked(root)
			return err
		}
		if active == name {
			if err := config.UpdateScalarLocked(s.globalConfigPath(), "activeEnv", "lab", 0o600); err != nil {
				_ = s.recoverJournalUsingRootLocked(root)
				return err
			}
		}
	}
	if projectRoot != "" {
		active, _, err := config.ReadScalarLocked(projectConfigPath(projectRoot), "environment")
		if err != nil {
			_ = s.recoverJournalUsingRootLocked(root)
			return err
		}
		var resolvedAfter *Resolved
		if source == ProfileSourceProject && active == name {
			if globalProfile, err := s.loadProfile(s.globalProfilesDir(), name); errors.Is(err, os.ErrNotExist) {
				if err := config.RemoveScalarLocked(projectConfigPath(projectRoot), "environment", 0o644); err != nil {
					_ = s.recoverJournalUsingRootLocked(root)
					return err
				}
			} else if err != nil {
				_ = s.recoverJournalUsingRootLocked(root)
				return err
			} else {
				fallback := Resolved{Profile: globalProfile, Source: ProfileSourceGlobal}
				resolvedAfter = &fallback
			}
		}
		if resolvedAfter == nil {
			resolved, err := s.resolveActiveWithoutRegistrationLocked(projectRoot)
			if err != nil {
				_ = s.recoverJournalUsingRootLocked(root)
				return err
			}
			resolvedAfter = &resolved
		}
		if err := s.registerProjectLocked(projectRoot, *resolvedAfter); err != nil {
			_ = s.recoverJournalUsingRootLocked(root)
			return err
		}
	}
	globalAfter, err := config.SnapshotLocked(s.globalConfigPath())
	if err != nil {
		_ = s.recoverJournalUsingRootLocked(root)
		return err
	}
	journal.GlobalAfter = journalState(globalAfter)
	if projectRoot != "" {
		projectAfter, err := config.SnapshotLocked(projectConfigPath(projectRoot))
		if err != nil {
			_ = s.recoverJournalUsingRootLocked(root)
			return err
		}
		journal.ProjectAfter = journalState(projectAfter)
	}
	journal.Phase = phaseConfigs
	if err := s.writeJournalLocked(journal); err != nil {
		_ = s.recoverJournalUsingRootLocked(root)
		return err
	}
	if err := s.crashAfter(phaseConfigs); err != nil {
		return err
	}
	journal.Phase = phaseCommit
	if err := s.writeJournalLocked(journal); err != nil {
		_ = s.recoverJournalUsingRootLocked(root)
		return err
	}
	if err := s.crashAfter(phaseCommit); err != nil {
		return err
	}
	if err := root.unlink(journal.TombstoneName); err != nil {
		return serviceError("remove", ErrorConflict, name, source, err)
	}
	return s.removeJournalLocked()
}

func (s *Service) migrateLegacyLocked() error {
	legacy := ActiveFile(s.labHome)
	legacyState, err := config.SnapshotLocked(legacy)
	if err != nil {
		return serviceError("migrate", ErrorMigration, "", ProfileSourceGlobal, err)
	}
	if !legacyState.Exists {
		return nil
	}
	active, present, err := config.ReadScalarLocked(s.globalConfigPath(), "activeEnv")
	if err != nil {
		return serviceError("migrate", ErrorMigration, "", ProfileSourceGlobal, err)
	}
	if present {
		if err := config.RestoreLocked(legacy, config.FileState{}, 0o600); err != nil {
			return serviceError("migrate", ErrorMigration, active, ProfileSourceGlobal, err)
		}
		return nil
	}
	name := strings.TrimSuffix(string(legacyState.Data), "\n")
	if name != "lab" {
		if err := ValidateName(name); err != nil {
			return serviceError("migrate", ErrorMigration, name, ProfileSourceGlobal, err)
		}
		if _, err := s.loadProfile(s.globalProfilesDir(), name); err != nil {
			return serviceError("migrate", ErrorMigration, name, ProfileSourceGlobal, err)
		}
	}
	if err := config.UpdateScalarLocked(s.globalConfigPath(), "activeEnv", name, 0o600); err != nil {
		return serviceError("migrate", ErrorMigration, name, ProfileSourceGlobal, err)
	}
	if err := config.RestoreLocked(legacy, config.FileState{}, 0o600); err != nil {
		return serviceError("migrate", ErrorMigration, name, ProfileSourceGlobal, err)
	}
	return nil
}

const (
	phaseIntent     = "intent"
	phaseTombstoned = "tombstoned"
	phaseConfigs    = "configs"
	phaseCommit     = "commit"
)

type removalJournal struct {
	Version       int           `yaml:"version"`
	Phase         string        `yaml:"phase"`
	Name          string        `yaml:"name"`
	Source        ProfileSource `yaml:"source"`
	ProjectRoot   string        `yaml:"projectRoot,omitempty"`
	ProfileRoot   string        `yaml:"profileRoot"`
	ProfileName   string        `yaml:"profileName"`
	TombstoneName string        `yaml:"tombstoneName"`
	GlobalPrior   journalFile   `yaml:"globalPrior"`
	ProjectPrior  journalFile   `yaml:"projectPrior"`
	GlobalAfter   journalFile   `yaml:"globalAfter,omitempty"`
	ProjectAfter  journalFile   `yaml:"projectAfter,omitempty"`
}

type journalFile struct {
	Exists bool   `yaml:"exists"`
	Data   []byte `yaml:"data,omitempty"`
	Mode   uint32 `yaml:"mode,omitempty"`
}

func journalState(state config.FileState) journalFile {
	return journalFile{Exists: state.Exists, Data: state.Data, Mode: uint32(state.Mode.Perm())}
}

func (state journalFile) configState() config.FileState {
	return config.FileState{Exists: state.Exists, Data: state.Data, Mode: os.FileMode(state.Mode)}
}

func (s *Service) journalPath() string {
	return filepath.Join(s.labHome, ".env-remove-journal.yaml")
}

func (s *Service) writeJournalLocked(journal removalJournal) error {
	data, err := yaml.Marshal(journal)
	if err != nil {
		return err
	}
	return config.WriteLocked(s.journalPath(), data, 0o600)
}

func (s *Service) removeJournalLocked() error {
	return config.RestoreLocked(s.journalPath(), config.FileState{}, 0o600)
}

func (s *Service) readJournalLocked() (*removalJournal, error) {
	state, err := config.SnapshotLocked(s.journalPath())
	if err != nil || !state.Exists {
		return nil, err
	}
	if state.Mode.Perm() != 0o600 {
		return nil, fmt.Errorf("removal journal permissions must be 0600")
	}
	var journal removalJournal
	decoder := yaml.NewDecoder(bytes.NewReader(state.Data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&journal); err != nil {
		return nil, fmt.Errorf("parse removal journal: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("removal journal must contain one document")
	}
	if journal.Version != 1 || journal.Name == "" || journal.ProfileRoot == "" ||
		journal.ProfileName == "" || journal.TombstoneName == "" {
		return nil, fmt.Errorf("invalid removal journal")
	}
	if err := ValidateName(journal.Name); err != nil {
		return nil, fmt.Errorf("invalid removal journal name: %w", err)
	}
	if journal.ProfileName != journal.Name+".yaml" ||
		filepath.Base(journal.TombstoneName) != journal.TombstoneName ||
		!strings.HasPrefix(journal.TombstoneName, "."+journal.Name+".removing-") {
		return nil, fmt.Errorf("invalid removal journal paths")
	}
	expectedRoot := s.globalProfilesDir()
	if journal.Source == ProfileSourceProject {
		root, err := canonicalProjectRoot(journal.ProjectRoot, true)
		if err != nil {
			return nil, err
		}
		expectedRoot = projectProfilesDir(root)
	} else if journal.Source != ProfileSourceGlobal {
		return nil, fmt.Errorf("invalid removal journal source")
	}
	if filepath.Clean(journal.ProfileRoot) != filepath.Clean(expectedRoot) {
		return nil, fmt.Errorf("invalid removal journal profile root")
	}
	switch journal.Phase {
	case phaseIntent, phaseTombstoned, phaseConfigs, phaseCommit:
	default:
		return nil, fmt.Errorf("invalid removal journal phase %q", journal.Phase)
	}
	if err := validateJournalFile("globalPrior", journal.GlobalPrior, true, true); err != nil {
		return nil, err
	}
	if journal.Source == ProfileSourceProject {
		if err := validateJournalFile("projectPrior", journal.ProjectPrior, true, false); err != nil {
			return nil, err
		}
	} else if journal.ProjectRoot != "" || journal.ProjectPrior.Exists ||
		len(journal.ProjectPrior.Data) != 0 || journal.ProjectPrior.Mode != 0 {
		return nil, fmt.Errorf("global removal journal contains project state")
	}
	if journal.Phase == phaseIntent || journal.Phase == phaseTombstoned {
		if !zeroJournalFile(journal.GlobalAfter) || !zeroJournalFile(journal.ProjectAfter) {
			return nil, fmt.Errorf("%s journal must not contain post-config state", journal.Phase)
		}
	} else {
		if err := validateJournalFile("globalAfter", journal.GlobalAfter, true, true); err != nil {
			return nil, err
		}
		if journal.Source == ProfileSourceProject {
			if err := validateJournalFile("projectAfter", journal.ProjectAfter, true, false); err != nil {
				return nil, err
			}
		} else if !zeroJournalFile(journal.ProjectAfter) {
			return nil, fmt.Errorf("global removal journal contains project post-state")
		}
	}
	return &journal, nil
}

func zeroJournalFile(state journalFile) bool {
	return !state.Exists && len(state.Data) == 0 && state.Mode == 0
}

func validateJournalFile(name string, state journalFile, required, global bool) error {
	if !state.Exists {
		if required || len(state.Data) != 0 || state.Mode != 0 {
			return fmt.Errorf("invalid %s snapshot", name)
		}
		return nil
	}
	if len(state.Data) == 0 || state.Mode == 0 || state.Mode&^0o777 != 0 {
		return fmt.Errorf("invalid %s snapshot", name)
	}
	if global && state.Mode != 0o600 {
		return fmt.Errorf("invalid %s permissions", name)
	}
	if !global && state.Mode&0o133 != 0 {
		return fmt.Errorf("invalid %s permissions", name)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(state.Data, &document); err != nil ||
		len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("invalid %s config data", name)
	}
	if err := validateConfigMapping(document.Content[0], global); err != nil {
		return fmt.Errorf("invalid %s config data: %w", name, err)
	}
	return nil
}

func (s *Service) recoverJournalLocked() error {
	journal, err := s.readJournalLocked()
	if err != nil || journal == nil {
		return err
	}
	root, err := openProfileRoot(journal.ProfileRoot, false, s.hooks.afterProfileDirOpen)
	if err != nil {
		return serviceError("recover", ErrorConflict, journal.Name, journal.Source, err)
	}
	defer root.close()
	return s.recoverJournalUsingRootLocked(root)
}

func (s *Service) recoverJournalUsingRootLocked(root *profileRoot) error {
	journal, err := s.readJournalLocked()
	if err != nil || journal == nil {
		return err
	}
	if filepath.Clean(journal.ProfileRoot) != filepath.Clean(root.path) {
		return serviceError("recover", ErrorConflict, journal.Name, journal.Source, fmt.Errorf("journal profile root mismatch"))
	}
	if err := s.validateJournalPhaseStateLocked(root, *journal); err != nil {
		return serviceError("recover", ErrorConflict, journal.Name, journal.Source, err)
	}
	if journal.Phase == phaseCommit {
		if err := root.unlink(journal.TombstoneName); err != nil {
			return serviceError("recover", ErrorConflict, journal.Name, journal.Source, err)
		}
		return s.removeJournalLocked()
	}
	if err := config.RestoreLocked(s.globalConfigPath(), journal.GlobalPrior.configState(), 0o600); err != nil {
		return serviceError("recover", ErrorConflict, journal.Name, journal.Source, err)
	}
	if journal.ProjectRoot != "" {
		if err := config.RestoreLocked(projectConfigPath(journal.ProjectRoot), journal.ProjectPrior.configState(), 0o644); err != nil {
			return serviceError("recover", ErrorConflict, journal.Name, journal.Source, err)
		}
	}
	tombExists, err := root.exists(journal.TombstoneName)
	if err != nil {
		return err
	}
	if tombExists {
		profileExists, err := root.exists(journal.ProfileName)
		if err != nil {
			return err
		}
		if profileExists {
			return serviceError("recover", ErrorConflict, journal.Name, journal.Source, fmt.Errorf("profile and tombstone both exist"))
		}
		if err := root.rename(journal.TombstoneName, journal.ProfileName); err != nil {
			return serviceError("recover", ErrorConflict, journal.Name, journal.Source, err)
		}
	}
	return s.removeJournalLocked()
}

func (s *Service) validateJournalPhaseStateLocked(root *profileRoot, journal removalJournal) error {
	profileExists, err := root.exists(journal.ProfileName)
	if err != nil {
		return err
	}
	tombExists, err := root.exists(journal.TombstoneName)
	if err != nil {
		return err
	}
	switch journal.Phase {
	case phaseIntent:
		if !profileExists || tombExists {
			return fmt.Errorf("intent phase requires profile and no tombstone")
		}
	case phaseTombstoned, phaseConfigs:
		if profileExists || !tombExists {
			return fmt.Errorf("%s phase requires tombstone and no profile", journal.Phase)
		}
	case phaseCommit:
		if profileExists {
			return fmt.Errorf("commit phase must not contain profile")
		}
	default:
		return fmt.Errorf("invalid phase")
	}
	if journal.Phase == phaseIntent || journal.Phase == phaseTombstoned {
		global, err := config.SnapshotLocked(s.globalConfigPath())
		if err != nil {
			return err
		}
		if !sameConfigState(global, journal.GlobalPrior.configState()) {
			return fmt.Errorf("%s phase global config differs from prior state", journal.Phase)
		}
		if journal.Source == ProfileSourceProject {
			project, err := config.SnapshotLocked(projectConfigPath(journal.ProjectRoot))
			if err != nil {
				return err
			}
			if !sameConfigState(project, journal.ProjectPrior.configState()) {
				return fmt.Errorf("%s phase project config differs from prior state", journal.Phase)
			}
		}
	} else {
		global, err := config.SnapshotLocked(s.globalConfigPath())
		if err != nil || !sameConfigState(global, journal.GlobalAfter.configState()) {
			return fmt.Errorf("%s phase global config does not match journal", journal.Phase)
		}
		if journal.Source == ProfileSourceProject {
			project, err := config.SnapshotLocked(projectConfigPath(journal.ProjectRoot))
			if err != nil || !sameConfigState(project, journal.ProjectAfter.configState()) {
				return fmt.Errorf("%s phase project config does not match journal", journal.Phase)
			}
		}
	}
	return nil
}

func validateConfigMapping(mapping *yaml.Node, global bool) error {
	if global {
		active, present, err := nodeScalar(mapping, "activeEnv")
		if err != nil {
			return err
		}
		if present && active != "lab" {
			if err := ValidateName(active); err != nil {
				return err
			}
		}
		references, err := mappingChild(mapping, "environmentReferences", false)
		if err != nil {
			return err
		}
		if references != nil {
			if _, _, err := parseReferenceMetadata(references); err != nil {
				return err
			}
		}
		return nil
	}
	environment, present, err := nodeScalar(mapping, "environment")
	if err != nil {
		return err
	}
	if present && environment != "lab" {
		return ValidateName(environment)
	}
	return nil
}

func sameConfigState(left, right config.FileState) bool {
	return left.Exists == right.Exists && left.Mode.Perm() == right.Mode.Perm() && bytes.Equal(left.Data, right.Data)
}

func (s *Service) crashAfter(phase string) error {
	if s.hooks.crashAfterPhase == nil {
		return nil
	}
	return s.hooks.crashAfterPhase(phase)
}

func (s *Service) resolveActiveWithoutRegistrationLocked(projectRoot string) (Resolved, error) {
	name, present, err := config.ReadScalarLocked(projectConfigPath(projectRoot), "environment")
	if err != nil {
		return Resolved{}, err
	}
	if !present || name == "" {
		name, _, err = config.ReadScalarLocked(s.globalConfigPath(), "activeEnv")
		if err != nil {
			return Resolved{}, err
		}
	}
	if name == "" || name == "lab" {
		return Resolved{Profile: DefaultLab(), Source: ProfileSourceImplicit}, nil
	}
	return s.resolveNamedLocked(name, projectRoot)
}

func (s *Service) globalProfilesDir() string { return filepath.Join(s.labHome, "envs") }
func (s *Service) globalConfigPath() string  { return filepath.Join(s.labHome, "config.yaml") }

func projectProfilesDir(root string) string { return filepath.Join(root, "environments") }
func projectConfigPath(root string) string  { return filepath.Join(root, ".camunda.yaml") }

func (s *Service) listProfiles(dir string) ([]Profile, error) {
	root, err := openProfileRoot(dir, false, s.hooks.afterProfileDirOpen)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer root.close()
	return root.list()
}

func (s *Service) loadProfile(dir, name string) (Profile, error) {
	root, err := openProfileRoot(dir, false, s.hooks.afterProfileDirOpen)
	if err != nil {
		return Profile{}, err
	}
	defer root.close()
	return root.load(name)
}

func (s *Service) registerProjectLocked(root string, resolved Resolved) error {
	identity := projectIdentity(root)
	return config.UpdateNodeLocked(s.globalConfigPath(), 0o600, func(mapping *yaml.Node) error {
		references, err := mappingChild(mapping, "environmentReferences", false)
		if err != nil {
			return err
		}
		if references == nil {
			references, err = mappingChild(mapping, "environmentReferences", true)
			if err != nil {
				return err
			}
			setYAMLScalar(references, "complete", "false", "!!bool")
			projects := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			references.Content = append(references.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "projects"},
				projects,
			)
		} else if _, _, err := parseReferenceMetadata(references); err != nil {
			return serviceError("register", ErrorInvalid, resolved.Profile.Name, resolved.Source, err)
		}
		if references.Kind != yaml.MappingNode {
			return fmt.Errorf("environmentReferences must be a mapping")
		}
		projects, err := mappingChild(references, "projects", true)
		if err != nil {
			return err
		}
		if projects.Kind != yaml.MappingNode {
			return fmt.Errorf("environmentReferences.projects must be a mapping")
		}
		entry, err := mappingChild(projects, identity, true)
		if err != nil {
			return err
		}
		entry.Kind, entry.Tag = yaml.MappingNode, "!!map"
		entry.Content = nil
		setYAMLScalar(entry, "root", root, "!!str")
		setYAMLScalar(entry, "environment", resolved.Profile.Name, "!!str")
		setYAMLScalar(entry, "source", string(resolved.Source), "!!str")
		return nil
	})
}

func (s *Service) globalRemovalConflictsLocked(name string) error {
	state, err := config.SnapshotLocked(s.globalConfigPath())
	if err != nil {
		return err
	}
	if !state.Exists {
		return serviceError("remove", ErrorConflict, name, ProfileSourceGlobal, fmt.Errorf("project reference completeness is unknown"))
	}
	var document yaml.Node
	if err := yaml.Unmarshal(state.Data, &document); err != nil {
		return serviceError("remove", ErrorInvalid, name, ProfileSourceGlobal, err)
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return serviceError("remove", ErrorInvalid, name, ProfileSourceGlobal, fmt.Errorf("config root must be a mapping"))
	}
	mapping := document.Content[0]
	references, err := mappingChild(mapping, "environmentReferences", false)
	if err != nil {
		return serviceError("remove", ErrorInvalid, name, ProfileSourceGlobal, err)
	}
	if references == nil {
		return serviceError("remove", ErrorConflict, name, ProfileSourceGlobal, fmt.Errorf("project reference completeness is unknown"))
	}
	complete, records, err := parseReferenceMetadata(references)
	if err != nil {
		return serviceError("remove", ErrorInvalid, name, ProfileSourceGlobal, err)
	}
	if !complete {
		return serviceError("remove", ErrorConflict, name, ProfileSourceGlobal, fmt.Errorf("project reference completeness is unknown"))
	}
	var affected []string
	for _, record := range records {
		if record.environment == name && record.source == ProfileSourceGlobal {
			affected = append(affected, record.root)
		}
	}
	if len(affected) > 0 {
		sort.Strings(affected)
		return serviceError("remove", ErrorConflict, name, ProfileSourceGlobal, fmt.Errorf("referenced by projects: %s", strings.Join(affected, ", ")))
	}
	return nil
}

type referenceRecord struct {
	identity    string
	root        string
	environment string
	source      ProfileSource
}

func parseReferenceMetadata(references *yaml.Node) (bool, []referenceRecord, error) {
	if references == nil || references.Kind != yaml.MappingNode || references.Tag != "!!map" {
		return false, nil, fmt.Errorf("environmentReferences must be a mapping")
	}
	fields, err := strictMapping(references, map[string]bool{"complete": true, "projects": true})
	if err != nil {
		return false, nil, err
	}
	completeNode := fields["complete"]
	if completeNode.Kind != yaml.ScalarNode || completeNode.Tag != "!!bool" ||
		(completeNode.Value != "true" && completeNode.Value != "false") {
		return false, nil, fmt.Errorf("environmentReferences.complete must be a YAML boolean")
	}
	projects := fields["projects"]
	if projects.Kind != yaml.MappingNode || projects.Tag != "!!map" {
		return false, nil, fmt.Errorf("environmentReferences.projects must be a mapping")
	}
	seenRoots := make(map[string]struct{}, len(projects.Content)/2)
	seenIdentities := make(map[string]struct{}, len(projects.Content)/2)
	records := make([]referenceRecord, 0, len(projects.Content)/2)
	for i := 0; i < len(projects.Content); i += 2 {
		key := projects.Content[i]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
			return false, nil, fmt.Errorf("project reference identity must be a string")
		}
		identity := key.Value
		decoded, err := hex.DecodeString(identity)
		if err != nil || len(decoded) != 16 || strings.ToLower(identity) != identity {
			return false, nil, fmt.Errorf("invalid project reference identity %q", identity)
		}
		if _, duplicate := seenIdentities[identity]; duplicate {
			return false, nil, fmt.Errorf("duplicate project reference identity %q", identity)
		}
		seenIdentities[identity] = struct{}{}
		entry := projects.Content[i+1]
		fields, err := strictMapping(entry, map[string]bool{"root": true, "environment": true, "source": true})
		if err != nil {
			return false, nil, fmt.Errorf("project reference %s: %w", identity, err)
		}
		root, err := strictString(fields["root"], "root")
		if err != nil || !filepath.IsAbs(root) || filepath.Clean(root) != root {
			return false, nil, fmt.Errorf("project reference %s has invalid root", identity)
		}
		if projectIdentity(root) != identity {
			return false, nil, fmt.Errorf("project reference identity does not match root %q", root)
		}
		if canonical, err := filepath.EvalSymlinks(root); err == nil {
			if canonical != root {
				return false, nil, fmt.Errorf("project reference root %q is not canonical", root)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return false, nil, fmt.Errorf("project reference root %q cannot be validated", root)
		}
		if _, duplicate := seenRoots[root]; duplicate {
			return false, nil, fmt.Errorf("duplicate project reference root %q", root)
		}
		seenRoots[root] = struct{}{}
		environment, err := strictString(fields["environment"], "environment")
		if err != nil {
			return false, nil, err
		}
		if environment != "lab" {
			if err := ValidateName(environment); err != nil {
				return false, nil, fmt.Errorf("invalid referenced environment: %w", err)
			}
		}
		sourceValue, err := strictString(fields["source"], "source")
		if err != nil {
			return false, nil, err
		}
		source := ProfileSource(sourceValue)
		if source != ProfileSourceImplicit && source != ProfileSourceProject && source != ProfileSourceGlobal {
			return false, nil, fmt.Errorf("invalid project reference source %q", source)
		}
		if (environment == "lab") != (source == ProfileSourceImplicit) {
			return false, nil, fmt.Errorf("project reference source does not match environment")
		}
		records = append(records, referenceRecord{
			identity: identity, root: root, environment: environment, source: source,
		})
	}
	return completeNode.Value == "true", records, nil
}

func strictMapping(node *yaml.Node, allowed map[string]bool) (map[string]*yaml.Node, error) {
	if node == nil || node.Kind != yaml.MappingNode || node.Tag != "!!map" {
		return nil, fmt.Errorf("expected mapping")
	}
	fields := make(map[string]*yaml.Node, len(node.Content)/2)
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
			return nil, fmt.Errorf("mapping key must be string")
		}
		if !allowed[key.Value] {
			return nil, fmt.Errorf("unknown field %q", key.Value)
		}
		if _, duplicate := fields[key.Value]; duplicate {
			return nil, fmt.Errorf("duplicate field %q", key.Value)
		}
		fields[key.Value] = node.Content[i+1]
	}
	for key, required := range allowed {
		if required && fields[key] == nil {
			return nil, fmt.Errorf("missing field %q", key)
		}
	}
	return fields, nil
}

func strictString(node *yaml.Node, field string) (string, error) {
	if node == nil || node.Kind != yaml.ScalarNode || node.Tag != "!!str" || node.Value == "" {
		return "", fmt.Errorf("%s must be a non-empty string", field)
	}
	return node.Value, nil
}

func projectIdentity(root string) string {
	sum := sha256.Sum256([]byte(root))
	return hex.EncodeToString(sum[:16])
}

func mappingChild(mapping *yaml.Node, key string, create bool) (*yaml.Node, error) {
	var found *yaml.Node
	for i := 0; i < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value != key {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("duplicate %s field", key)
		}
		found = mapping.Content[i+1]
	}
	if found == nil && create {
		found = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		mapping.Content = append(mapping.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
			found,
		)
	}
	return found, nil
}

func nodeScalar(mapping *yaml.Node, key string) (string, bool, error) {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return "", false, fmt.Errorf("%s parent must be a mapping", key)
	}
	var value string
	found := false
	for i := 0; i < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value != key {
			continue
		}
		if found {
			return "", false, fmt.Errorf("duplicate %s field", key)
		}
		if mapping.Content[i+1].Kind != yaml.ScalarNode || mapping.Content[i+1].Tag != "!!str" {
			return "", false, fmt.Errorf("%s must be a string", key)
		}
		value, found = mapping.Content[i+1].Value, true
	}
	return value, found, nil
}

func setYAMLScalar(mapping *yaml.Node, key, value, tag string) {
	for i := 0; i < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content[i+1] = &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value}
			return
		}
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value},
	)
}

func canonicalProjectRoot(root string, required bool) (string, error) {
	if root == "" {
		if required {
			return "", fmt.Errorf("project root is required")
		}
		return "", nil
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("project root is not a directory")
	}
	configPath := filepath.Join(resolved, ".camunda.yaml")
	if info, err := os.Lstat(configPath); err != nil {
		return "", err
	} else if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", fmt.Errorf("project config must be a regular non-symlink file")
	}
	return resolved, nil
}

func serviceError(operation string, kind ErrorKind, name string, source ProfileSource, err error) error {
	return &Error{Kind: kind, Operation: operation, Name: name, Source: source, Err: err}
}

func (s *Service) withStateLocks(projectRoot string, fn func() error) error {
	globalPath := s.globalConfigPath()
	return config.WithLocks([]string{globalPath}, func() error {
		journal, err := s.readJournalLocked()
		if err != nil {
			return serviceError("recover", ErrorConflict, "", "", err)
		}
		roots := make(map[string]struct{})
		if projectRoot != "" {
			roots[projectRoot] = struct{}{}
		}
		if journal != nil && journal.ProjectRoot != "" {
			recoveredRoot, err := canonicalProjectRoot(journal.ProjectRoot, true)
			if err != nil {
				return serviceError("recover", ErrorConflict, journal.Name, journal.Source, err)
			}
			roots[recoveredRoot] = struct{}{}
		}
		orderedRoots := make([]string, 0, len(roots))
		for root := range roots {
			orderedRoots = append(orderedRoots, root)
		}
		sort.Strings(orderedRoots)
		projectPaths := make([]string, 0, len(orderedRoots))
		for _, root := range orderedRoots {
			projectPaths = append(projectPaths, projectConfigPath(root))
		}
		return config.WithLocks(projectPaths, func() error {
			if err := s.recoverJournalLocked(); err != nil {
				return err
			}
			return fn()
		})
	})
}
