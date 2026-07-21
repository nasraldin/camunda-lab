package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nasraldin/camunda-lab/internal/backup"
	"github.com/nasraldin/camunda-lab/internal/bpmn"
	"github.com/nasraldin/camunda-lab/internal/cluster"
	"github.com/nasraldin/camunda-lab/internal/config"
	bpmdiff "github.com/nasraldin/camunda-lab/internal/diff"
	"github.com/nasraldin/camunda-lab/internal/doctor"
	"github.com/nasraldin/camunda-lab/internal/drift"
	"github.com/nasraldin/camunda-lab/internal/env"
	"github.com/nasraldin/camunda-lab/internal/explain"
	"github.com/nasraldin/camunda-lab/internal/incidents"
	"github.com/nasraldin/camunda-lab/internal/k8s"
	"github.com/nasraldin/camunda-lab/internal/lint"
	"github.com/nasraldin/camunda-lab/internal/paths"
	"github.com/nasraldin/camunda-lab/internal/plan"
	"github.com/nasraldin/camunda-lab/internal/project"
	"github.com/nasraldin/camunda-lab/internal/review"
	"github.com/nasraldin/camunda-lab/internal/scan"
	"github.com/nasraldin/camunda-lab/internal/testgen"
	"github.com/nasraldin/camunda-lab/internal/trace"
)

func registerToolkit(mux *http.ServeMux, h *handler) {
	mux.HandleFunc("POST /api/v1/bpmn/lint", h.bpmnLint)
	mux.HandleFunc("POST /api/v1/bpmn/diff", h.bpmnDiff)
	mux.HandleFunc("POST /api/v1/bpmn/explain", h.bpmnExplain)
	mux.HandleFunc("POST /api/v1/bpmn/review", h.bpmnReview)
	mux.HandleFunc("POST /api/v1/bpmn/test-generate", h.bpmnTestGenerate)
	mux.HandleFunc("POST /api/v1/bpmn/scan", h.bpmnScan)

	mux.HandleFunc("GET /api/v1/incidents", h.listIncidents)
	mux.HandleFunc("POST /api/v1/incidents/{key}/retry", h.retryIncident)
	mux.HandleFunc("GET /api/v1/trace/{instanceKey}", h.traceInstance)
	mux.HandleFunc("POST /api/v1/plan", h.runPlan)
	mux.HandleFunc("POST /api/v1/drift", h.runDrift)
	mux.HandleFunc("GET /api/v1/doctor/deep", h.runDoctorDeep)

	mux.HandleFunc("POST /api/v1/project/init", h.projectInit)
	mux.HandleFunc("GET /api/v1/env", h.envList)
	mux.HandleFunc("POST /api/v1/env/use", h.envUse)
	mux.HandleFunc("POST /api/v1/env", h.envAdd)
	mux.HandleFunc("DELETE /api/v1/env/{name}", h.envRemove)
	mux.HandleFunc("POST /api/v1/backup", h.runBackup)
	mux.HandleFunc("POST /api/v1/restore", h.runRestore)

	mux.HandleFunc("GET /api/v1/k8s/status", h.k8sStatus)
	mux.HandleFunc("GET /api/v1/k8s/logs/{component}", h.k8sLogs)
	mux.HandleFunc("POST /api/v1/k8s/restart", h.k8sRestart)
	mux.HandleFunc("POST /api/v1/k8s/scale", h.k8sScale)
}

// --- BPMN helpers ---

func writeTempBPMN(r *http.Request, field string) (path string, cleanup func(), err error) {
	cleanup = func() {}
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		// fall through — may be JSON
	}
	if r.MultipartForm != nil {
		files := r.MultipartForm.File[field]
		if len(files) > 0 {
			fh := files[0]
			if fh.Size > maxUploadBytes {
				return "", cleanup, fmt.Errorf("file too large (max 10MB)")
			}
			src, err := fh.Open()
			if err != nil {
				return "", cleanup, err
			}
			defer src.Close()
			tmp, err := os.CreateTemp("", "camunda-lab-*.bpmn")
			if err != nil {
				return "", cleanup, err
			}
			if _, err := io.Copy(tmp, io.LimitReader(src, maxUploadBytes+1)); err != nil {
				_ = tmp.Close()
				_ = os.Remove(tmp.Name())
				return "", cleanup, err
			}
			_ = tmp.Close()
			cleanup = func() { _ = os.Remove(tmp.Name()) }
			return tmp.Name(), cleanup, nil
		}
	}
	return "", cleanup, fmt.Errorf("missing upload field %q (or use paths in JSON)", field)
}

