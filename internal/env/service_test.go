package env

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestServiceResolveProjectOverridesGlobal(t *testing.T) {
	home, projectRoot := serviceFixture(t)
	service := NewService(home)
	if err := service.SaveGlobal(remoteProfile("shared", "https://global.example")); err != nil {
		t.Fatal(err)
	}
	if err := service.SaveProject(projectRoot, remoteProfile("shared", "https://project.example")); err != nil {
		t.Fatal(err)
	}

	resolved, err := service.Resolve(ResolveRequest{Name: "shared", ProjectRoot: projectRoot})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Source != ProfileSourceProject {
		t.Fatalf("source = %q, want %q", resolved.Source, ProfileSourceProject)
	}
	if got := resolved.Profile.Endpoints["orchestration"]; got != "https://project.example" {
		t.Fatalf("orchestration = %q", got)
	}
}

func TestServiceResolveActivePrecedenceAndMetadata(t *testing.T) {
	home, projectRoot := serviceFixture(t)
	service := NewService(home)
	if err := service.SaveGlobal(remoteProfile("global", "https://global.example")); err != nil {
		t.Fatal(err)
	}
	if err := service.SaveProject(projectRoot, remoteProfile("project", "https://project.example")); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Use("global", ""); err != nil {
		t.Fatal(err)
	}

	global, err := service.Resolve(ResolveRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if global.Profile.Name != "global" || global.Source != ProfileSourceGlobal {
		t.Fatalf("global resolution = %+v", global)
	}

	project, err := service.Use("project", projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	if project.Source != ProfileSourceProject {
		t.Fatalf("use source = %q", project.Source)
	}
	active, err := service.Resolve(ResolveRequest{ProjectRoot: projectRoot})
	if err != nil {
		t.Fatal(err)
	}
	if active.Profile.Name != "project" || active.Source != ProfileSourceProject {
		t.Fatalf("project resolution = %+v", active)
	}
}

func TestServiceResolveDefaultsToImplicitLab(t *testing.T) {
	home, _ := serviceFixture(t)
	resolved, err := NewService(home).Resolve(ResolveRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Profile.Name != "lab" || resolved.Source != ProfileSourceImplicit {
		t.Fatalf("resolution = %+v", resolved)
	}
}

func TestServiceUseValidatesTargetBeforeChangingConfig(t *testing.T) {
	home, projectRoot := serviceFixture(t)
	service := NewService(home)
	before := readServiceFile(t, filepath.Join(projectRoot, ".camunda.yaml"))

	_, err := service.Use("missing", projectRoot)
	var typed *Error
	if !errors.As(err, &typed) || typed.Kind != ErrorMissing {
		t.Fatalf("error = %v, want missing typed error", err)
	}
	after := readServiceFile(t, filepath.Join(projectRoot, ".camunda.yaml"))
	if after != before {
		t.Fatalf("project config changed on failed use:\n%s", after)
	}
}

func TestServiceMigratesLegacyActiveOnceWithoutOverwritingNewConfig(t *testing.T) {
	t.Run("successful migration", func(t *testing.T) {
		home, _ := serviceFixture(t)
		service := NewService(home)
		if err := service.SaveGlobal(remoteProfile("prod", "https://prod.example")); err != nil {
			t.Fatal(err)
		}
		legacy := filepath.Join(home, "active-env")
		if err := os.WriteFile(legacy, []byte("prod\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		got, err := service.Resolve(ResolveRequest{})
		if err != nil {
			t.Fatal(err)
		}
		if got.Profile.Name != "prod" || got.Source != ProfileSourceGlobal {
			t.Fatalf("resolution = %+v", got)
		}
		if _, err := os.Stat(legacy); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("legacy pointer still exists: %v", err)
		}
		config := readServiceFile(t, filepath.Join(home, "config.yaml"))
		if !strings.Contains(config, "activeEnv: prod") || !strings.Contains(config, "custom: keep") {
			t.Fatalf("config not migrated without data loss:\n%s", config)
		}
	})

	t.Run("new config wins", func(t *testing.T) {
		home, _ := serviceFixture(t)
		service := NewService(home)
		for _, name := range []string{"old", "new"} {
			if err := service.SaveGlobal(remoteProfile(name, "https://"+name+".example")); err != nil {
				t.Fatal(err)
			}
		}
		configPath := filepath.Join(home, "config.yaml")
		if err := os.WriteFile(configPath, []byte("version: \"8.9\"\nactiveEnv: new\ncustom: keep\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		legacy := filepath.Join(home, "active-env")
		if err := os.WriteFile(legacy, []byte("old\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		got, err := service.Resolve(ResolveRequest{})
		if err != nil {
			t.Fatal(err)
		}
		if got.Profile.Name != "new" {
			t.Fatalf("active = %q, want new", got.Profile.Name)
		}
		if _, err := os.Stat(legacy); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("superseded legacy pointer was not retired: %v", err)
		}
	})
}

func TestServiceMigrationRejectsInvalidOrMissingTarget(t *testing.T) {
	tests := []struct {
		name  string
		value string
		kind  ErrorKind
	}{
		{name: "invalid", value: "../escape\n", kind: ErrorMigration},
		{name: "missing", value: "ghost\n", kind: ErrorMigration},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			home, _ := serviceFixture(t)
			legacy := filepath.Join(home, "active-env")
			if err := os.WriteFile(legacy, []byte(test.value), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := NewService(home).Resolve(ResolveRequest{})
			var typed *Error
			if !errors.As(err, &typed) || typed.Kind != test.kind {
				t.Fatalf("error = %v, want migration error", err)
			}
			if _, statErr := os.Stat(legacy); statErr != nil {
				t.Fatalf("failed migration removed legacy state: %v", statErr)
			}
		})
	}
}

func TestServiceSaveProjectPersistsReferencesNotSecretValues(t *testing.T) {
	home, projectRoot := serviceFixture(t)
	service := NewService(home)
	profile := remoteProfile("prod", "https://prod.example")
	profile.Auth.ClientSecretEnv = "CAMUNDA_PROD_SECRET"
	if err := service.SaveProject(projectRoot, profile); err != nil {
		t.Fatal(err)
	}
	data := readServiceFile(t, filepath.Join(projectRoot, "environments", "prod.yaml"))
	if !strings.Contains(data, "clientSecretEnv: CAMUNDA_PROD_SECRET") {
		t.Fatalf("secret reference missing:\n%s", data)
	}
	if strings.Contains(data, "clientSecret:") {
		t.Fatalf("inline secret field persisted:\n%s", data)
	}
}

func TestEnvironmentUpdatesPreserveNestedConfigData(t *testing.T) {
	home, root := serviceFixture(t)
	globalPath := filepath.Join(home, "config.yaml")
	if err := os.WriteFile(globalPath, []byte("version: \"8.9\"\nai:\n  # keep global nested\n  enabled: false\n  future: keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	projectPath := filepath.Join(root, ".camunda.yaml")
	if err := os.WriteFile(projectPath, []byte("name: fixture\npaths:\n  # keep project nested\n  bpmn: bpmn\n  future: keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	service := NewService(home)
	if err := service.SaveGlobal(remoteProfile("prod", "https://prod.example")); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Use("prod", root); err != nil {
		t.Fatal(err)
	}
	if text := readServiceFile(t, globalPath); !strings.Contains(text, "# keep global nested") || !strings.Contains(text, "future: keep") {
		t.Fatalf("global nested config lost:\n%s", text)
	}
	if text := readServiceFile(t, projectPath); !strings.Contains(text, "# keep project nested") || !strings.Contains(text, "future: keep") {
		t.Fatalf("project nested config lost:\n%s", text)
	}
}

func TestServiceRemoveActiveGlobalFallsBackAtomically(t *testing.T) {
	home, _ := serviceFixtureWithCompleteReferences(t)
	service := NewService(home)
	if err := service.SaveGlobal(remoteProfile("prod", "https://prod.example")); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Use("prod", ""); err != nil {
		t.Fatal(err)
	}

	if err := service.Remove("prod", "", ProfileSourceGlobal); err != nil {
		t.Fatal(err)
	}
	global, err := service.Resolve(ResolveRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if global.Source != ProfileSourceImplicit {
		t.Fatalf("global fallback = %+v", global)
	}
}

func TestServiceListUsesDeterministicPrecedence(t *testing.T) {
	home, projectRoot := serviceFixture(t)
	service := NewService(home)
	if err := service.SaveGlobal(remoteProfile("shared", "https://global.example")); err != nil {
		t.Fatal(err)
	}
	if err := service.SaveGlobal(remoteProfile("zeta", "https://zeta.example")); err != nil {
		t.Fatal(err)
	}
	if err := service.SaveProject(projectRoot, remoteProfile("shared", "https://project.example")); err != nil {
		t.Fatal(err)
	}
	got, err := service.List(projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 ||
		got[0].Profile.Name != "lab" || got[0].Source != ProfileSourceImplicit ||
		got[1].Profile.Name != "shared" || got[1].Source != ProfileSourceProject ||
		got[2].Profile.Name != "zeta" || got[2].Source != ProfileSourceGlobal {
		t.Fatalf("list = %+v", got)
	}
}

func TestServiceRejectsUnsafeProfileRootsEverywhere(t *testing.T) {
	operations := []struct {
		name string
		run  func(*Service, string) error
	}{
		{name: "list", run: func(service *Service, root string) error {
			_, err := service.List(root)
			return err
		}},
		{name: "resolve", run: func(service *Service, root string) error {
			_, err := service.Resolve(ResolveRequest{Name: "prod", ProjectRoot: root})
			return err
		}},
		{name: "save", run: func(service *Service, root string) error {
			return service.SaveProject(root, remoteProfile("other", "https://other.example"))
		}},
		{name: "use", run: func(service *Service, root string) error {
			_, err := service.Use("prod", root)
			return err
		}},
		{name: "remove", run: func(service *Service, root string) error {
			return service.Remove("prod", root, ProfileSourceProject)
		}},
	}
	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			home, root := serviceFixture(t)
			outside := t.TempDir()
			if err := SaveProfile(outside, remoteProfile("prod", "https://outside.example")); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(root, "environments")); err != nil {
				t.Fatal(err)
			}
			err := operation.run(NewService(home), root)
			var typed *Error
			if !errors.As(err, &typed) || typed.Kind != ErrorInvalid {
				t.Fatalf("error = %v, want invalid unsafe-root error", err)
			}
		})
	}
}

func TestServiceRejectsProfileRootSwapBeforeMutation(t *testing.T) {
	home, root := serviceFixture(t)
	service := NewService(home)
	original := filepath.Join(root, "environments")
	if err := os.Mkdir(original, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	var called atomic.Bool
	service.hooks.afterProfileDirOpen = func() {
		if !called.CompareAndSwap(false, true) {
			return
		}
		if err := os.Rename(original, original+".old"); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, original); err != nil {
			t.Fatal(err)
		}
	}
	err := service.SaveProject(root, remoteProfile("prod", "https://prod.example"))
	if err == nil {
		t.Fatal("save succeeded after profile directory swap")
	}
	if _, err := os.Stat(filepath.Join(outside, "prod.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("outside directory was modified: %v", err)
	}
}

func TestServiceRemoveProjectShadowRevealsGlobal(t *testing.T) {
	home, root := serviceFixture(t)
	service := NewService(home)
	if err := service.SaveGlobal(remoteProfile("shared", "https://global.example")); err != nil {
		t.Fatal(err)
	}
	if err := service.SaveProject(root, remoteProfile("shared", "https://project.example")); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Use("shared", root); err != nil {
		t.Fatal(err)
	}
	if err := service.Remove("shared", root, ProfileSourceProject); err != nil {
		t.Fatal(err)
	}
	resolved, err := service.Resolve(ResolveRequest{ProjectRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Source != ProfileSourceGlobal || resolved.Profile.Endpoints["orchestration"] != "https://global.example" {
		t.Fatalf("fallback = %+v", resolved)
	}
	config := readServiceFile(t, filepath.Join(root, ".camunda.yaml"))
	if !strings.Contains(config, "environment: shared") {
		t.Fatalf("project selection was cleared despite global fallback:\n%s", config)
	}
}

func TestServiceGlobalRemoveRejectsUnknownOrKnownProjectReferences(t *testing.T) {
	t.Run("unknown completeness", func(t *testing.T) {
		home, _ := serviceFixture(t)
		service := NewService(home)
		if err := service.SaveGlobal(remoteProfile("prod", "https://prod.example")); err != nil {
			t.Fatal(err)
		}
		err := service.Remove("prod", "", ProfileSourceGlobal)
		var typed *Error
		if !errors.As(err, &typed) || typed.Kind != ErrorConflict {
			t.Fatalf("error = %v, want conflict", err)
		}
	})

	t.Run("known reference", func(t *testing.T) {
		home, root := serviceFixtureWithCompleteReferences(t)
		service := NewService(home)
		if err := service.SaveGlobal(remoteProfile("prod", "https://prod.example")); err != nil {
			t.Fatal(err)
		}
		if _, err := service.Use("prod", root); err != nil {
			t.Fatal(err)
		}
		err := service.Remove("prod", "", ProfileSourceGlobal)
		var typed *Error
		if !errors.As(err, &typed) || typed.Kind != ErrorConflict || !strings.Contains(err.Error(), root) {
			t.Fatalf("error = %v, want affected-project conflict", err)
		}
	})
}

func TestServiceRecoversRemovalAfterEveryDurablePhase(t *testing.T) {
	for _, phase := range []string{"intent", "tombstoned", "configs", "commit"} {
		t.Run(phase, func(t *testing.T) {
			home, root := serviceFixtureWithCompleteReferences(t)
			service := NewService(home)
			if err := service.SaveProject(root, remoteProfile("prod", "https://prod.example")); err != nil {
				t.Fatal(err)
			}
			if _, err := service.Use("prod", root); err != nil {
				t.Fatal(err)
			}
			service.hooks.crashAfterPhase = func(got string) error {
				if got == phase {
					return errSimulatedCrash
				}
				return nil
			}
			if err := service.Remove("prod", root, ProfileSourceProject); !errors.Is(err, errSimulatedCrash) {
				t.Fatalf("remove error = %v, want simulated crash", err)
			}

			restarted := NewService(home)
			_, resolveErr := restarted.Resolve(ResolveRequest{ProjectRoot: root})
			if phase == "commit" {
				if resolveErr != nil {
					t.Fatalf("committed recovery did not fall back: %v", resolveErr)
				}
				if _, err := os.Stat(filepath.Join(root, "environments", "prod.yaml")); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("committed profile still exists: %v", err)
				}
			} else {
				if resolveErr != nil {
					t.Fatalf("rollback recovery failed: %v", resolveErr)
				}
				if _, err := os.Stat(filepath.Join(root, "environments", "prod.yaml")); err != nil {
					t.Fatalf("profile not restored: %v", err)
				}
			}
			if _, err := os.Stat(filepath.Join(home, ".env-remove-journal.yaml")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("journal not cleaned: %v", err)
			}
		})
	}
}

func TestCorruptJournalFailsClosedAtEveryPhase(t *testing.T) {
	for _, phase := range []string{phaseIntent, phaseTombstoned, phaseConfigs, phaseCommit} {
		t.Run(phase, func(t *testing.T) {
			home, root := serviceFixtureWithCompleteReferences(t)
			service := NewService(home)
			if err := service.SaveProject(root, remoteProfile("prod", "https://prod.example")); err != nil {
				t.Fatal(err)
			}
			if _, err := service.Use("prod", root); err != nil {
				t.Fatal(err)
			}
			service.hooks.crashAfterPhase = func(got string) error {
				if got == phase {
					return errSimulatedCrash
				}
				return nil
			}
			if err := service.Remove("prod", root, ProfileSourceProject); !errors.Is(err, errSimulatedCrash) {
				t.Fatalf("remove = %v", err)
			}
			journalPath := filepath.Join(home, ".env-remove-journal.yaml")
			data, err := os.ReadFile(journalPath)
			if err != nil {
				t.Fatal(err)
			}
			var journal removalJournal
			if err := yaml.Unmarshal(data, &journal); err != nil {
				t.Fatal(err)
			}
			if phase == phaseCommit {
				if err := os.Remove(filepath.Join(journal.ProfileRoot, journal.TombstoneName)); err != nil {
					t.Fatal(err)
				}
			}
			data = append(data, []byte("unexpected: true\n")...)
			if err := os.WriteFile(journalPath, data, 0o600); err != nil {
				t.Fatal(err)
			}
			beforeConfig := readServiceFile(t, filepath.Join(home, "config.yaml"))
			beforeProject := readServiceFile(t, filepath.Join(root, ".camunda.yaml"))
			beforeEntries := directoryNames(t, filepath.Join(root, "environments"))

			_, err = NewService(home).Resolve(ResolveRequest{ProjectRoot: root})
			if err == nil {
				t.Fatal("corrupt journal was accepted")
			}
			if got := readServiceFile(t, filepath.Join(home, "config.yaml")); got != beforeConfig {
				t.Fatal("global config mutated while rejecting journal")
			}
			if got := readServiceFile(t, filepath.Join(root, ".camunda.yaml")); got != beforeProject {
				t.Fatal("project config mutated while rejecting journal")
			}
			if got := directoryNames(t, filepath.Join(root, "environments")); !reflect.DeepEqual(got, beforeEntries) {
				t.Fatalf("profile root mutated: before=%v after=%v", beforeEntries, got)
			}
		})
	}
}

func TestJournalPhaseInvariantsFailClosed(t *testing.T) {
	tests := []struct {
		name       string
		phase      string
		breakState func(t *testing.T, home string, journal removalJournal)
	}{
		{
			name: "unknown phase", phase: phaseIntent,
			breakState: func(t *testing.T, home string, journal removalJournal) {
				rewriteJournal(t, home, journal, func(j *removalJournal) { j.Phase = "future" })
			},
		},
		{
			name: "intent missing profile", phase: phaseIntent,
			breakState: func(t *testing.T, _ string, journal removalJournal) {
				if err := os.Remove(filepath.Join(journal.ProfileRoot, journal.ProfileName)); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "tombstoned restored profile", phase: phaseTombstoned,
			breakState: func(t *testing.T, _ string, journal removalJournal) {
				if err := os.Rename(
					filepath.Join(journal.ProfileRoot, journal.TombstoneName),
					filepath.Join(journal.ProfileRoot, journal.ProfileName),
				); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			home, root := serviceFixtureWithCompleteReferences(t)
			service := NewService(home)
			if err := service.SaveProject(root, remoteProfile("prod", "https://prod.example")); err != nil {
				t.Fatal(err)
			}
			if _, err := service.Use("prod", root); err != nil {
				t.Fatal(err)
			}
			service.hooks.crashAfterPhase = func(got string) error {
				if got == test.phase {
					return errSimulatedCrash
				}
				return nil
			}
			if err := service.Remove("prod", root, ProfileSourceProject); !errors.Is(err, errSimulatedCrash) {
				t.Fatal(err)
			}
			journal := readRemovalJournal(t, home)
			test.breakState(t, home, journal)
			beforeConfig := readServiceFile(t, filepath.Join(home, "config.yaml"))
			if _, err := NewService(home).Resolve(ResolveRequest{ProjectRoot: root}); err == nil {
				t.Fatal("inconsistent journal was accepted")
			}
			if got := readServiceFile(t, filepath.Join(home, "config.yaml")); got != beforeConfig {
				t.Fatal("config mutated while rejecting inconsistent journal")
			}
		})
	}
}

func TestMalformedReferenceMetadataBlocksGlobalRemoval(t *testing.T) {
	tests := []struct {
		name     string
		metadata string
	}{
		{name: "string true", metadata: "  complete: \"true\"\n  projects: {}\n"},
		{name: "duplicate complete", metadata: "  complete: true\n  complete: true\n  projects: {}\n"},
		{name: "unknown field", metadata: "  complete: true\n  projects: {}\n  future: value\n"},
		{name: "identity mismatch", metadata: "  complete: true\n  projects:\n    wrong:\n      root: {ROOT}\n      environment: prod\n      source: global\n"},
		{name: "invalid source", metadata: "  complete: true\n  projects:\n    {ID}:\n      root: {ROOT}\n      environment: prod\n      source: other\n"},
		{name: "invalid environment", metadata: "  complete: true\n  projects:\n    {ID}:\n      root: {ROOT}\n      environment: ../bad\n      source: global\n"},
		{name: "duplicate roots", metadata: "  complete: true\n  projects:\n    {ID}:\n      root: {ROOT}\n      environment: prod\n      source: global\n    00000000000000000000000000000000:\n      root: {ROOT}\n      environment: prod\n      source: global\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			home, root := serviceFixture(t)
			service := NewService(home)
			if err := service.SaveGlobal(remoteProfile("prod", "https://prod.example")); err != nil {
				t.Fatal(err)
			}
			metadata := strings.ReplaceAll(test.metadata, "{ROOT}", root)
			metadata = strings.ReplaceAll(metadata, "{ID}", projectIdentity(root))
			configPath := filepath.Join(home, "config.yaml")
			if err := os.WriteFile(configPath, []byte("version: \"8.9\"\nenvironmentReferences:\n"+metadata), 0o600); err != nil {
				t.Fatal(err)
			}
			err := service.Remove("prod", "", ProfileSourceGlobal)
			var typed *Error
			if !errors.As(err, &typed) || (typed.Kind != ErrorInvalid && typed.Kind != ErrorConflict) {
				t.Fatalf("error = %v, want typed invalid/conflict", err)
			}
			if _, err := os.Stat(filepath.Join(home, "envs", "prod.yaml")); err != nil {
				t.Fatalf("profile mutated after malformed metadata: %v", err)
			}
		})
	}
}

var errSimulatedCrash = errors.New("simulated crash")

func serviceFixture(t *testing.T) (string, string) {
	t.Helper()
	home := t.TempDir()
	projectRoot := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(home, "config.yaml"),
		[]byte("version: \"8.9\"\ncustom: keep\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(projectRoot, ".camunda.yaml"),
		[]byte("name: fixture\ncamundaVersion: \"8.9\"\ncustomProject: keep\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	return home, projectRoot
}

func serviceFixtureWithCompleteReferences(t *testing.T) (string, string) {
	t.Helper()
	home, root := serviceFixture(t)
	if err := os.WriteFile(
		filepath.Join(home, "config.yaml"),
		[]byte("version: \"8.9\"\ncustom: keep\nenvironmentReferences:\n  complete: true\n  projects: {}\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	return home, root
}

func remoteProfile(name, endpoint string) Profile {
	return Profile{
		Name:      name,
		Kind:      "remote",
		Endpoints: map[string]string{"orchestration": endpoint},
		Auth: AuthRefs{
			ClientIDEnv:     "CAMUNDA_CLIENT_ID",
			ClientSecretEnv: "CAMUNDA_CLIENT_SECRET",
		},
	}
}

func readServiceFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func directoryNames(t *testing.T, path string) []string {
	t.Helper()
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func readRemovalJournal(t *testing.T, home string) removalJournal {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, ".env-remove-journal.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var journal removalJournal
	if err := yaml.Unmarshal(data, &journal); err != nil {
		t.Fatal(err)
	}
	return journal
}

func rewriteJournal(t *testing.T, home string, journal removalJournal, mutate func(*removalJournal)) {
	t.Helper()
	mutate(&journal)
	data, err := yaml.Marshal(journal)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".env-remove-journal.yaml"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}
