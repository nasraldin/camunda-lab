package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
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
	"github.com/nasraldin/camunda-lab/internal/lint"
	"github.com/nasraldin/camunda-lab/internal/paths"
	"github.com/nasraldin/camunda-lab/internal/plan"
	"github.com/nasraldin/camunda-lab/internal/project"
	"github.com/nasraldin/camunda-lab/internal/review"
	"github.com/nasraldin/camunda-lab/internal/scan"
	"github.com/nasraldin/camunda-lab/internal/toolkit"
	"github.com/nasraldin/camunda-lab/internal/trace"
)

func registerToolkit(mux *http.ServeMux, h *handler) {
	mux.HandleFunc("POST /api/v1/bpmn/lint", h.bpmnLint)
	mux.HandleFunc("POST /api/v1/bpmn/diff", h.bpmnDiff)
	mux.HandleFunc("POST /api/v1/bpmn/explain", h.bpmnExplain)
	mux.HandleFunc("POST /api/v1/bpmn/review", h.bpmnReview)
	mux.HandleFunc("POST /api/v1/bpmn/test-generate", h.bpmnTestGenerate)
	mux.HandleFunc("POST /api/v1/bpmn/test-generate/download", h.bpmnTestGenerateDownload)
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
	mux.HandleFunc("POST /api/v1/backup/download", h.runBackupDownload)
	mux.HandleFunc("POST /api/v1/restore", h.runRestore)
}

func (h *handler) backupService() *backup.Service {
	return backup.NewService(h.labChecker())
}

func (h *handler) createBackup(ctx context.Context, opts backup.Options) (backup.Manifest, error) {
	if h != nil && h.backupCreate != nil {
		return h.backupCreate(ctx, opts)
	}
	return h.backupService().Create(ctx, opts)
}

func (h *handler) labChecker() backup.RunningChecker {
	if h != nil && h.runningLab != nil {
		return h.runningLab
	}
	if h != nil && h.lab != nil {
		return h.lab
	}
	return nil
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

func (h *handler) legacyBpmnLint(w http.ResponseWriter, r *http.Request) {
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
		all = append(all, lint.Run(m, lint.Options{File: label, FailOn: failOn}).Findings...)
	}
	ok := !lint.ShouldFail(all, failOn)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       ok,
		"findings": all,
		"output": lint.FormatText(lint.Result{
			Failed: !ok, Findings: all,
		}),
		"cli": "camunda lint <file.bpmn>",
	})
}

