package testgen

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/bpmn"
)

func TestRenderIsDeterministicAndDoesNotWrite(t *testing.T) {
	doc := parseDocument(t, twoProcessBPMN)
	before := snapshotTree(t, ".")
	first, err := Render(doc, Options{Lang: "js"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Render(doc, Options{Lang: "js"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("render is not deterministic:\n%+v\n%+v", first, second)
	}
	if after := snapshotTree(t, "."); !reflect.DeepEqual(before, after) {
		t.Fatalf("Render wrote to disk: before=%v after=%v", before, after)
	}
	if got := artifactPaths(first); !reflect.DeepEqual(got, []string{
		"js/One.spec.js",
		"js/Two.spec.js",
	}) {
		t.Fatalf("paths = %v", got)
	}
	for _, artifact := range first {
		assertSafeRelativePath(t, artifact.Path)
		if artifact.MediaType != "text/javascript" {
			t.Fatalf("media type = %q", artifact.MediaType)
		}
		content := string(artifact.Content)
		if !strings.Contains(content, "@playwright/test") || !strings.Contains(content, "TODO") {
			t.Fatalf("not a meaningful Playwright scaffold:\n%s", content)
		}
	}
}

func TestRenderProducesMeaningfulScaffoldForEachLanguage(t *testing.T) {
	doc := parseDocument(t, duplicateJobsBPMN)
	tests := []struct {
		lang      string
		path      string
		mediaType string
		contains  []string
	}{
		{
			lang: "java", path: "java/Order_ProcessTest.java", mediaType: "text/x-java-source",
			contains: []string{"class Order_ProcessTest", `PROCESS_ID = "order/process"`, "validate_customer", "TODO"},
		},
		{
			lang: "js", path: "js/Order_Process.spec.js", mediaType: "text/javascript",
			contains: []string{"@playwright/test", `PROCESS_ID = 'order/process'`, "validate-customer", "TODO"},
		},
		{
			lang: "python", path: "python/test_order_process.py", mediaType: "text/x-python",
			contains: []string{"import pytest", `PROCESS_ID = "order/process"`, "validate-customer", "TODO"},
		},
	}
	for _, test := range tests {
		t.Run(test.lang, func(t *testing.T) {
			artifacts, err := Render(doc, Options{Lang: test.lang})
			if err != nil {
				t.Fatal(err)
			}
			if len(artifacts) != 1 || artifacts[0].Path != test.path || artifacts[0].MediaType != test.mediaType {
				t.Fatalf("artifacts = %+v", artifacts)
			}
			for _, text := range test.contains {
				if !bytes.Contains(artifacts[0].Content, []byte(text)) {
					t.Fatalf("missing %q:\n%s", text, artifacts[0].Content)
				}
			}
			if got := bytes.Count(artifacts[0].Content, []byte("validate-customer")); got != 1 {
				t.Fatalf("duplicate job type rendered %d times", got)
			}
		})
	}
}

func TestRenderMatchesGoldenScaffolds(t *testing.T) {
	doc, err := bpmn.ParseFile(filepath.Join("..", "..", "testdata", "bpmn", "order-v1.bpmn"))
	if err != nil {
		t.Fatal(err)
	}
	for _, lang := range []string{"java", "js"} {
		t.Run(lang, func(t *testing.T) {
			artifacts, err := Render(doc, Options{Lang: lang})
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(filepath.Join("..", "..", "testdata", "golden", "testgen", "order-v1."+lang+".golden"))
			if err != nil {
				t.Fatal(err)
			}
			if len(artifacts) != 1 || !bytes.Equal(artifacts[0].Content, want) {
				t.Fatalf("golden mismatch for %s:\n%s", lang, artifacts[0].Content)
			}
		})
	}
}

func TestRenderRejectsInvalidRequestsAndPathCollisions(t *testing.T) {
	if artifacts, err := Render(parseDocument(t, twoProcessBPMN), Options{Lang: "ruby"}); err == nil || artifacts != nil {
		t.Fatalf("unsupported language returned artifacts=%v err=%v", artifacts, err)
	}
	collision := strings.Replace(twoProcessBPMN, `id="one"`, `id="a-b"`, 1)
	collision = strings.Replace(collision, `id="two"`, `id="a_b"`, 1)
	if artifacts, err := Render(parseDocument(t, collision), Options{Lang: "js"}); err == nil || artifacts != nil {
		t.Fatalf("collision returned artifacts=%v err=%v", artifacts, err)
	}
	if artifacts, err := Render(bpmn.Document{}, Options{Lang: "js"}); err == nil || artifacts != nil {
		t.Fatalf("empty document returned artifacts=%v err=%v", artifacts, err)
	}
}

func TestRenderedScaffoldsCompileWithoutExecution(t *testing.T) {
	doc := parseDocument(t, duplicateJobsBPMN)
	tests := []struct {
		lang    string
		command string
		args    func(string) []string
	}{
		{lang: "java", command: "javac", args: func(path string) []string { return []string{path} }},
		{lang: "js", command: "node", args: func(path string) []string { return []string{"--check", path} }},
		{lang: "python", command: "python3", args: func(path string) []string { return []string{"-m", "py_compile", path} }},
	}
	for _, test := range tests {
		t.Run(test.lang, func(t *testing.T) {
			if _, err := exec.LookPath(test.command); err != nil {
				t.Skipf("%s is not installed", test.command)
			}
			artifacts, err := Render(doc, Options{Lang: test.lang})
			if err != nil {
				t.Fatal(err)
			}
			paths, err := Write(t.TempDir(), artifacts, false)
			if err != nil {
				t.Fatal(err)
			}
			args := test.args(paths[0])
			if test.lang == "java" {
				stubs := writeJUnitStubs(t, filepath.Dir(paths[0]))
				args = append(stubs, paths[0])
			}
			command := exec.Command(test.command, args...)
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("%s rejected generated scaffold: %v\n%s\n%s", test.command, err, output, artifacts[0].Content)
			}
			if test.lang == "java" {
				content := string(artifacts[0].Content)
				if strings.Count(content, "@Test") != 2 || strings.Count(content, "@Disabled") != 2 {
					t.Fatalf("JUnit discovery annotations missing:\n%s", content)
				}
				if strings.Count(content, "UnsupportedOperationException") != 2 {
					t.Fatalf("disabled methods lack no-execution sentinels:\n%s", content)
				}
			}
		})
	}
}

func TestGeneratedJavaIsDiscoveredAndDisabledByJUnitPlatform(t *testing.T) {
	if _, err := exec.LookPath("mvn"); err != nil {
		t.Skipf("Maven is unavailable: %v", err)
	}
	doc := parseDocument(t, duplicateJobsBPMN)
	artifacts, err := Render(doc, Options{Lang: "java"})
	if err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	testSource := filepath.Join(project, "src", "test", "java")
	if err := os.MkdirAll(testSource, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "pom.xml"), []byte(junitDiscoveryPOM), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(testSource, filepath.Base(artifacts[0].Path)), artifacts[0].Content, 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("mvn", "-B", "-ntp", "test")
	command.Dir = project
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("Maven/JUnit Platform rejected generated test: %v\n%s\n%s", err, output, artifacts[0].Content)
	}
	report, err := os.ReadFile(filepath.Join(project, "target", "surefire-reports", "TEST-Order_ProcessTest.xml"))
	if err != nil {
		t.Fatalf("read JUnit Platform report: %v\nMaven output:\n%s", err, output)
	}
	for _, attribute := range []string{`tests="2"`, `skipped="2"`, `failures="0"`, `errors="0"`} {
		if !bytes.Contains(report, []byte(attribute)) {
			t.Fatalf("JUnit discovery report missing %s:\n%s", attribute, report)
		}
	}
}

func TestWritePublishesAtomicallyWithOverwriteSemantics(t *testing.T) {
	root := t.TempDir()
	artifacts := []Artifact{
		{Path: "js/one.spec.js", MediaType: "text/javascript", Content: []byte("one")},
		{Path: "js/two.spec.js", MediaType: "text/javascript", Content: []byte("two")},
	}
	paths, err := Write(root, artifacts, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 {
		t.Fatalf("paths = %v", paths)
	}
	for _, path := range paths {
		info, statErr := os.Stat(path)
		if statErr != nil {
			t.Fatal(statErr)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode = %o", path, info.Mode().Perm())
		}
	}

	artifacts[0].Content = []byte("replacement")
	if _, err := Write(root, artifacts, false); err == nil {
		t.Fatal("expected overwrite refusal")
	}
	if got, _ := os.ReadFile(paths[0]); string(got) != "one" {
		t.Fatalf("existing file changed: %q", got)
	}
	if _, err := Write(root, artifacts, true); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(paths[0]); string(got) != "replacement" {
		t.Fatalf("forced content = %q", got)
	}
}

func TestWriteRejectsUnsafePathsAndSymlinksBeforePublishing(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	tests := []Artifact{
		{Path: "../outside.js", Content: []byte("bad")},
		{Path: filepath.Join(root, "absolute.js"), Content: []byte("bad")},
		{Path: "escape/outside.js", Content: []byte("bad")},
	}
	for _, artifact := range tests {
		if _, err := Write(root, []Artifact{artifact}, false); err == nil {
			t.Fatalf("accepted unsafe path %q", artifact.Path)
		}
	}
	if files := snapshotTree(t, outside); len(files) != 0 {
		t.Fatalf("wrote outside root: %v", files)
	}
}

func TestWriteRollsBackAfterPublishFailure(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "blocked")
	if err := os.WriteFile(blocker, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	artifacts := []Artifact{
		{Path: "created/one.js", Content: []byte("one")},
		{Path: "blocked/two.js", Content: []byte("two")},
	}
	if _, err := Write(root, artifacts, false); err == nil {
		t.Fatal("expected publish failure")
	}
	if _, err := os.Stat(filepath.Join(root, "created")); !os.IsNotExist(err) {
		t.Fatalf("partial output remains: %v", err)
	}
	if got, err := os.ReadFile(blocker); err != nil || string(got) != "keep" {
		t.Fatalf("blocker changed: %q, %v", got, err)
	}
}

func TestWriteRejectsDuplicatePathsBeforePublishing(t *testing.T) {
	root := filepath.Join(t.TempDir(), "not-created")
	_, err := Write(root, []Artifact{
		{Path: "js/Same.js", Content: []byte("one")},
		{Path: "js/same.js", Content: []byte("two")},
	}, false)
	if err == nil {
		t.Fatal("expected collision")
	}
	if _, statErr := os.Stat(root); !os.IsNotExist(statErr) {
		t.Fatalf("output root created before collision rejection: %v", statErr)
	}
}

func TestWriteRejectsSymlinkInRootAncestors(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(base, "linked")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	_, err := Write(filepath.Join(link, "generated"), []Artifact{{Path: "one.js", Content: []byte("bad")}}, false)
	if err == nil {
		t.Fatal("accepted symlink root ancestor")
	}
	if files := snapshotTree(t, outside); len(files) != 0 {
		t.Fatalf("wrote through root ancestor symlink: %v", files)
	}
}

func TestWriteRejectsParentSwapWithoutOutsideWrite(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	parent := filepath.Join(root, "js")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	held := filepath.Join(root, "js-held")
	_, err := writeWithHooks(root, []Artifact{{Path: "js/one.js", Content: []byte("bad")}}, false, writerHooks{
		afterPrepare: func() error {
			if err := os.Rename(parent, held); err != nil {
				return err
			}
			return os.Symlink(outside, parent)
		},
	})
	if err == nil {
		t.Fatal("parent swap was not detected")
	}
	if files := snapshotTree(t, outside); len(files) != 0 {
		t.Fatalf("wrote through swapped parent: %v", files)
	}
	if _, statErr := os.Stat(filepath.Join(held, "one.js")); !os.IsNotExist(statErr) {
		t.Fatalf("published into detached parent: %v", statErr)
	}
}

func TestWriteRejectsDestinationSwapWithoutOutsideWrite(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "js"), 0o700); err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(outside, "victim")
	if err := os.WriteFile(victim, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := writeWithHooks(root, []Artifact{{Path: "js/one.js", Content: []byte("bad")}}, true, writerHooks{
		afterPrepare: func() error {
			return os.Symlink(victim, filepath.Join(root, "js", "one.js"))
		},
	})
	if err == nil {
		t.Fatal("destination swap was not detected")
	}
	if got, readErr := os.ReadFile(victim); readErr != nil || string(got) != "keep" {
		t.Fatalf("outside destination changed: %q, %v", got, readErr)
	}
}

func TestWriteRestoresCurrentAndPriorBackupsAfterPublishFailure(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "one.js"), "one")
	writeFile(t, filepath.Join(root, "two.js"), "two")
	_, err := writeWithHooks(root, []Artifact{
		{Path: "one.js", Content: []byte("new-one")},
		{Path: "two.js", Content: []byte("new-two")},
	}, true, writerHooks{
		fail: func(operation string, index int) error {
			if operation == "publish" && index == 1 {
				return errors.New("injected publish failure")
			}
			return nil
		},
	})
	if err == nil {
		t.Fatal("expected injected failure")
	}
	for path, want := range map[string]string{"one.js": "one", "two.js": "two"} {
		got, readErr := os.ReadFile(filepath.Join(root, path))
		if readErr != nil || string(got) != want {
			t.Fatalf("%s = %q, %v; want %q", path, got, readErr, want)
		}
	}
}

