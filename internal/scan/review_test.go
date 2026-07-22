package scan

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDetectsStructuredBearerAndJWTCredentials(t *testing.T) {
	root := t.TempDir()
	secrets := []string{
		"json-secret-value-123",
		"yaml-secret-value-123",
		"standardBearerToken12345",
		"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.signaturevalue123456",
	}
	writeScanFile(t, root, "config.json", `{"client_secret":"`+secrets[0]+`"}`+"\n")
	writeScanFile(t, root, "config.yaml", `'password': '`+secrets[1]+`'`+"\n")
	writeScanFile(t, root, "headers.txt", "Authorization: Bearer "+secrets[2]+"\n")
	writeScanFile(t, root, "oauth.json", `{"access_token":"`+secrets[3]+`"}`+"\n")

	result, err := WalkWithReport(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 4 {
		t.Fatalf("findings = %+v", result.Findings)
	}
	rules := map[string]int{}
	for _, finding := range result.Findings {
		rules[finding.RuleID]++
		for _, secret := range secrets {
			if strings.Contains(finding.Snippet, secret) || strings.Contains(FormatText(result), secret) {
				t.Fatalf("secret leaked: %q in %+v", secret, finding)
			}
		}
	}
	if rules["secret.client"] != 1 || rules["secret.password"] != 1 ||
		rules["secret.token"] != 1 || rules["secret.oauth"] != 1 {
		t.Fatalf("rules = %v", rules)
	}
}

func TestStructuredDetectionRejectsProseSamplesAndMalformedJWTs(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "scan", "detection")
	result, err := WalkWithReport(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("false positives = %+v", result.Findings)
	}
}

func TestInlineSuppressionRequiresValidTrailingComment(t *testing.T) {
	root := t.TempDir()
	writeScanFile(t, root, "script.sh", "CLIENT_SECRET=shell-secret-value # camunda-scan-ignore\n")
	writeScanFile(t, root, "config.yaml", "password: yaml-secret-value # camunda-scan-ignore\n")
	writeScanFile(t, root, "worker.js", `const apiKey = "js-secret-value"; // camunda-scan-ignore`+"\n")
	writeScanFile(t, root, "Worker.java", `String password = "java-secret-value"; /* camunda-scan-ignore */`+"\n")
	writeScanFile(t, root, "process.bpmn", `<property name="password" value="xml-secret-value"/><!-- camunda-scan-ignore -->`+"\n")
	writeScanFile(t, root, "invalid.json", `{"client_secret":"json-secret-value camunda-scan-ignore"}`+"\n")
	writeScanFile(t, root, "invalid.properties", "password=properties-secret-value # camunda-scan-ignore\n")
	writeScanFile(t, root, "invalid.txt", "password=text-secret-value # camunda-scan-ignore\n")
	writeScanFile(t, root, "before.bpmn", `<!-- camunda-scan-ignore --><property name="password" value="before-secret-value"/>`+"\n")

	result, err := WalkWithReport(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, finding := range result.Findings {
		files = append(files, finding.File)
	}
	want := []string{"before.bpmn", "invalid.json", "invalid.properties", "invalid.txt"}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("files = %v, want %v", files, want)
	}
}

func TestTemplateLiteralDirectiveTextDoesNotSuppress(t *testing.T) {
	root := t.TempDir()
	writeScanFile(t, root, "line-template.js",
		"const password = \"line-template-secret\"; const note = `// camunda-scan-ignore`;\n")
	writeScanFile(t, root, "block-template.ts",
		"const apiKey = \"block-template-secret\"; const note = `/* camunda-scan-ignore */`;\n")
	writeScanFile(t, root, "interpolation-template.js",
		"const clientSecret = \"interpolation-secret\"; const note = `value ${\"// camunda-scan-ignore\"}`;\n")
	writeScanFile(t, root, "escaped-template.ts",
		"const password = \"escaped-template-secret\"; const note = `escaped \\` // camunda-scan-ignore`;\n")
	writeScanFile(t, root, "real-comment.js",
		"const note = `value ${name}`; const password = \"suppressed-secret\"; // camunda-scan-ignore\n")

	result, err := WalkWithReport(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, finding := range result.Findings {
		files = append(files, finding.File)
	}
	want := []string{"block-template.ts", "escaped-template.ts", "interpolation-template.js", "line-template.js"}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("files = %v, want %v", files, want)
	}
}