func (h *handler) legacyBpmnDiff(w http.ResponseWriter, r *http.Request) {
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

func (h *handler) legacyBpmnExplain(w http.ResponseWriter, r *http.Request) {
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

func (h *handler) legacyBpmnReview(w http.ResponseWriter, r *http.Request) {
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

func (h *handler) legacyBpmnTestGenerate(w http.ResponseWriter, r *http.Request) {
	path, lang, outDir, force, cleanup, err := decodeGenerateRequest(r)
	defer cleanup()
	if err != nil {
		writeGenerateError(w, err)
		return
	}
	result, err := (toolkit.Service{}).Generate(r.Context(), toolkit.GenerateRequest{
		Input:  toolkit.BPMNInput{Path: path},
		OutDir: outDir,
		Lang:   toolkit.GenerateLanguage(lang),
		Force:  force,
	})
	if err != nil {
		writeGenerateError(w, err)
		return
	}
	paths := make([]string, len(result.Artifacts))
	contents := map[string]string{}
	for index, artifact := range result.Artifacts {
		paths[index] = artifact.Path
		contents[artifact.Path] = string(artifact.Content)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"paths":    paths,
		"contents": contents,
		"output":   strings.Join(paths, "\n"),
		"cli":      "camunda test generate <file.bpmn> -o <dir>",
	})
}

type generateRequestDTO struct {
	Path   string `json:"path"`
	Lang   string `json:"lang,omitempty"`
	Output string `json:"output,omitempty"`
	Force  bool   `json:"force,omitempty"`
}

func decodeGenerateRequest(r *http.Request) (
	path string,
	lang string,
	outDir string,
	force bool,
	cleanup func(),
	err error,
) {
	cleanup = func() {}
	lang = "java"
	if strings.Contains(r.Header.Get("Content-Type"), "multipart/form-data") {
		path, cleanup, err = writeTempBPMN(r, "file")
		if err != nil {
			return
		}
		if value := r.FormValue("lang"); value != "" {
			lang = value
		}
		if value := r.FormValue("output"); value != "" {
			outDir, err = allowPath(value)
			if err != nil {
				return
			}
		}
		switch value := r.FormValue("force"); value {
		case "", "false":
		case "true":
			force = true
		default:
			err = fmt.Errorf("force must be true or false")
		}
		return
	}

	defer r.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(r.Body, maxUploadBytes))
	decoder.DisallowUnknownFields()
	var body generateRequestDTO
	if err = decoder.Decode(&body); err != nil {
		return
	}
	var trailing json.RawMessage
	if trailingErr := decoder.Decode(&trailing); trailingErr != io.EOF {
		if trailingErr == nil {
			err = errors.New("request body must contain one JSON object")
		} else {
			err = trailingErr
		}
		return
	}
	path, err = allowPath(body.Path)
	if err != nil {
		return
	}
	if body.Lang != "" {
		lang = body.Lang
	}
	if body.Output != "" {
		outDir, err = allowPath(body.Output)
		if err != nil {
			return
		}
	}
	force = body.Force
	return
}

func writeGenerateError(w http.ResponseWriter, err error) {
	writeMappedErr(w, err)
}

func (h *handler) legacyBpmnScan(w http.ResponseWriter, r *http.Request) {
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

func (h *handler) clusterClient(r *http.Request, projectRoot string) (*cluster.Client, env.Resolved, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, env.Resolved{}, err
	}
	factory := h.clusterFactory
	if factory == nil {
		factory = cluster.NewFactory(paths.Home(), cfg)
	}
	return factory.Client(r.Context(), "", projectRoot)
}

func (h *handler) listIncidents(w http.ResponseWriter, r *http.Request) {
	root, err := optionalAuthorizedDir(r.URL.Query().Get("dir"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	environment := strings.TrimSpace(r.URL.Query().Get("environment"))
	limit := 50
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		n, parseErr := strconv.Atoi(raw)
		if parseErr != nil || n < 1 {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid limit"))
			return
		}
		limit = n
	}
	request := incidents.ListRequest{
		Environment: environment,
		ProjectRoot: root,
		Limit:       limit,
		Filter:      incidents.ListFilter{State: "ACTIVE"},
	}
	var result incidents.Result
	if h != nil && h.listIncidentsFn != nil {
		result, err = h.listIncidentsFn(r.Context(), request)
	} else {
		cfg, loadErr := config.Load()
		if loadErr != nil {
			writeErr(w, http.StatusBadRequest, loadErr)
			return
		}
		factory := h.clusterFactory
		if factory == nil {
			factory = cluster.NewFactory(paths.Home(), cfg)
		}
		result, err = incidents.NewService(factory).List(r.Context(), request)
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("incidents search: %w", err))
		return
	}
	cli := "camunda incidents list"
	if root != "" {
		cli += " (project " + root + ")"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"items":  incidents.FormatAPIItems(result.Incidents),
		"result": result,
		"output": incidents.FormatText(result),
		"cli":    cli,
	})
}

func (h *handler) retryIncident(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Confirm     bool   `json:"confirm"`
		Yes         bool   `json:"yes"`
		DryRun      bool   `json:"dryRun"`
		Dir         string `json:"dir"`
		Environment string `json:"environment"`
	}
	_ = decodeJSON(r, &body)
	if !body.Confirm && !body.Yes && !body.DryRun {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("refusing retry without confirm/yes"))
		return
	}
	key := strings.TrimSpace(r.PathValue("key"))
	if key == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("incident key is required"))
		return
	}
	root, err := optionalAuthorizedDir(body.Dir)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	request := incidents.ResolveRequest{
		Environment: strings.TrimSpace(body.Environment),
		ProjectRoot: root,
		Key:         key,
		DryRun:      body.DryRun,
	}
	var result incidents.Result
	if h != nil && h.resolveIncidentFn != nil {
		result, err = h.resolveIncidentFn(r.Context(), request)
	} else {
		cfg, loadErr := config.Load()
		if loadErr != nil {
			writeErr(w, http.StatusBadRequest, loadErr)
			return
		}
		factory := h.clusterFactory
		if factory == nil {
			factory = cluster.NewFactory(paths.Home(), cfg)
		}
		result, err = incidents.NewService(factory).ResolveWithOptions(r.Context(), request)
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	cli := "camunda incidents retry " + key + " --yes"
	if body.DryRun {
		cli = "camunda incidents retry " + key + " --dry-run"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "result": result, "cli": cli,
	})
}