func TestWritePreservesBackupWhenRestoreFails(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "one.js"), "original")
	_, err := writeWithHooks(root, []Artifact{{Path: "one.js", Content: []byte("replacement")}}, true, writerHooks{
		fail: func(operation string, index int) error {
			switch operation {
			case "publish", "restore":
				return errors.New("injected " + operation + " failure")
			default:
				return nil
			}
		},
	})
	var recovery *RecoveryError
	if !errors.As(err, &recovery) || len(recovery.Paths) != 1 {
		t.Fatalf("recovery error = %#v", err)
	}
	got, readErr := os.ReadFile(recovery.Paths[0])
	if readErr != nil || string(got) != "original" {
		t.Fatalf("recovery backup = %q, %v", got, readErr)
	}
	canonicalRoot, canonicalErr := filepath.EvalSymlinks(root)
	if canonicalErr != nil {
		t.Fatal(canonicalErr)
	}
	if !strings.HasPrefix(recovery.Paths[0], canonicalRoot+string(filepath.Separator)) {
		t.Fatalf("recovery path escaped root: %q", recovery.Paths[0])
	}
}

func TestWriteRollsBackFinalParentReplacement(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	parent := filepath.Join(root, "js")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	detached := filepath.Join(root, "js-detached")
	_, err := writeWithHooks(root, []Artifact{{Path: "js/one.js", Content: []byte("new")}}, false, writerHooks{
		afterPublish: func() error {
			if err := os.Rename(parent, detached); err != nil {
				return err
			}
			return os.Symlink(outside, parent)
		},
	})
	if err == nil {
		t.Fatal("final parent replacement was not detected")
	}
	if _, statErr := os.Stat(filepath.Join(detached, "one.js")); !os.IsNotExist(statErr) {
		t.Fatalf("published artifact was not rolled back: %v", statErr)
	}
	if files := snapshotTree(t, outside); len(files) != 0 {
		t.Fatalf("outside path changed: %v", files)
	}
}