func parsePathsJSON(r *http.Request) (paths []string, extra map[string]any, err error) {
	extra = map[string]any{}
	ct := r.Header.Get("Content-Type")
	if strings.Contains(ct, "multipart/form-data") {
		return nil, extra, nil
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxUploadBytes))
	if err != nil {
		return nil, nil, err
	}
	if len(body) == 0 {
		return nil, extra, nil
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, nil, err
	}
	extra = raw
	switch v := raw["paths"].(type) {
	case []any:
		for _, x := range v {
			if s, ok := x.(string); ok && s != "" {
				paths = append(paths, s)
			}
		}
	}
	if p, ok := raw["path"].(string); ok && p != "" {
		paths = append(paths, p)
	}
	if d, ok := raw["dir"].(string); ok && d != "" {
		extra["dir"] = d
	}
	return paths, extra, nil
}

func resolveAllowedPaths(in []string) ([]string, error) {
	var out []string
	for _, p := range in {
		abs, err := allowPath(p)
		if err != nil {
			return nil, err
		}
		out = append(out, abs)
	}
	return out, nil
}

func (h *handler) bpmnLint(w http.ResponseWriter, r *http.Request) {
	failOn := "error"
	var files []string
	var cleanups []func()
	defer func() {
		for _, c := range cleanups {
			c()
		}
	}()

	pathsIn, extra, err := parsePathsJSON(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if s, ok := extra["failOn"].(string); ok && s != "" {
		failOn = s
	}
	if len(pathsIn) > 0 {
		files, err = resolveAllowedPaths(pathsIn)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
	} else {
		// multipart: file or files
		_ = r.ParseMultipartForm(maxUploadBytes)
		if r.MultipartForm != nil {
			for _, field := range []string{"file", "files"} {
				for _, fh := range r.MultipartForm.File[field] {
					if fh.Size > maxUploadBytes {
						writeErr(w, http.StatusBadRequest, fmt.Errorf("file too large"))
						return
					}
					src, err := fh.Open()
					if err != nil {
						writeErr(w, http.StatusBadRequest, err)
						return
					}
					tmp, err := os.CreateTemp("", "camunda-lab-*.bpmn")
					if err != nil {
						_ = src.Close()
						writeErr(w, http.StatusInternalServerError, err)
						return
					}
					_, _ = io.Copy(tmp, io.LimitReader(src, maxUploadBytes))
					_ = src.Close()
					_ = tmp.Close()
					name := tmp.Name()
					cleanups = append(cleanups, func() { _ = os.Remove(name) })
					files = append(files, name)
				}
			}
		}
	}
	if len(files) == 0 {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("provide BPMN upload(s) or JSON paths[]"))
		return
	}

	var all []lint.Finding
	for _, f := range files {
		m, err := bpmn.ParseFile(f)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		label := f
		all = append(all, lint.Run(m, lint.Options{File: label})...)
	}
	ok := !lint.ShouldFail(all, failOn)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       ok,
		"findings": all,
		"output":   lint.FormatText(all),
		"cli":      "camunda lint <file.bpmn>",
	})
}

