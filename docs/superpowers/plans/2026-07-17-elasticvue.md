# ElasticVue Auto-Connect Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Auto-start ElasticVue on `:9800` for every Camunda lab profile that exposes Elasticsearch, preconfigured to the lab cluster so users open and see data with no manual setup.

**Architecture:** Thin Compose overlays (CORS patch + ElasticVue service), selected by `versions.HasHostElasticsearch`, synced via existing `overlay.ComposeOverrideFiles` embed pattern. CLI surfaces the URL through `urls` / `status` / `open`; smoke treats ElasticVue as optional.

**Tech Stack:** Go CLI, Docker Compose overlays, `cars10/elasticvue:1.15.0`, Camunda distributions.

## Global Constraints

- Do not rewrite Camunda official compose files — overlays only
- Host port for ElasticVue: `9800` → `http://localhost:9800`
- Predefined cluster URI must be `http://localhost:9200` (browser → host), not a Docker-internal hostname
- Image pin: `cars10/elasticvue:1.15.0` (not `latest`)
- Include when host ES exists: light 8.7–8.9, full all supported minors; exclude modeler and 8.10 light
- Align with existing `urls` elasticsearch listing rules
- Follow existing embed ↔ `overlays/` byte-parity test pattern
- Commits only when the user explicitly asks (skip commit steps unless requested)

---

## File map

| File | Responsibility |
|------|----------------|
| `internal/versions/adapter.go` | `HasHostElasticsearch(minor, profile bool)` |
| `internal/versions/adapter_test.go` | Table tests for that helper |
| `overlays/elasticsearch-cors.yaml` | CORS env on `elasticsearch` |
| `overlays/elasticvue.yaml` | ElasticVue service on `:9800` |
| `overlays/elasticsearch-8.10.yaml` | Add CORS env to our 8.10 sidecar ES |
| `internal/overlay/embed/*.yaml` | Embedded copies of the three overlays |
| `internal/overlay/overlay.go` | Sync + return override files |
| `internal/overlay/overlay_test.go` | Selection + parity tests |
| `internal/urls/urls.go` | Add `elasticvue` entry when host ES exists |
| `internal/urls/urls_test.go` | Presence/absence tests |
| `internal/lab/lab.go` | Include `elasticvue` in status Apps wanted list |
| `internal/cli/switch.go` | Include `elasticvue` in Web apps URL section |
| `internal/smoke/smoke.go` | Keep elasticvue optional (not in `smokeRequired`) |
| `docs/profiles.md`, `docs/cli-reference.md`, `README.md` | Document URL / behavior |
| `docs/superpowers/specs/2026-07-17-elasticvue-design.md` | Spec (already written) |

---

### Task 1: `HasHostElasticsearch`

**Files:**
- Modify: `internal/versions/adapter.go`
- Modify: `internal/versions/adapter_test.go`

**Interfaces:**
- Consumes: existing `ValidateMinor` / profile conventions
- Produces: `func HasHostElasticsearch(minor, profile string) bool`

- [ ] **Step 1: Write the failing test**

Add to `internal/versions/adapter_test.go`:

```go
func TestHasHostElasticsearch(t *testing.T) {
	cases := []struct {
		minor, profile string
		want           bool
	}{
		{"8.7", "light", true},
		{"8.7", "full", true},
		{"8.7", "modeler", false},
		{"8.8", "light", true},
		{"8.9", "light", true},
		{"8.9", "full", true},
		{"8.10", "light", false},
		{"8.10", "full", true},
		{"8.10", "modeler", false},
	}
	for _, tc := range cases {
		got := versions.HasHostElasticsearch(tc.minor, tc.profile)
		if got != tc.want {
			t.Fatalf("%s/%s: got %v want %v", tc.minor, tc.profile, got, tc.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/versions/ -run TestHasHostElasticsearch -v`  
Expected: FAIL — `HasHostElasticsearch` undefined

- [ ] **Step 3: Implement**

In `internal/versions/adapter.go`, after `NeedsElasticsearchOverlay`:

```go
// HasHostElasticsearch reports whether this profile publishes Elasticsearch on host :9200.
// Matches urls.List elasticsearch entries (no ES for modeler or 8.10 light).
func HasHostElasticsearch(minor, profile string) bool {
	if profile == "modeler" {
		return false
	}
	if minor == "8.10" && profile == "light" {
		return false
	}
	switch profile {
	case "light", "full":
		return true
	default:
		return false
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/versions/ -run TestHasHostElasticsearch -v`  
Expected: PASS

---

### Task 2: Overlay YAMLs (CORS + ElasticVue + 8.10 CORS)

**Files:**
- Create: `overlays/elasticsearch-cors.yaml`
- Create: `overlays/elasticvue.yaml`
- Create: `internal/overlay/embed/elasticsearch-cors.yaml`
- Create: `internal/overlay/embed/elasticvue.yaml`
- Modify: `overlays/elasticsearch-8.10.yaml`
- Modify: `internal/overlay/embed/elasticsearch-8.10.yaml` (keep byte-identical to repo overlay)

**Interfaces:**
- Consumes: Compose project network name `camunda-platform` (already used by 8.10 ES overlay)
- Produces: three overlay files on disk + embeds

- [ ] **Step 1: Write `overlays/elasticsearch-cors.yaml`**

```yaml
# camunda-lab — enable CORS so browser ElasticVue can reach Elasticsearch.
services:
  elasticsearch:
    environment:
      - http.cors.enabled=true
      - http.cors.allow-origin=/.*/
      - http.cors.allow-headers=X-Requested-With,Content-Type,Content-Length,Authorization
```

- [ ] **Step 2: Write `overlays/elasticvue.yaml`**

```yaml
# camunda-lab — ElasticVue UI preconfigured for the local Camunda Elasticsearch.
# Browser talks to http://localhost:9200; container serves UI on host :9800.
services:
  elasticvue:
    image: cars10/elasticvue:1.15.0
    container_name: camunda-lab-elasticvue
    ports:
      - "9800:8080"
    environment:
      ELASTICVUE_CLUSTERS: '[{"name":"camunda-lab","uri":"http://localhost:9200"}]'
    restart: unless-stopped
    networks:
      - camunda-platform
```

- [ ] **Step 3: Add CORS to `overlays/elasticsearch-8.10.yaml`**

Append these env entries to the existing `environment:` list (keep discovery/security/JAVA/cluster.name):

```yaml
      - http.cors.enabled=true
      - http.cors.allow-origin=/.*/
      - http.cors.allow-headers=X-Requested-With,Content-Type,Content-Length,Authorization
```

- [ ] **Step 4: Copy all three overlays into `internal/overlay/embed/`**

```bash
cp overlays/elasticsearch-cors.yaml internal/overlay/embed/
cp overlays/elasticvue.yaml internal/overlay/embed/
cp overlays/elasticsearch-8.10.yaml internal/overlay/embed/
```

---

### Task 3: Wire `ComposeOverrideFiles`

**Files:**
- Modify: `internal/overlay/overlay.go`
- Modify: `internal/overlay/overlay_test.go`

**Interfaces:**
- Consumes: `versions.HasHostElasticsearch`, `versions.NeedsElasticsearchOverlay`
- Produces: ordered absolute paths: optional `elasticsearch-8.10.yaml`, then `elasticsearch-cors.yaml`, then `elasticvue.yaml`

- [ ] **Step 1: Update failing tests**

Replace `TestComposeOverrideFiles810Full` and `TestComposeOverrideFiles89FullNone` with:

```go
func TestComposeOverrideFiles810Full(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CAMUNDA_LAB_HOME", home)
	paths.Reset()
	files, err := overlay.ComposeOverrideFiles("8.10", "full")
	if err != nil {
		t.Fatal(err)
	}
	bases := basenames(files)
	want := []string{"elasticsearch-8.10.yaml", "elasticsearch-cors.yaml", "elasticvue.yaml"}
	if len(bases) != len(want) {
		t.Fatalf("got %v want %v", bases, want)
	}
	for i := range want {
		if bases[i] != want[i] {
			t.Fatalf("got %v want %v", bases, want)
		}
	}
}

func TestComposeOverrideFiles89Full(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CAMUNDA_LAB_HOME", home)
	paths.Reset()
	files, err := overlay.ComposeOverrideFiles("8.9", "full")
	if err != nil {
		t.Fatal(err)
	}
	bases := basenames(files)
	want := []string{"elasticsearch-cors.yaml", "elasticvue.yaml"}
	if len(bases) != len(want) {
		t.Fatalf("got %v want %v", bases, want)
	}
	for i := range want {
		if bases[i] != want[i] {
			t.Fatalf("got %v want %v", bases, want)
		}
	}
}

func TestComposeOverrideFiles810LightNone(t *testing.T) {
	files, err := overlay.ComposeOverrideFiles("8.10", "light")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("%v", files)
	}
}

func TestComposeOverrideFilesModelerNone(t *testing.T) {
	files, err := overlay.ComposeOverrideFiles("8.9", "modeler")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("%v", files)
	}
}

func basenames(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = filepath.Base(p)
	}
	return out
}
```

Extend parity test to all three overlay pairs (or add `TestOverlaysInSync` looping names).

- [ ] **Step 2: Run tests — expect FAIL**

Run: `go test ./internal/overlay/ -v`  
Expected: FAIL (old expectations / missing embeds)

- [ ] **Step 3: Implement `overlay.go`**

```go
//go:embed embed/elasticsearch-8.10.yaml
var elasticsearch810YAML []byte

//go:embed embed/elasticsearch-cors.yaml
var elasticsearchCorsYAML []byte

//go:embed embed/elasticvue.yaml
var elasticvueYAML []byte

// ComposeOverrideFiles returns extra -f compose files (absolute paths).
func ComposeOverrideFiles(minor, profile string) ([]string, error) {
	if err := os.MkdirAll(paths.OverlaysDir(), 0o755); err != nil {
		return nil, err
	}
	var out []string
	write := func(name string, data []byte) error {
		dest := filepath.Join(paths.OverlaysDir(), name)
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			return err
		}
		out = append(out, dest)
		return nil
	}
	if versions.NeedsElasticsearchOverlay(minor, profile) {
		if err := write("elasticsearch-8.10.yaml", elasticsearch810YAML); err != nil {
			return nil, err
		}
	}
	if versions.HasHostElasticsearch(minor, profile) {
		if err := write("elasticsearch-cors.yaml", elasticsearchCorsYAML); err != nil {
			return nil, err
		}
		if err := write("elasticvue.yaml", elasticvueYAML); err != nil {
			return nil, err
		}
	}
	return out, nil
}
```

- [ ] **Step 4: Run tests — expect PASS**

Run: `go test ./internal/overlay/ ./internal/versions/ -v`  
Expected: PASS

---

### Task 4: URLs + status + CLI web apps

**Files:**
- Modify: `internal/urls/urls.go`
- Modify: `internal/urls/urls_test.go`
- Modify: `internal/lab/lab.go` (`summarizeURLs` wanted list)
- Modify: `internal/cli/switch.go` (`webApps` list)
- Modify: `internal/smoke/smoke.go` — confirm `elasticvue` is **not** in `smokeRequired` (no code change if already optional by default)

**Interfaces:**
- Consumes: `versions.HasHostElasticsearch` **or** mirror the same rules inline in `urls` (prefer calling `versions.HasHostElasticsearch` from `urls` to avoid drift)
- Produces: `elasticvue` → `http://<host>:9800`

- [ ] **Step 1: Failing URL tests**