func TestWriteRollsBackFinalDestinationReplacement(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	victim := filepath.Join(outside, "victim")
	writeFile(t, victim, "keep")
	final := filepath.Join(root, "one.js")
	_, err := writeWithHooks(root, []Artifact{{Path: "one.js", Content: []byte("new")}}, false, writerHooks{
		afterPublish: func() error {
			if err := os.Remove(final); err != nil {
				return err
			}
			return os.Symlink(victim, final)
		},
	})
	if err == nil {
		t.Fatal("final destination replacement was not detected")
	}
	if got, readErr := os.ReadFile(victim); readErr != nil || string(got) != "keep" {
		t.Fatalf("outside destination changed: %q, %v", got, readErr)
	}
}

func TestWriteRelocatesAndVerifiesRecoveryOutsideReplacedRoot(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "output")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "one.js"), "original")
	detached := filepath.Join(base, "detached")
	outside := t.TempDir()
	_, err := writeWithHooks(root, []Artifact{{Path: "one.js", Content: []byte("replacement")}}, true, writerHooks{
		afterPublish: func() error {
			if err := os.Rename(root, detached); err != nil {
				return err
			}
			return os.Symlink(outside, root)
		},
		fail: func(operation string, _ int) error {
			if operation == "restore" {
				return errors.New("injected restore failure")
			}
			return nil
		},
	})
	var recovery *RecoveryError
	if !errors.As(err, &recovery) || len(recovery.Paths) != 1 {
		t.Fatalf("recovery error = %#v", err)
	}
	canonicalBase, canonicalErr := filepath.EvalSymlinks(base)
	if canonicalErr != nil {
		t.Fatal(canonicalErr)
	}
	canonicalRoot := filepath.Join(canonicalBase, filepath.Base(root))
	if strings.HasPrefix(recovery.Paths[0], canonicalRoot+string(filepath.Separator)) ||
		!strings.HasPrefix(recovery.Paths[0], canonicalBase+string(filepath.Separator)) {
		t.Fatalf("unverified recovery path reported: %q", recovery.Paths[0])
	}
	got, readErr := os.ReadFile(recovery.Paths[0])
	if readErr != nil || string(got) != "original" {
		t.Fatalf("relocated recovery backup = %q, %v", got, readErr)
	}
	if files := snapshotTree(t, outside); len(files) != 0 {
		t.Fatalf("outside replacement changed: %v", files)
	}
}