func (h *handler) bpmnDiff(w http.ResponseWriter, r *http.Request) {
	var fromPath, toPath string
	var cleanups []func()
	defer func() {
		for _, c := range cleanups {
			c()
		}
	}()

	pathsIn, extra, err := parsePathsJSON(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if len(pathsIn) >= 2 {
		resolved, err := resolveAllowedPaths(pathsIn[:2])
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		fromPath, toPath = resolved[0], resolved[1]
	} else {
		if f, ok := extra["from"].(string); ok {
			fromPath, err = allowPath(f)
			if err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
		}
		if t, ok := extra["to"].(string); ok {
			toPath, err = allowPath(t)
			if err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
		}
	}
	if fromPath == "" || toPath == "" {
		_ = r.ParseMultipartForm(maxUploadBytes)
		p1, c1, err1 := writeTempBPMN(r, "from")
		p2, c2, err2 := writeTempBPMN(r, "to")
		if err1 == nil {
			fromPath = p1
			cleanups = append(cleanups, c1)
		}
		if err2 == nil {
			toPath = p2
			cleanups = append(cleanups, c2)
		}
		// also try fileA / fileB
		if fromPath == "" {
			p, c, e := writeTempBPMN(r, "fileA")
			if e == nil {
				fromPath = p
				cleanups = append(cleanups, c)
			}
		}
		if toPath == "" {
			p, c, e := writeTempBPMN(r, "fileB")
			if e == nil {
				toPath = p
				cleanups = append(cleanups, c)
			}
		}
	}
	if fromPath == "" || toPath == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("need two BPMN files (from/to uploads or paths)"))
		return
	}
	a, err := bpmn.ParseFile(fromPath)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	b, err := bpmn.ParseFile(toPath)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	changes := bpmdiff.Compare(a, b)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      len(changes) == 0,
		"changes": changes,
		"output":  bpmdiff.FormatText(changes),
		"cli":     "camunda diff a.bpmn --against b.bpmn",
	})
}

func (h *handler) bpmnExplain(w http.ResponseWriter, r *http.Request) {
	path, cleanup, _, err := h.oneBPMN(r)
	defer cleanup()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	m, err := bpmn.ParseFile(path)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	md := explain.Offline(m).Markdown()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"markdown": md,
		"output":   md,
		"cli":      "camunda explain <file.bpmn>",
	})
}

func (h *handler) bpmnReview(w http.ResponseWriter, r *http.Request) {
	failOn := "error"
	aiFlag := false
	path, cleanup, extra, err := h.oneBPMN(r)
	defer cleanup()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if s, ok := extra["failOn"].(string); ok && s != "" {
		failOn = s
	}
	if b, ok := extra["ai"].(bool); ok {
		aiFlag = b
	}
	if r.MultipartForm != nil {
		if v := r.FormValue("failOn"); v != "" {
			failOn = v
		}
		if r.FormValue("ai") == "true" {
			aiFlag = true
		}
	}
	m, err := bpmn.ParseFile(path)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	res, err := review.Run(m, review.Options{File: path, FailOn: failOn, AI: aiFlag})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	ok := !lint.ShouldFail(res.Findings, failOn)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       ok,
		"findings": res.Findings,
		"output":   review.FormatText(res),
		"cli":      "camunda review <file.bpmn>",
	})
}

func (h *handler) bpmnTestGenerate(w http.ResponseWriter, r *http.Request) {
	lang := "java"
	outDir := ""
	force := false
	path, cleanup, extra, err := h.oneBPMN(r)
	defer cleanup()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if s, ok := extra["lang"].(string); ok && s != "" {
		lang = s
	}
	if s, ok := extra["output"].(string); ok && s != "" {
		outDir, err = allowPath(s)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
	}
	if b, ok := extra["force"].(bool); ok {
		force = b
	}
	if r.MultipartForm != nil {
		if v := r.FormValue("lang"); v != "" {
			lang = v
		}
		if v := r.FormValue("output"); v != "" {
			outDir, err = allowPath(v)
			if err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
		}
		if r.FormValue("force") == "true" {
			force = true
		}
	}
	if outDir == "" {
		tmp, err := os.MkdirTemp("", "camunda-lab-tests-*")
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		outDir = tmp
	}
	m, err := bpmn.ParseFile(path)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	written, err := testgen.Generate(m, testgen.Options{Lang: lang, Force: force, OutDir: outDir})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	contents := map[string]string{}
	for _, p := range written {
		data, _ := os.ReadFile(p)
		contents[p] = string(data)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"paths":    written,
		"contents": contents,
		"output":   strings.Join(written, "\n"),
		"cli":      "camunda test generate <file.bpmn> -o <dir>",
	})
}