func (h *handler) traceInstance(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("instanceKey")
	root, err := optionalAuthorizedDir(r.URL.Query().Get("dir"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	request := trace.Request{
		ProcessInstanceKey: key,
		ProjectRoot:        root,
		Environment:        strings.TrimSpace(r.URL.Query().Get("environment")),
	}

	follow := r.URL.Query().Get("follow") == "1" || strings.EqualFold(r.URL.Query().Get("follow"), "true")
	if !follow {
		var tl trace.Timeline
		if h != nil && h.traceGetFn != nil {
			tl, err = h.traceGetFn(r.Context(), request)
		} else {
			cfg, loadErr := config.Load()
			if loadErr != nil {
				writeErr(w, http.StatusBadRequest, loadErr)
				return
			}
			factory := h.clusterFactory
			if factory == nil {
				factory = cluster.NewFactory(paths.Home(), cfg)
			}
			tl, err = trace.NewService(factory).Get(r.Context(), request)
		}
		if err != nil {
			var notFound *trace.NotFoundError
			if errors.As(err, &notFound) {
				writeErr(w, http.StatusNotFound, err)
				return
			}
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":       true,
			"timeline": tl,
			"output":   trace.FormatText(tl),
			"cli":      "camunda trace " + key,
		})
		return
	}

	interval := 2 * time.Second
	if raw := r.URL.Query().Get("interval"); raw != "" {
		parsed, parseErr := time.ParseDuration(raw)
		if parseErr != nil {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid interval: %w", parseErr))
			return
		}
		interval = parsed
	}
	// API/UI follow uses safer bounded defaults than interactive CLI (5m / unbounded domain max).
	timeout := 30 * time.Second
	if raw := r.URL.Query().Get("timeout"); raw != "" {
		parsed, parseErr := time.ParseDuration(raw)
		if parseErr != nil {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid timeout: %w", parseErr))
			return
		}
		timeout = parsed
	}
	if timeout > 2*time.Minute {
		timeout = 2 * time.Minute
	}
	maxEvents := 20
	if raw := r.URL.Query().Get("maxEvents"); raw != "" {
		n, parseErr := strconv.Atoi(raw)
		if parseErr != nil || n < 1 {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid maxEvents"))
			return
		}
		maxEvents = n
	}
	if maxEvents > 50 {
		maxEvents = 50
	}
	request.Timeout = timeout
	request.MaxEvents = maxEvents
	// IdleStop remains CLI-only; API follow does not accept idleStop.

	var timelines []trace.Timeline
	var output strings.Builder
	emit := func(tl trace.Timeline) error {
		timelines = append(timelines, tl)
		if output.Len() > 0 {
			output.WriteByte('\n')
		}
		output.WriteString(trace.FormatText(tl))
		return nil
	}
	if h != nil && h.traceFollowFn != nil {
		err = h.traceFollowFn(r.Context(), request, interval, emit)
	} else {
		cfg, loadErr := config.Load()
		if loadErr != nil {
			writeErr(w, http.StatusBadRequest, loadErr)
			return
		}
		factory := h.clusterFactory
		if factory == nil {
			factory = cluster.NewFactory(paths.Home(), cfg)
		}
		err = trace.NewService(factory).Follow(r.Context(), request, interval, emit)
	}
	if err != nil {
		var notFound *trace.NotFoundError
		if errors.As(err, &notFound) {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	resp := map[string]any{
		"ok":        true,
		"follow":    true,
		"timelines": timelines,
		"output":    output.String(),
		"cli":       "camunda trace " + key + " --follow",
	}
	if len(timelines) > 0 {
		resp["timeline"] = timelines[len(timelines)-1]
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *handler) runPlan(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Dir         string `json:"dir"`
		Environment string `json:"environment,omitempty"`
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
	request := plan.Request{ProjectRoot: root, Environment: strings.TrimSpace(body.Environment)}
	var result plan.Result
	if h != nil && h.plan != nil {
		result, err = h.plan(r.Context(), request)
	} else {
		cfg, loadErr := config.Load()
		if loadErr != nil {
			writeErr(w, http.StatusBadRequest, loadErr)
			return
		}
		factory := h.clusterFactory
		if factory == nil {
			factory = cluster.NewFactory(paths.Home(), cfg)
		}
		result, err = plan.NewService(factory).Run(r.Context(), request)
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("cluster inventory: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     result.Policy.ExitCode == 0,
		"plan":   result,
		"output": plan.FormatText(result),
		"cli":    "camunda plan --dir " + root,
	})
}

func (h *handler) runDrift(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Dir         string `json:"dir"`
		GitRef      string `json:"gitRef,omitempty"`
		Ref         string `json:"ref,omitempty"`
		Environment string `json:"environment,omitempty"`
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
	gitRef := body.GitRef
	if gitRef == "" {
		gitRef = body.Ref
	}
	if gitRef == "" {
		gitRef = "HEAD"
	}
	request := drift.Request{
		ProjectRoot: root, GitRef: gitRef, Environment: strings.TrimSpace(body.Environment),
	}
	var res drift.Report
	var runErr error
	if h != nil && h.drift != nil {
		res, runErr = h.drift(r.Context(), request)
	} else {
		cfg, loadErr := config.Load()
		if loadErr != nil {
			writeErr(w, http.StatusBadRequest, loadErr)
			return
		}
		factory := h.clusterFactory
		if factory == nil {
			factory = cluster.NewFactory(paths.Home(), cfg)
		}
		res, runErr = drift.NewService(factory).Run(r.Context(), request)
	}
	ok := res.Policy.ExitCode == 0
	payload := map[string]any{
		"ok":     ok,
		"status": res.Status,
		"policy": res.Policy,
		"drift":  res,
		"output": drift.FormatText(res),
		"cli":    "camunda drift --dir " + root + " --ref " + gitRef,
	}
	if runErr != nil || res.Policy.ExitCode == 2 {
		payload["error"] = map[string]string{
			"code":    "drift_" + string(res.Status),
			"message": "drift comparison could not complete safely",
		}
	}
	writeJSON(w, http.StatusOK, payload)
}

func (h *handler) legacyRunDoctorDeep(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	base := doctor.Run(false)
	deepReport, err := doctor.RunDeep(r.Context(), cfg, doctor.DeepOptions{})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	report := doctor.FormatDeep(base, deepReport)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       doctor.DeepOK(deepReport) && base.OK,
		"report":   report,
		"sections": deepReport.Checks,
		"cli":      "camunda doctor --deep",
	})
}

// --- Project / env / backup ---

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
	root, err := optionalAuthorizedDir(r.URL.Query().Get("dir"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	service := h.environments()
	active, err := service.Resolve(env.ResolveRequest{ProjectRoot: root})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	profiles, err := service.List(root)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	items := make([]map[string]any, 0, len(profiles))
	for _, resolved := range profiles {
		items = append(items, map[string]any{
			"name":      resolved.Profile.Name,
			"kind":      resolved.Profile.Kind,
			"endpoints": resolved.Profile.Endpoints,
			"auth":      resolved.Profile.Auth,
			"source":    resolved.Source,
		})
	}
	cli := "camunda env list"
	if root != "" {
		cli += " (project " + root + ")"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"active":   active.Profile.Name,
		"source":   active.Source,
		"profiles": items,
		"cli":      cli,
	})
}

func (h *handler) envUse(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
		Dir  string `json:"dir"`
	}
	if err := decodeJSON(r, &body); err != nil || body.Name == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("name is required"))
		return
	}
	root, err := optionalAuthorizedDir(body.Dir)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if _, err := h.environments().Use(body.Name, root); err != nil {
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
		TokenURL        string `json:"tokenUrl"`
		TokenURLEnv     string `json:"tokenUrlEnv"`
		Audience        string `json:"audience"`
		Scope           string `json:"scope"`
		Dir             string `json:"dir"`
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
	root, err := optionalAuthorizedDir(body.Dir)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	p := env.Profile{
		Name: body.Name, Kind: body.Kind,
		Endpoints: map[string]string{},
		Auth: env.AuthRefs{
			ClientIDEnv: body.ClientIDEnv, ClientSecretEnv: body.ClientSecretEnv,
			TokenURL: body.TokenURL, TokenURLEnv: body.TokenURLEnv,
			Audience: body.Audience, Scope: body.Scope,
		},
	}
	if body.Orchestration != "" {
		p.Endpoints["orchestration"] = body.Orchestration
	}
	service := h.environments()
	if root != "" {
		if err := service.SaveProject(root, p); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
	} else if err := service.SaveGlobal(p); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "profile": p, "cli": "camunda env add " + body.Name})
}