func TestRuntimeReferencesAreNotLiteralCredentials(t *testing.T) {
	root := t.TempDir()
	writeScanFile(t, root, "runtime.env", "CLIENT_SECRET=$CAMUNDA_CLIENT_SECRET\nPASSWORD=${CAMUNDA_PASSWORD}\n")
	writeScanFile(t, root, "runtime.sh", "api_key=$CAMUNDA_API_KEY\n")
	writeScanFile(t, root, "runtime.js", "const apiKey = process.env.CAMUNDA_API_KEY;\n")
	writeScanFile(t, root, "runtime.ts",
		"const clientSecret = Deno.env.get(\"CAMUNDA_CLIENT_SECRET\");\n"+
			"const apiKey = import.meta.env.CAMUNDA_API_KEY;\n")
	writeScanFile(t, root, "Runtime.java", "String password = System.getenv(\"CAMUNDA_PASSWORD\");\n")
	writeScanFile(t, root, "runtime.go", "password := os.Getenv(\"CAMUNDA_PASSWORD\")\n")
	writeScanFile(t, root, "literals.js", "const apiKey = \"literal-process.env.CAMUNDA_API_KEY-value\";\n")
	writeScanFile(t, root, "literals.ts", "const clientSecret = \"literal-Deno.env.get-value\";\n")
	writeScanFile(t, root, "Literals.java", "String password = \"literal-System.getenv-value\";\n")
	writeScanFile(t, root, "literals.go", "password := \"literal-os.Getenv-value\"\n")
	writeScanFile(t, root, "literals.env", "CLIENT_SECRET=literal-$CAMUNDA_CLIENT_SECRET-value\n")

	result, err := WalkWithReport(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, finding := range result.Findings {
		files = append(files, finding.File)
	}
	want := []string{"Literals.java", "literals.env", "literals.go", "literals.js", "literals.ts"}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("files = %v, want %v; findings=%+v", files, want, result.Findings)
	}
}

func TestRuntimeReferenceQuotingIsSourceAware(t *testing.T) {
	root := t.TempDir()
	writeScanFile(t, root, "single.env", "CLIENT_SECRET='$CAMUNDA_CLIENT_SECRET'\n")
	writeScanFile(t, root, "single.sh", "password='${CAMUNDA_PASSWORD}'\n")
	writeScanFile(t, root, "quoted-dollar.json", `{"client_secret":"$CAMUNDA_CLIENT_SECRET"}`+"\n")
	writeScanFile(t, root, "quoted-braced.json", `{"client_secret":"${CAMUNDA_CLIENT_SECRET}"}`+"\n")
	writeScanFile(t, root, "quoted-dollar.form", `{"password":"$CAMUNDA_PASSWORD"}`+"\n")
	writeScanFile(t, root, "quoted-braced.form", `{"password":"${CAMUNDA_PASSWORD}"}`+"\n")
	writeScanFile(t, root, "quoted-dollar.properties", `password="$CAMUNDA_PASSWORD"`+"\n")
	writeScanFile(t, root, "quoted-braced.properties", `password="${CAMUNDA_PASSWORD}"`+"\n")
	writeScanFile(t, root, "quoted-dollar.yaml", `client_secret: "$CAMUNDA_CLIENT_SECRET"`+"\n")
	writeScanFile(t, root, "quoted-braced.yaml", `client_secret: "${CAMUNDA_CLIENT_SECRET}"`+"\n")
	writeScanFile(t, root, "runtime.env",
		"CLIENT_SECRET=$CAMUNDA_CLIENT_SECRET\nPASSWORD=\"${CAMUNDA_PASSWORD}\"\n")
	writeScanFile(t, root, "runtime.sh",
		"api_key=$CAMUNDA_API_KEY\nclient_secret=\"${CAMUNDA_CLIENT_SECRET}\"\n")

	result, err := WalkWithReport(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, finding := range result.Findings {
		files = append(files, finding.File)
	}
	want := []string{
		"quoted-braced.form", "quoted-braced.json", "quoted-braced.properties", "quoted-braced.yaml",
		"quoted-dollar.form", "quoted-dollar.json", "quoted-dollar.properties", "quoted-dollar.yaml",
		"single.env", "single.sh",
	}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("files = %v, want %v; findings=%+v", files, want, result.Findings)
	}
}