func (h *handler) bpmnScan(w http.ResponseWriter, r *http.Request) {
	failOn := "medium"
	dir := ""
	ct := r.Header.Get("Content-Type")
	if strings.Contains(ct, "multipart/form-data") {
		_ = r.ParseMultipartForm(maxUploadBytes)
		dir = r.FormValue("dir")
		if v := r.FormValue("failOn"); v != "" {
			failOn = v
		}
	} else {
		_, extra, err := parsePathsJSON(r)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if d, ok := extra["dir"].(string); ok {
			dir = d
		}
		if s, ok := extra["failOn"].(string); ok && s != "" {
			failOn = s
		}
	}
	if dir == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("dir is required (absolute project path)"))
		return
	}
	abs, err := allowPath(dir)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	fs, err := scan.Walk(scan.Options{Root: abs, FailOn: failOn})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	ok := !scan.ShouldFail(fs, failOn)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       ok,
		"findings": fs,
		"output":   scan.FormatText(fs),
		"cli":      "camunda scan <dir>",
	})
}

func (h *handler) oneBPMN(r *http.Request) (string, func(), map[string]any, error) {
	pathsIn, extra, err := parsePathsJSON(r)
	if err != nil {
		return "", func() {}, extra, err
	}
	if len(pathsIn) > 0 {
		abs, err := allowPath(pathsIn[0])
		return abs, func() {}, extra, err
	}
	path, cleanup, err := writeTempBPMN(r, "file")
	return path, cleanup, extra, err
}

// --- Cluster ---

func (h *handler) clusterClient(r *http.Request) (*cluster.Client, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	return cluster.NewFromLab(paths.Home(), cfg)
}

func (h *handler) listIncidents(w http.ResponseWriter, r *http.Request) {
	cl, err := h.clusterClient(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	raw, err := cl.SearchIncidents(r.Context(), 50)
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("incidents search: %w", err))
		return
	}
	items := make([]incidents.Incident, 0, len(raw))
	for _, it := range raw {
		created, _ := time.Parse(time.RFC3339, it.CreationTime)
		items = append(items, incidents.Incident{
			ID:        it.Key,
			Created:   created,
			Error:     it.ErrorMessage,
			Process:   it.ProcessDefinitionID,
			JobWorker: it.ElementID,
			Key:       it.ProcessInstanceKey,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"items":  items,
		"output": incidents.FormatTable(items),
		"cli":    "camunda incidents list",
	})
}

func (h *handler) retryIncident(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Confirm bool `json:"confirm"`
		Yes     bool `json:"yes"`
	}
	_ = decodeJSON(r, &body)
	if !body.Confirm && !body.Yes {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("refusing retry without confirm/yes"))
		return
	}
	key := r.PathValue("key")
	cl, err := h.clusterClient(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := cl.ResolveIncident(r.Context(), key); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "cli": "camunda incidents retry " + key + " --yes"})
}

func (h *handler) traceInstance(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("instanceKey")
	cl, err := h.clusterClient(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	inst, err := cl.GetProcessInstance(r.Context(), key)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	els, err := cl.SearchElementInstances(r.Context(), key, 200)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	tl := trace.Timeline{InstanceKey: key, State: inst.State}
	for _, el := range els {
		st := el.State
		if el.IncidentKey != "" {
			st = "INCIDENT"
		}
		name := el.ElementName
		if name == "" {
			name = el.ElementID
		}
		tl.Steps = append(tl.Steps, trace.Step{Name: name, State: st, Detail: el.Type})
	}
	out := trace.RenderASCII(tl)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"timeline": tl,
		"output":   out,
		"cli":      "camunda trace " + key,
	})
}