func writeJUnitStubs(t *testing.T, root string) []string {
	t.Helper()
	dir := filepath.Join(root, "org", "junit", "jupiter", "api")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"Test.java":     "package org.junit.jupiter.api; public @interface Test {}\n",
		"Disabled.java": "package org.junit.jupiter.api; public @interface Disabled { String value() default \"\"; }\n",
	}
	var paths []string
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func parseDocument(t *testing.T, source string) bpmn.Document {
	t.Helper()
	doc, err := bpmn.Parse(strings.NewReader(source))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func artifactPaths(artifacts []Artifact) []string {
	paths := make([]string, len(artifacts))
	for i, artifact := range artifacts {
		paths[i] = artifact.Path
	}
	return paths
}

func assertSafeRelativePath(t *testing.T, path string) {
	t.Helper()
	if filepath.IsAbs(path) || path == "." || path == ".." || strings.HasPrefix(path, ".."+string(filepath.Separator)) {
		t.Fatalf("unsafe artifact path %q", path)
	}
}

func snapshotTree(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root || entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, relative)
		return nil
	})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return files
}

const twoProcessBPMN = `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="one"><startEvent id="s1"/></process>
  <process id="two"><startEvent id="s2"/></process>
</definitions>`

const duplicateJobsBPMN = `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
 xmlns:zeebe="http://camunda.org/schema/zeebe/1.0">
  <process id="order/process">
    <startEvent id="start"/>
    <serviceTask id="validate-one"><extensionElements><zeebe:taskDefinition type="validate-customer"/></extensionElements></serviceTask>
    <serviceTask id="validate-two"><extensionElements><zeebe:taskDefinition type="validate-customer"/></extensionElements></serviceTask>
  </process>
</definitions>`

const junitDiscoveryPOM = `<project xmlns="http://maven.apache.org/POM/4.0.0">
  <modelVersion>4.0.0</modelVersion>
  <groupId>testgen</groupId>
  <artifactId>generated-discovery</artifactId>
  <version>1.0.0</version>
  <properties>
    <maven.compiler.release>17</maven.compiler.release>
    <project.build.sourceEncoding>UTF-8</project.build.sourceEncoding>
  </properties>
  <dependencies>
    <dependency>
      <groupId>org.junit.jupiter</groupId>
      <artifactId>junit-jupiter</artifactId>
      <version>5.11.4</version>
      <scope>test</scope>
    </dependency>
  </dependencies>
  <build>
    <plugins>
      <plugin>
        <groupId>org.apache.maven.plugins</groupId>
        <artifactId>maven-surefire-plugin</artifactId>
        <version>3.5.2</version>
      </plugin>
    </plugins>
  </build>
</project>
`