func (h *handler) envRemove(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	root, err := optionalAuthorizedDir(r.URL.Query().Get("dir"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	service := h.environments()
	source := env.ProfileSourceGlobal
	if root != "" {
		resolved, err := service.Resolve(env.ResolveRequest{Name: name, ProjectRoot: root})
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		source = resolved.Source
	}
	if err := service.Remove(name, root, source); err != nil {
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
	if strings.TrimSpace(body.Output) == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("output path required; use POST /api/v1/backup/download for browser download"))
		return
	}
	out, err := allowPath(body.Output)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	cfg, _ := config.Load()
	proj := body.Dir
	if proj != "" {
		proj, err = allowPath(proj)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
	}
	m, err := h.createBackup(r.Context(), backup.Options{
		LabHome: paths.Home(), ProjectDir: proj, OutPath: out,
		IncludeSecrets: body.IncludeSecrets, LabVersion: cfg.Version, LabProfile: cfg.Profile,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":              true,
		"files":           len(m.Files),
		"includesSecrets": m.IncludesSecrets,
		"cli":             "camunda backup -o <authorized-path>",
	})
}

func (h *handler) runBackupDownload(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IncludeSecrets bool   `json:"includeSecrets"`
		Dir            string `json:"dir"`
	}
	_ = decodeJSON(r, &body)
	proj := body.Dir
	if proj != "" {
		var err error
		proj, err = allowPath(proj)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
	}
	tmp, err := os.CreateTemp("", "camunda-lab-backup-dl-*.tar.gz")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Errorf("could not stage backup download"))
		return
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)

	cfg, _ := config.Load()
	m, err := h.createBackup(r.Context(), backup.Options{
		LabHome: paths.Home(), ProjectDir: proj, OutPath: tmpPath,
		IncludeSecrets: body.IncludeSecrets, LabVersion: cfg.Version, LabProfile: cfg.Profile,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("could not create backup archive"))
		return
	}
	f, err := os.Open(tmpPath)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Errorf("could not read backup archive"))
		return
	}
	defer f.Close()

	name := fmt.Sprintf("camunda-lab-backup-%s.tar.gz", time.Now().UTC().Format("20060102-150405"))
	writeAttachment(w, "application/gzip", name)
	w.Header().Set("X-Camunda-Lab-Backup-Files", strconv.Itoa(len(m.Files)))
	w.Header().Set("X-Camunda-Lab-Backup-Secrets", strconv.FormatBool(m.IncludesSecrets))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, f)
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
	force := r.FormValue("force") == "true"
	fh, _, err := r.FormFile("archive")
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("archive file required"))
		return
	}
	defer fh.Close()
	tmp, err := os.CreateTemp("", "camunda-lab-restore-*.tar.gz")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Errorf("could not stage restore upload"))
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
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
	m, err := h.backupService().Restore(r.Context(), backup.RestoreOptions{
		ArchivePath: tmpPath,
		LabHome:     paths.Home(),
		ProjectDir:  proj,
		Force:       force,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"files": len(m.Files),
		"cli":   "camunda restore <archive.tar.gz> --yes",
	})
}