func (h *handler) runPlan(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Dir string `json:"dir"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	root, err := allowPath(body.Dir)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if _, err := os.Stat(filepath.Join(root, project.ConfigFileName)); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("no .camunda.yaml in %s — run Project → Init first", root))
		return
	}
	local, err := plan.LocalInventory(root)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	cl, err := h.clusterClient(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	remote, err := cl.RemoteInventory(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("cluster inventory: %w", err))
		return
	}
	active, err := env.GetActive(paths.Home())
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	p := plan.Build(active, local, remote)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"plan":   p,
		"output": plan.FormatText(p),
		"cli":    "camunda plan --dir " + root,
	})
}

func (h *handler) runDrift(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Dir string `json:"dir"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	root, err := allowPath(body.Dir)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if _, err := os.Stat(filepath.Join(root, project.ConfigFileName)); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("no .camunda.yaml in %s", root))
		return
	}
	local, err := plan.LocalInventory(root)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	cl, err := h.clusterClient(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	remote, err := cl.RemoteInventory(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("cluster inventory: %w", err))
		return
	}
	active, err := env.GetActive(paths.Home())
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	res := drift.Compare(active, local, remote)
	ok := !drift.HasDrift(res)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     ok,
		"drift":  res,
		"output": drift.FormatText(res),
		"cli":    "camunda drift --dir " + root,
	})
}

func (h *handler) runDoctorDeep(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	base := doctor.Run(false)
	sections := doctor.Deep(r.Context(), cfg, doctor.DeepOptions{})
	report := doctor.FormatDeep(base, sections)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       doctor.DeepOK(sections) && base.OK,
		"report":   report,
		"sections": sections,
		"cli":      "camunda doctor --deep",
	})
}

// --- Project / env / backup / k8s ---

func (h *handler) projectInit(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Dir       string `json:"dir"`
		Name      string `json:"name"`
		Version   string `json:"version"`
		Profile   string `json:"profile"`
		Resources string `json:"resources"`
		Force     bool   `json:"force"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	dir, err := allowPath(body.Dir)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := project.Scaffold(project.ScaffoldOpts{
		Dir: dir, Name: body.Name, Version: body.Version,
		Profile: body.Profile, Resources: body.Resources, Force: body.Force,
	}); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"dir":  dir,
		"cli":  "camunda init " + dir + " -y",
		"hint": "Project scaffolded. Add BPMN under bpmn/ then use Cluster → Plan.",
	})
}

func (h *handler) envList(w http.ResponseWriter, r *http.Request) {
	active, err := env.GetActive(paths.Home())
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	ps, err := env.ListProfiles(filepath.Join(paths.Home(), "envs"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"active":   active,
		"profiles": append([]env.Profile{env.DefaultLab()}, ps...),
		"cli":      "camunda env list",
	})
}

func (h *handler) envUse(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &body); err != nil || body.Name == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("name is required"))
		return
	}
	if err := env.SetActive(paths.Home(), filepath.Join(paths.Home(), "envs"), body.Name); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "active": body.Name, "cli": "camunda env use " + body.Name})
}

func (h *handler) envAdd(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name            string `json:"name"`
		Kind            string `json:"kind"`
		Orchestration   string `json:"orchestration"`
		ClientIDEnv     string `json:"clientIdEnv"`
		ClientSecretEnv string `json:"clientSecretEnv"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if body.Name == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("name is required"))
		return
	}
	if body.Kind == "" {
		body.Kind = "remote"
	}
	if body.ClientIDEnv == "" {
		body.ClientIDEnv = "CAMUNDA_CLIENT_ID"
	}
	if body.ClientSecretEnv == "" {
		body.ClientSecretEnv = "CAMUNDA_CLIENT_SECRET"
	}
	p := env.Profile{
		Name: body.Name, Kind: body.Kind,
		Endpoints: map[string]string{},
		Auth:      env.AuthRefs{ClientIDEnv: body.ClientIDEnv, ClientSecretEnv: body.ClientSecretEnv},
	}
	if body.Orchestration != "" {
		p.Endpoints["orchestration"] = body.Orchestration
	}
	if err := env.SaveProfile(filepath.Join(paths.Home(), "envs"), p); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "profile": p, "cli": "camunda env add " + body.Name})
}