```go
func TestElasticvueWhenHostES(t *testing.T) {
	mustURL(t, config.Config{Version: "8.9", Profile: "light", Host: "localhost"},
		"elasticvue", "http://localhost:9800")
	mustURL(t, config.Config{Version: "8.10", Profile: "full", Host: "localhost"},
		"elasticvue", "http://localhost:9800")
	if _, err := urls.Find(config.Config{Version: "8.10", Profile: "light", Host: "localhost"}, "elasticvue"); err == nil {
		t.Fatal("8.10 light should not list elasticvue")
	}
	if _, err := urls.Find(config.Config{Version: "8.9", Profile: "modeler", Host: "localhost"}, "elasticvue"); err == nil {
		t.Fatal("modeler should not list elasticvue")
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

Run: `go test ./internal/urls/ -run TestElasticvueWhenHostES -v`

- [ ] **Step 3: Implement URL entries**

In `lightEntries` / `fullEntries`, after elasticsearch append (or whenever `versions.HasHostElasticsearch(version, profile)` — for light/full helpers the profile is implied):

Helper in `urls.go`:

```go
func maybeElasticvue(host string, entries []Entry) []Entry {
	return append(entries, Entry{
		Name:  "elasticvue",
		URL:   fmt.Sprintf("http://%s:9800", host),
		Notes: "ElasticVue — cluster camunda-lab preconfigured",
	})
}
```

Call it at the end of light/full entry builders **only when** elasticsearch is included for that matrix (same conditions as ES today): light if `version != "8.10"`; full always. Or:

```go
if versions.HasHostElasticsearch(version, "light") { ... }
```

inside `lightEntries`, and `HasHostElasticsearch(version, "full")` inside `fullEntries`.

- [ ] **Step 4: Status + CLI lists**

In `internal/lab/lab.go` `summarizeURLs` wanted slice, add `"elasticvue"` near web apps (after `keycloak` or before connectors).

In `internal/cli/switch.go` `webApps` slice, add `"elasticvue"`.

- [ ] **Step 5: Run tests**

Run: `go test ./internal/urls/ ./internal/lab/ ./internal/cli/ -count=1`  
Expected: PASS

---

### Task 5: Docs

**Files:**
- Modify: `docs/profiles.md` — add ElasticVue row (`http://localhost:9800`) where ES is listed
- Modify: `docs/cli-reference.md` — note `open elasticvue`, port 9800
- Modify: `README.md` — one bullet under features/URLs
- Modify: `docs/troubleshooting.md` — if ElasticVue connects but cluster empty: CORS / ES not published

- [ ] **Step 1: Update docs with concrete rows**

Example profiles row:

`| ElasticVue | http://localhost:9800 (preconfigured → localhost:9200) |`

- [ ] **Step 2: Skim for accuracy against Task 1 matrix**

---

### Task 6: Local verify (optional LIVE)

**Files:** none (manual)

- [ ] **Step 1: Rebuild CLI**

```bash
cd /Users/nasr/homelab/camunda-lab
go test ./...
go build -o "$HOME/.local/bin/camunda" ./cmd/camunda
```

- [ ] **Step 2: Recreate stack so overlays apply**

```bash
camunda down
camunda up
camunda wait --timeout 10m
camunda urls
camunda open elasticvue
```

Expected:
- `elasticvue -> http://localhost:9800` in urls
- Browser shows cluster `camunda-lab` without manual add
- Cluster connects to indices (Operate/Zeebe data may appear after traffic)

- [ ] **Step 3: Negative check**

Document-only or quick thought: 8.10 light / modeler must not start elasticvue container (`docker compose ps` / `camunda urls` without elasticvue).

---

## Spec coverage check

| Spec requirement | Task |
|------------------|------|
| Auto when host ES | 1, 3 |
| Port 9800 + ELASTICVUE_CLUSTERS | 2 |
| CORS on ES | 2, 3 |
| 8.10 sidecar CORS | 2 |
| urls / status / open | 4 |
| smoke optional | 4 (confirm) |
| Docs | 5 |
| Success criteria LIVE | 6 |

## Placeholder / consistency scan

- Image tag consistently `cars10/elasticvue:1.15.0`
- Helper name consistently `HasHostElasticsearch`
- Overlay order: ES 8.10 → CORS → ElasticVue
- Cluster name consistently `camunda-lab`
- URI consistently `http://localhost:9200`