func TestOversizedCredentialTokensStillProduceFindings(t *testing.T) {
	root := t.TempDir()
	jsonSecret := strings.Repeat("J", 600)
	bearer := strings.Repeat("B", 1200)
	jwt := strings.Repeat("H", 1100) + "." + strings.Repeat("P", 30) + "." + strings.Repeat("S", 30)
	writeScanFile(t, root, "long.json", `{"client_secret":"`+jsonSecret+`"}`+"\n")
	writeScanFile(t, root, "bearer.txt", "Authorization: Bearer "+bearer+"\n")
	writeScanFile(t, root, "jwt.json", `{"access_token":"`+jwt+`"}`+"\n")

	result, err := WalkWithReport(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || len(result.Findings) != 3 {
		t.Fatalf("result = %+v", result)
	}
	rules := map[string]int{}
	for _, finding := range result.Findings {
		rules[finding.RuleID]++
	}
	if rules["secret.client"] != 1 || rules["secret.token"] != 1 || rules["secret.oauth"] != 1 {
		t.Fatalf("rules = %v", rules)
	}
	for _, secret := range []string{jsonSecret, bearer, jwt} {
		if strings.Contains(FormatText(result), secret) {
			t.Fatal("oversized secret leaked")
		}
	}
}

func TestSubdirectoryScanUsesProjectAndNestedIgnoreSemantics(t *testing.T) {
	root := t.TempDir()
	writeScanFile(t, root, ".camunda.yaml", "name: review\n")
	writeScanFile(t, root, ".gitignore", "sub/root-ignored/\nsub/reincluded/\n")
	writeScanFile(t, root, ".camunda-scanignore", "!sub/reincluded/**\n")
	writeScanFile(t, root, "sub/.gitignore", "nested-only/\nplain\n")
	for _, file := range []string{
		"sub/visible.env",
		"sub/root-ignored/a.env",
		"sub/reincluded/a.env",
		"sub/nested-only/a.env",
		"sub/plain/a.env",
	} {
		writeScanFile(t, root, file, "CLIENT_SECRET="+strings.ReplaceAll(file, "/", "-")+"-secret\n")
	}

	result, err := WalkWithReport(Options{Root: filepath.Join(root, "sub")})
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, finding := range result.Findings {
		files = append(files, finding.File)
	}
	if want := []string{"sub/reincluded/a.env", "sub/visible.env"}; !reflect.DeepEqual(files, want) {
		t.Fatalf("files = %v, want %v; result=%+v", files, want, result)
	}
	if result.Stats != (Stats{Discovered: 5, Scanned: 2, Ignored: 3}) {
		t.Fatalf("stats = %+v", result.Stats)
	}
}

func TestBuiltInPrunedTreesAreTerminalCandidates(t *testing.T) {
	root := t.TempDir()
	writeScanFile(t, root, ".camunda.yaml", "name: accounting\n")
	writeScanFile(t, root, "vendor/a.env", "CLIENT_SECRET=vendor-secret-value\n")
	writeScanFile(t, root, "vendor/deep/b.env", "CLIENT_SECRET=deep-vendor-secret\n")
	writeScanFile(t, root, "node_modules/a.js", "apiKey=module-secret-value\n")
	writeScanFile(t, root, "source/app.yaml", "name: clean\n")

	result, err := WalkWithReport(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if result.Stats != (Stats{Discovered: 4, Scanned: 2, Ignored: 2}) ||
		len(result.Issues) != 2 {
		t.Fatalf("bounded accounting = %+v", result)
	}
	if result.Issues[0].Path != "node_modules" || result.Issues[1].Path != "vendor" {
		t.Fatalf("pruned paths = %+v", result.Issues)
	}

	explicitFile, err := WalkWithReport(Options{Root: filepath.Join(root, "vendor", "a.env")})
	if err != nil {
		t.Fatal(err)
	}
	if explicitFile.Stats != (Stats{Discovered: 1, Ignored: 1}) {
		t.Fatalf("explicit file accounting = %+v", explicitFile)
	}
	explicitTree, err := WalkWithReport(Options{Root: filepath.Join(root, "node_modules")})
	if err != nil {
		t.Fatal(err)
	}
	if explicitTree.Stats != (Stats{Discovered: 1, Ignored: 1}) {
		t.Fatalf("explicit tree accounting = %+v", explicitTree)
	}
}

func TestExplicitEscapingRootSymlinkIsRejected(t *testing.T) {
	parent := t.TempDir()
	outside := t.TempDir()
	writeScanFile(t, outside, "outside.yaml", "CLIENT_SECRET=outside-secret-value\n")
	link := filepath.Join(parent, "scan-root")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	result, err := WalkWithReport(Options{Root: link})
	if err == nil || !strings.Contains(err.Error(), "root") || len(result.Findings) != 0 {
		t.Fatalf("result=%+v error=%v", result, err)
	}
}

func TestDescriptorTraversalRejectsParentAndFileSwaps(t *testing.T) {
	tests := []struct {
		name  string
		hooks func(root, outside string) walkHooks
	}{
		{
			name: "parent before open",
			hooks: func(root, outside string) walkHooks {
				return walkHooks{beforeOpen: func(path string) error {
					if path != "safe/clean.yaml" {
						return nil
					}
					if err := os.Rename(filepath.Join(root, "safe"), filepath.Join(root, "safe-held")); err != nil {
						return err
					}
					return os.Symlink(outside, filepath.Join(root, "safe"))
				}}
			},
		},
		{
			name: "file before open",
			hooks: func(root, outside string) walkHooks {
				return walkHooks{beforeOpen: func(path string) error {
					if path != "safe/clean.yaml" {
						return nil
					}
					if err := os.Remove(filepath.Join(root, "safe", "clean.yaml")); err != nil {
						return err
					}
					return os.Symlink(filepath.Join(outside, "outside.yaml"), filepath.Join(root, "safe", "clean.yaml"))
				}}
			},
		},
		{
			name: "file after open",
			hooks: func(root, outside string) walkHooks {
				return walkHooks{afterOpen: func(path string) error {
					if path != "safe/clean.yaml" {
						return nil
					}
					if err := os.Remove(filepath.Join(root, "safe", "clean.yaml")); err != nil {
						return err
					}
					return os.Symlink(filepath.Join(outside, "outside.yaml"), filepath.Join(root, "safe", "clean.yaml"))
				}}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			outside := t.TempDir()
			writeScanFile(t, root, ".camunda.yaml", "name: races\n")
			writeScanFile(t, root, "safe/clean.yaml", "name: clean\n")
			writeScanFile(t, outside, "outside.yaml", "CLIENT_SECRET=outside-secret-value\n")
			result, err := walkWithHooks(Options{Root: root}, test.hooks(root, outside))
			if err != nil {
				t.Fatal(err)
			}
			if result.Complete || len(result.Findings) != 0 || result.Stats.Errored != 1 {
				t.Fatalf("race escaped or was not reported: %+v", result)
			}
		})
	}
}