func (h *handler) envRemove(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := env.RemoveProfile(paths.Home(), filepath.Join(paths.Home(), "envs"), name); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "cli": "camunda env remove " + name})
}

func (h *handler) runBackup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Output         string `json:"output"`
		IncludeSecrets bool   `json:"includeSecrets"`
		Dir            string `json:"dir"`
	}
	_ = decodeJSON(r, &body)
	out := body.Output
	if out == "" {
		out = filepath.Join(os.TempDir(), fmt.Sprintf("camunda-lab-backup-%s.tar.gz", time.Now().Format("20060102-150405")))
	} else {
		var err error
		out, err = allowPath(out)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
	}
	cfg, _ := config.Load()
	proj := body.Dir
	if proj != "" {
		var err error
		proj, err = allowPath(proj)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
	}
	m, err := backup.Create(backup.Options{
		LabHome: paths.Home(), ProjectDir: proj, OutPath: out,
		IncludeSecrets: body.IncludeSecrets, LabVersion: cfg.Version, LabProfile: cfg.Profile,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":              true,
		"path":            out,
		"files":           len(m.Files),
		"includesSecrets": m.IncludesSecrets,
		"cli":             "camunda backup -o " + out,
	})
}

func (h *handler) runRestore(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxUploadBytes * 5); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if r.FormValue("yes") != "true" && r.FormValue("confirm") != "true" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("refusing restore without yes/confirm"))
		return
	}
	fh, hdr, err := r.FormFile("archive")
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("archive file required"))
		return
	}
	defer fh.Close()
	tmp, err := os.CreateTemp("", "camunda-lab-restore-*.tar.gz")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, io.LimitReader(fh, maxUploadBytes*5)); err != nil {
		_ = tmp.Close()
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	_ = tmp.Close()
	proj := r.FormValue("dir")
	if proj != "" {
		proj, err = allowPath(proj)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
	}
	m, err := backup.Restore(r.Context(), backup.RestoreOptions{
		ArchivePath: tmp.Name(),
		LabHome:     paths.Home(),
		ProjectDir:  proj,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	_ = hdr
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"files": len(m.Files),
		"cli":   "camunda restore <archive.tar.gz> --yes",
	})
}

func (h *handler) k8sStatus(w http.ResponseWriter, r *http.Request) {
	out, err := k8s.Status(k8s.Options{})
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": false, "output": out, "error": err.Error(),
			"hint": "Kubernetes helpers need kubectl + a Camunda Helm release. Skip on Docker Compose labs.",
			"cli":  "camunda k8s status",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "output": out, "cli": "camunda k8s status"})
}

func (h *handler) k8sLogs(w http.ResponseWriter, r *http.Request) {
	comp := r.PathValue("component")
	out, err := k8s.Logs(k8s.Options{}, comp, false, 100)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "output": out, "cli": "camunda k8s logs " + comp})
}

func (h *handler) k8sRestart(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Component string `json:"component"`
		Confirm   bool   `json:"confirm"`
		Yes       bool   `json:"yes"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if !body.Confirm && !body.Yes {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("refusing restart without confirm"))
		return
	}
	out, err := k8s.Restart(k8s.Options{}, body.Component)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "output": out, "cli": "camunda k8s restart " + body.Component + " --yes"})
}

func (h *handler) k8sScale(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Component string `json:"component"`
		Replicas  int    `json:"replicas"`
		Confirm   bool   `json:"confirm"`
		Yes       bool   `json:"yes"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if !body.Confirm && !body.Yes {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("refusing scale without confirm"))
		return
	}
	if body.Replicas < 0 {
		body.Replicas = 1
	}
	out, err := k8s.Scale(k8s.Options{}, body.Component, body.Replicas)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "output": out, "cli": "camunda k8s scale " + body.Component})
}
