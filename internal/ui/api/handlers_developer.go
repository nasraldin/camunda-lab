package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nasraldin/camunda-lab/internal/ai"
	"github.com/nasraldin/camunda-lab/internal/config"
	bpmndiff "github.com/nasraldin/camunda-lab/internal/diff"
	"github.com/nasraldin/camunda-lab/internal/doctor"
	"github.com/nasraldin/camunda-lab/internal/lint"
	"github.com/nasraldin/camunda-lab/internal/review"
	"github.com/nasraldin/camunda-lab/internal/scan"
	"github.com/nasraldin/camunda-lab/internal/toolkit"
)

const (
	maxDeveloperMultipartBytes    = 20 << 20
	developerMultipartMemoryBytes = 64 << 10
	maxDeveloperUploadFiles       = 8
)

type developerDoctorDependencies struct {
	loadConfig func() (config.Config, error)
	runShallow func(bool) doctor.Report
	runDeep    func(context.Context, config.Config, doctor.DeepOptions) (doctor.DeepReport, error)
}

func (h *handler) developerDoctorDependencies() developerDoctorDependencies {
	if h.doctor != nil {
		return *h.doctor
	}
	return developerDoctorDependencies{
		loadConfig: config.Load,
		runShallow: doctor.Run,
		runDeep:    doctor.RunDeep,
	}
}

func decodeStrictDeveloperJSON(r *http.Request, value any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(r.Body, maxUploadBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return errors.New("request body must contain one JSON object")
		}
		return err
	}
	return nil
}

func normalizedProjectDir(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	allowed, err := allowPath(value)
	if err != nil {
		return "", err
	}
	return canonicalizeForAuthorization(allowed)
}

func authorizedInputs(paths []string, projectDir string) ([]toolkit.BPMNInput, string, error) {
	root, err := normalizedProjectDir(projectDir)
	if err != nil {
		return nil, "", err
	}
	inputs := make([]toolkit.BPMNInput, 0, len(paths))
	for _, path := range paths {
		var resolved string
		if root == "" {
			resolved, err = allowPath(path)
			if err != nil {
				return nil, "", err
			}
		} else {
			candidate := path
			if !filepath.IsAbs(candidate) {
				candidate = filepath.Join(root, candidate)
			}
			resolved, err = canonicalizeForAuthorization(candidate)
			if err != nil {
				return nil, "", err
			}
			relative, relErr := filepath.Rel(root, resolved)
			if relErr != nil || filepath.IsAbs(relative) || relative == ".." ||
				strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				return nil, "", fmt.Errorf("path %q escapes declared project", path)
			}
		}
		inputs = append(inputs, toolkit.BPMNInput{Name: path, Path: resolved})
	}
	return inputs, root, nil
}

func prepareDeveloperMultipart(w http.ResponseWriter, r *http.Request) (func(), error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxDeveloperMultipartBytes)
	if err := r.ParseMultipartForm(developerMultipartMemoryBytes); err != nil {
		return func() {}, fmt.Errorf("multipart body exceeds limits or is malformed: %w", err)
	}
	cleanup := func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}
	count := 0
	for _, headers := range r.MultipartForm.File {
		count += len(headers)
		for _, header := range headers {
			if header.Size > maxUploadBytes {
				cleanup()
				return func() {}, fmt.Errorf("file too large (max 10MB)")
			}
		}
	}
	if count > maxDeveloperUploadFiles {
		cleanup()
		return func() {}, fmt.Errorf("too many uploaded files (max %d)", maxDeveloperUploadFiles)
	}
	return cleanup, nil
}

func uploadedInputs(r *http.Request, fields ...string) ([]toolkit.BPMNInput, error) {
	if r.MultipartForm == nil {
		return nil, errors.New("multipart form was not prepared")
	}
	var inputs []toolkit.BPMNInput
	for _, field := range fields {
		for _, header := range r.MultipartForm.File[field] {
			content, err := readUpload(header)
			if err != nil {
				return nil, err
			}
			inputs = append(inputs, toolkit.BPMNInput{Name: filepath.Base(header.Filename), Content: content})
		}
	}
	return inputs, nil
}

func readUpload(header *multipart.FileHeader) ([]byte, error) {
	if header.Size > maxUploadBytes {
		return nil, fmt.Errorf("file too large (max 10MB)")
	}
	file, err := header.Open()
	if err != nil {
		return nil, err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxUploadBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > maxUploadBytes {
		return nil, fmt.Errorf("file too large (max 10MB)")
	}
	return content, nil
}

func (h *handler) bpmnLint(w http.ResponseWriter, r *http.Request) {
	request := developerInputRequest{FailOn: "error"}
	var inputs []toolkit.BPMNInput
	var projectDir string
	var err error
	if strings.Contains(r.Header.Get("Content-Type"), "multipart/form-data") {
		var cleanup func()
		cleanup, err = prepareDeveloperMultipart(w, r)
		if err == nil {
			defer cleanup()
			inputs, err = uploadedInputs(r, "file", "files")
			request.FailOn = nonEmptyForm(r.FormValue("failOn"), "error")
			request.Ignore = r.MultipartForm.Value["ignore"]
		}
	} else {
		err = decodeStrictDeveloperJSON(r, &request)
		if err == nil {
			paths := append([]string(nil), request.Paths...)
			if request.Path != "" {
				paths = append(paths, request.Path)
			}
			inputs, projectDir, err = authorizedInputs(paths, request.ProjectDir)
		}
	}
	if err != nil {
		writeDeveloperError(w, err)
		return
	}
	result, err := h.bpmnToolkit().Lint(r.Context(), toolkit.LintRequest{
		Inputs: inputs, ProjectDir: projectDir, FailOn: toolkit.LintThreshold(request.FailOn), Ignore: request.Ignore,
	})
	if err != nil {
		writeDeveloperError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, lintResponse{
		OK: result.Status != toolkit.StatusFailed, Status: result.Status, Complete: result.Complete,
		Warnings: nonNilWarnings(result.Warnings), Findings: nonNilLintFindings(result.Findings),
		Inputs: nonNilStrings(result.Inputs), Output: formatLintOutput(result.Findings),
		Contents: developerBPMNContents(inputs, result.Inputs), CLI: "camunda lint <file.bpmn>",
	})
}

func (h *handler) bpmnDiff(w http.ResponseWriter, r *http.Request) {
	request := developerDiffRequest{}
	var service toolkit.Service
	var domainRequest toolkit.DiffRequest
	var err error
	if strings.Contains(r.Header.Get("Content-Type"), "multipart/form-data") {
		cleanup, prepareErr := prepareDeveloperMultipart(w, r)
		err = prepareErr
		if err == nil {
			defer cleanup()
			from, fromErr := uploadedInputs(r, "from", "fileA")
			to, toErr := uploadedInputs(r, "to", "fileB")
			if fromErr != nil {
				err = fromErr
			} else if toErr != nil {
				err = toErr
			} else if len(from) != 1 || len(to) != 1 {
				err = errors.New("exactly two BPMN uploads are required")
			} else {
				domainRequest.Before, domainRequest.After = from[0], to[0]
			}
		}
	} else {
		err = decodeStrictDeveloperJSON(r, &request)
		if err == nil {
			domainRequest, service, err = buildDiffRequest(request)
		}
	}
	if err != nil {
		writeDeveloperError(w, err)
		return
	}
	var result toolkit.DiffResult
	if h != nil && h.toolkitService != nil {
		result, err = h.toolkitService.Diff(r.Context(), domainRequest)
	} else {
		result, err = service.Diff(r.Context(), domainRequest)
	}
	if err != nil {
		writeDeveloperError(w, err)
		return
	}
	contents := map[string]string{}
	if len(domainRequest.Before.Content) > 0 {
		contents["before"] = string(domainRequest.Before.Content)
	} else if domainRequest.Before.Path != "" {
		if data, readErr := os.ReadFile(domainRequest.Before.Path); readErr == nil {
			contents["before"] = string(data)
		}
	}
	if len(domainRequest.After.Content) > 0 {
		contents["after"] = string(domainRequest.After.Content)
	} else if domainRequest.After.Path != "" {
		if data, readErr := os.ReadFile(domainRequest.After.Path); readErr == nil {
			contents["after"] = string(data)
		}
	}
	writeJSON(w, http.StatusOK, diffResponse{
		OK: len(result.Changes) == 0, Status: result.Status, Complete: result.Complete,
		Warnings: nonNilWarnings(result.Warnings), Changes: nonNilChanges(result.Changes),
		Output: formatDiffOutput(result.Changes), Contents: contents,
		CLI: "camunda diff before.bpmn after.bpmn",
	})
}

func buildDiffRequest(request developerDiffRequest) (toolkit.DiffRequest, toolkit.Service, error) {
	var domain toolkit.DiffRequest
	var service toolkit.Service
	root, err := normalizedProjectDir(request.ProjectDir)
	if err != nil {
		return domain, service, err
	}
	pathsMode := len(request.Paths) > 0
	fromToMode := request.From != "" || request.To != ""
	againstMode := request.Against != ""
	gitMode := request.Base != ""
	modeCount := 0
	for _, selected := range []bool{pathsMode, fromToMode, againstMode, gitMode} {
		if selected {
			modeCount++
		}
	}
	if modeCount != 1 {
		return domain, service, errors.New("exactly one diff mode is required")
	}
	switch {
	case gitMode:
		if request.Path == "" || request.Against != "" || request.From != "" || request.To != "" ||
			len(request.Paths) != 0 {
			return domain, service, errors.New("Git diff requires only projectDir, path, and base")
		}
		if root == "" {
			return domain, service, errors.New("projectDir is required for Git diff")
		}
		after, err := authorizedProjectRelativePath(root, request.Path)
		if err != nil {
			return domain, service, err
		}
		domain = toolkit.DiffRequest{
			BeforeGit: &toolkit.GitInput{Ref: request.Base, Path: filepath.ToSlash(request.Path)},
			After:     toolkit.BPMNInput{Name: request.Path, Path: after}, ProjectDir: root,
		}
		service.Git = bpmndiff.NewGitReader(root)
	case againstMode:
		if request.Path == "" || request.Base != "" || request.From != "" || request.To != "" ||
			len(request.Paths) != 0 {
			return domain, service, errors.New("against diff requires only path and against")
		}
		inputs, _, err := authorizedInputs([]string{request.Path, request.Against}, root)
		if err != nil {
			return domain, service, err
		}
		domain.Before, domain.After = inputs[0], inputs[1]
	case pathsMode:
		if len(request.Paths) != 2 || request.Path != "" || request.From != "" || request.To != "" ||
			request.Against != "" || request.Base != "" {
			return domain, service, errors.New("paths diff requires exactly two paths")
		}
		inputs, _, err := authorizedInputs(request.Paths, root)
		if err != nil {
			return domain, service, err
		}
		domain.Before, domain.After = inputs[0], inputs[1]
	case fromToMode:
		if request.From == "" || request.To == "" || request.Path != "" || request.Against != "" ||
			request.Base != "" || len(request.Paths) != 0 {
			return domain, service, errors.New("from/to diff requires both from and to")
		}
		inputs, _, err := authorizedInputs([]string{request.From, request.To}, root)
		if err != nil {
			return domain, service, err
		}
		domain.Before, domain.After = inputs[0], inputs[1]
	default:
		return domain, service, errors.New("choose paths[2], from/to, path/against, or projectDir/path/base")
	}
	return domain, service, nil
}

func authorizedProjectRelativePath(root, relative string) (string, error) {
	if filepath.IsAbs(relative) || strings.ContainsAny(relative, "\x00\r\n") {
		return "", errors.New("Git diff path must be project-relative")
	}
	clean := filepath.Clean(relative)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("Git diff path escapes the project")
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	resolvedPath, err := allowPath(filepath.Join(root, clean))
	if err != nil {
		return "", err
	}
	inside, err := filepath.Rel(resolvedRoot, resolvedPath)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		return "", errors.New("Git diff path escapes the project")
	}
	return resolvedPath, nil
}

func (h *handler) bpmnExplain(w http.ResponseWriter, r *http.Request) {
	input, projectDir, format, err := decodeOneDeveloperInput(w, r)
	if err != nil {
		writeDeveloperError(w, err)
		return
	}
	result, err := h.bpmnToolkit().Explain(r.Context(), toolkit.ExplainRequest{Input: input, ProjectDir: projectDir})
	if err != nil {
		writeDeveloperError(w, err)
		return
	}
	processes := make([]explainProcessDTO, 0, len(result.Processes))
	var output strings.Builder
	for index, process := range result.Processes {
		markdown := process.Explanation.Markdown()
		processes = append(processes, explainProcessDTO{ProcessID: process.ProcessID, Markdown: markdown})
		if index > 0 {
			output.WriteString("\n")
		}
		output.WriteString(markdown)
	}
	if format != "" && format != "text" && format != "json" {
		writeDeveloperError(w, errors.New("format must be text or json"))
		return
	}
	writeJSON(w, http.StatusOK, explainResponse{
		OK: true, Status: result.Status, Complete: result.Complete, Warnings: nonNilWarnings(result.Warnings),
		Processes: processes, Output: output.String(),
		Contents: developerBPMNContents([]toolkit.BPMNInput{input}, []string{bpmnInputLabel(input)}),
		CLI:      "camunda explain <file.bpmn>",
	})
}

func (h *handler) bpmnReview(w http.ResponseWriter, r *http.Request) {
	request := developerReviewRequest{FailOn: "error"}
	var inputs []toolkit.BPMNInput
	var projectDir string
	var err error
	if strings.Contains(r.Header.Get("Content-Type"), "multipart/form-data") {
		var cleanup func()
		cleanup, err = prepareDeveloperMultipart(w, r)
		if err == nil {
			defer cleanup()
			inputs, err = uploadedInputs(r, "file", "files")
			request.FailOn = nonEmptyForm(r.FormValue("failOn"), "error")
			request.Ignore = r.MultipartForm.Value["ignore"]
			request.AI, err = strictOptionalMultipartBool(r.MultipartForm, "ai")
			if err == nil {
				request.AIRequired, err = strictOptionalMultipartBool(
					r.MultipartForm,
					"aiRequired",
					"required",
				)
			}
			if err == nil {
				if values, exists := r.MultipartForm.Value["provider"]; exists && len(values) > 0 {
					request.Provider = &values[0]
				}
			}
			if err == nil {
				if values, exists := r.MultipartForm.Value["model"]; exists && len(values) > 0 {
					request.Model = &values[0]
				}
			}
		}
	} else {
		err = decodeStrictDeveloperJSON(r, &request)
		if err == nil {
			paths := append([]string(nil), request.Paths...)
			if request.Path != "" {
				paths = append(paths, request.Path)
			}
			inputs, projectDir, err = authorizedInputs(paths, request.ProjectDir)
		}
	}
	if err != nil {
		writeDeveloperError(w, err)
		return
	}
	aiRequested := request.AI || request.AIRequired
	if !aiRequested && (request.Provider != nil || request.Model != nil) {
		writeDeveloperError(w, errors.New("provider/model require ai=true or aiRequired=true"))
		return
	}
	provider := "openai"
	model := "gpt-4o-mini"
	if request.Provider != nil {
		provider = *request.Provider
	}
	if request.Model != nil {
		model = *request.Model
	}
	reviewRequest := toolkit.ReviewRequest{
		Inputs: inputs, ProjectDir: projectDir, FailOn: toolkit.LintThreshold(request.FailOn),
		Ignore: request.Ignore, AI: toolkit.AIOptions{Enabled: request.AI, Required: request.AIRequired},
	}
	var result toolkit.ReviewResult
	if h != nil && h.toolkitService != nil {
		result, err = h.toolkitService.Review(r.Context(), reviewRequest)
	} else {
		service := toolkit.Service{}
		if aiRequested {
			secrets, loadErr := ai.LoadSecrets()
			if loadErr != nil {
				writeDeveloperError(w, loadErr)
				return
			}
			if err = review.ValidateClientConfiguration(provider, model, secrets); err != nil {
				writeDeveloperError(w, err)
				return
			}
			service.AI, err = review.NewConfiguredClient(provider, model, secrets)
			if err != nil && (!review.IsMissingCredentials(err) || request.AIRequired) {
				writeDeveloperError(w, err)
				return
			}
			if review.IsMissingCredentials(err) {
				service.AI = nil
			}
		}
		result, err = service.Review(r.Context(), reviewRequest)
	}
	if err != nil {
		writeDeveloperError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, reviewResponse{
		OK: result.Status != toolkit.StatusFailed, Status: result.Status, Complete: result.Complete,
		Warnings: nonNilWarnings(result.Warnings), Findings: nonNilLintFindings(result.Findings),
		AIStatus: result.AIStatus, Reviews: nonNilReviews(result.Processes),
		Output:   formatLintOutput(result.Findings),
		Contents: developerBPMNContents(inputs, result.Inputs), CLI: "camunda review <file.bpmn>",
	})
}

func (h *handler) bpmnTestGenerate(w http.ResponseWriter, r *http.Request) {
	request := developerGenerateRequest{Lang: "java"}
	var input toolkit.BPMNInput
	var projectDir string
	var err error
	if strings.Contains(r.Header.Get("Content-Type"), "multipart/form-data") {
		cleanup, prepareErr := prepareDeveloperMultipart(w, r)
		err = prepareErr
		if err == nil {
			defer cleanup()
			inputs, uploadErr := uploadedInputs(r, "file")
			err = uploadErr
			if err == nil && len(inputs) != 1 {
				err = errors.New("exactly one BPMN upload is required")
			}
			if err == nil {
				input = inputs[0]
				request.Lang = nonEmptyForm(r.FormValue("lang"), "java")
				request.Write, err = strictOptionalMultipartBool(r.MultipartForm, "write")
				request.Output = r.FormValue("output")
				if err == nil {
					request.Force, err = strictOptionalMultipartBool(r.MultipartForm, "force")
				}
			}
		}
	} else {
		err = decodeStrictDeveloperJSON(r, &request)
		if err == nil {
			var inputs []toolkit.BPMNInput
			inputs, projectDir, err = authorizedInputs([]string{request.Path}, request.ProjectDir)
			if err == nil {
				input = inputs[0]
			}
		}
	}
	if err == nil && request.Output != "" && !request.Write {
		err = errors.New("output requires write=true")
	}
	output := ""
	if err == nil && request.Write {
		if request.Output == "" {
			err = errors.New("write=true requires output")
		} else {
			output, err = allowPath(request.Output)
		}
	}
	if err != nil {
		writeDeveloperError(w, err)
		return
	}
	result, err := h.bpmnToolkit().Generate(r.Context(), toolkit.GenerateRequest{
		Input: input, ProjectDir: projectDir, OutDir: output,
		Lang: toolkit.GenerateLanguage(request.Lang), Force: request.Force,
	})
	if err != nil {
		writeDeveloperError(w, err)
		return
	}
	mode := "download"
	artifacts := make([]artifactDTO, 0, len(result.Artifacts))
	paths := make([]string, 0, len(result.Artifacts))
	contents := map[string]string{}
	if request.Write {
		mode = "written"
	}
	for _, artifact := range result.Artifacts {
		item := artifactDTO{Path: artifact.Path, MediaType: artifact.MediaType}
		paths = append(paths, artifact.Path)
		if !request.Write {
			item.Content = string(artifact.Content)
			contents[artifact.Path] = string(artifact.Content)
		}
		artifacts = append(artifacts, item)
	}
	writeJSON(w, http.StatusOK, generateResponse{
		OK: true, Status: result.Status, Complete: result.Complete, Warnings: nonNilWarnings(result.Warnings),
		Mode: mode, Artifacts: artifacts, Paths: paths, Contents: contents,
		CLI: "camunda test generate <file.bpmn>",
	})
}

func (h *handler) bpmnTestGenerateDownload(w http.ResponseWriter, r *http.Request) {
	request := developerGenerateRequest{Lang: "java"}
	var input toolkit.BPMNInput
	var projectDir string
	var err error
	if strings.Contains(r.Header.Get("Content-Type"), "multipart/form-data") {
		cleanup, prepareErr := prepareDeveloperMultipart(w, r)
		err = prepareErr
		if err == nil {
			defer cleanup()
			inputs, uploadErr := uploadedInputs(r, "file")
			err = uploadErr
			if err == nil && len(inputs) != 1 {
				err = errors.New("exactly one BPMN upload is required")
			}
			if err == nil {
				input = inputs[0]
				request.Lang = nonEmptyForm(r.FormValue("lang"), "java")
			}
		}
	} else {
		err = decodeStrictDeveloperJSON(r, &request)
		if err == nil {
			var inputs []toolkit.BPMNInput
			inputs, projectDir, err = authorizedInputs([]string{request.Path}, request.ProjectDir)
			if err == nil {
				input = inputs[0]
			}
		}
	}
	if err != nil {
		writeDeveloperError(w, err)
		return
	}
	// Download route never writes artifacts; ignore write/output/force.
	result, err := h.bpmnToolkit().Generate(r.Context(), toolkit.GenerateRequest{
		Input: input, ProjectDir: projectDir, Lang: toolkit.GenerateLanguage(request.Lang),
	})
	if err != nil {
		writeDeveloperError(w, err)
		return
	}
	payload, err := toolkit.PackArtifactsZIP(result.Artifacts)
	if err != nil {
		writeDeveloperError(w, err)
		return
	}
	writeAttachment(w, "application/zip", "camunda-lab-tests.zip")
	w.Header().Set("X-Camunda-Lab-Artifact-Count", strconv.Itoa(len(result.Artifacts)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (h *handler) bpmnScan(w http.ResponseWriter, r *http.Request) {
	request := developerScanRequest{FailOn: "medium"}
	if err := decodeStrictDeveloperJSON(r, &request); err != nil {
		writeDeveloperError(w, err)
		return
	}
	roots := append([]string(nil), request.Roots...)
	if request.Dir != "" {
		roots = append(roots, request.Dir)
	}
	for index, root := range roots {
		authorized, err := allowPath(root)
		if err != nil {
			writeDeveloperError(w, err)
			return
		}
		roots[index] = authorized
	}
	projectDir, err := normalizedProjectDir(request.ProjectDir)
	if err != nil {
		writeDeveloperError(w, err)
		return
	}
	result, err := h.bpmnToolkit().Scan(r.Context(), toolkit.ScanRequest{
		Roots: roots, ProjectDir: projectDir, FailOn: toolkit.ScanThreshold(request.FailOn), Ignore: request.Ignore,
	})
	if err != nil {
		writeDeveloperError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, scanResponse{
		OK: result.Status != toolkit.StatusFailed && result.Complete, Status: result.Status, Complete: result.Complete,
		Warnings: nonNilWarnings(result.Warnings), ScannedRoots: nonNilStrings(result.ScannedRoots),
		FailedRoots: nonNilStrings(result.FailedRoots), Findings: nonNilScanFindings(result.Findings),
		Issues: nonNilScanIssues(result.Issues), Stats: result.Stats, CLI: "camunda scan <dir>",
	})
}

func (h *handler) runDoctorDeep(w http.ResponseWriter, r *http.Request) {
	deps := h.developerDoctorDependencies()
	cfg, err := deps.loadConfig()
	if err != nil {
		writeDeveloperError(w, err)
		return
	}
	base := deps.runShallow(false)
	result, err := deps.runDeep(r.Context(), cfg, doctor.DeepOptions{})
	if err != nil {
		writeDeveloperError(w, err)
		return
	}
	checks := append([]doctor.Check(nil), result.Checks...)
	if !base.OK {
		checks = append(checks, doctor.Check{
			ID: "shallow.prerequisites", Category: "configuration", Status: doctor.StatusFail,
			Summary: "Basic prerequisites have issues", Detail: "One or more basic prerequisite checks failed",
			Remediation: base.FixHint, Required: true,
		})
	}
	combined := doctor.DeepReport{Checks: checks}
	combined.Aggregate()
	status := "completed"
	if !combined.OK {
		status = "failed"
	}
	writeJSON(w, http.StatusOK, doctorDeepResponse{
		OK: combined.OK, Status: status, Checks: combined.Checks,
		Report: combined.Text(), CLI: "camunda doctor --deep",
	})
}

func decodeOneDeveloperInput(
	w http.ResponseWriter,
	r *http.Request,
) (toolkit.BPMNInput, string, string, error) {
	if strings.Contains(r.Header.Get("Content-Type"), "multipart/form-data") {
		cleanup, err := prepareDeveloperMultipart(w, r)
		if err != nil {
			return toolkit.BPMNInput{}, "", "", err
		}
		defer cleanup()
		inputs, err := uploadedInputs(r, "file")
		if err != nil {
			return toolkit.BPMNInput{}, "", "", err
		}
		if len(inputs) != 1 {
			return toolkit.BPMNInput{}, "", "", errors.New("exactly one BPMN upload is required")
		}
		return inputs[0], "", r.FormValue("format"), nil
	}
	request := developerInputRequest{}
	if err := decodeStrictDeveloperJSON(r, &request); err != nil {
		return toolkit.BPMNInput{}, "", "", err
	}
	paths := append([]string(nil), request.Paths...)
	if request.Path != "" {
		paths = append(paths, request.Path)
	}
	if len(paths) != 1 {
		return toolkit.BPMNInput{}, "", "", errors.New("exactly one BPMN path is required")
	}
	inputs, projectDir, err := authorizedInputs(paths, request.ProjectDir)
	if err != nil {
		return toolkit.BPMNInput{}, "", "", err
	}
	return inputs[0], projectDir, request.Format, nil
}

func writeDeveloperError(w http.ResponseWriter, err error) {
	writeMappedErr(w, err)
}

func nonEmptyForm(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func strictOptionalMultipartBool(form *multipart.Form, names ...string) (bool, error) {
	var values []string
	for _, name := range names {
		values = append(values, form.Value[name]...)
	}
	if len(values) == 0 {
		return false, nil
	}
	if len(values) != 1 {
		return false, fmt.Errorf("%s must appear at most once", strings.Join(names, "/"))
	}
	switch values[0] {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be exactly true or false", strings.Join(names, "/"))
	}
}

func nonNilWarnings(value []toolkit.Warning) []toolkit.Warning {
	if value == nil {
		return []toolkit.Warning{}
	}
	return value
}

func nonNilStrings(value []string) []string {
	if value == nil {
		return []string{}
	}
	return value
}

func nonNilLintFindings(value []toolkit.LintFinding) []toolkit.LintFinding {
	if value == nil {
		return []toolkit.LintFinding{}
	}
	return value
}

func nonNilChanges(value []toolkit.ProcessChange) []toolkit.ProcessChange {
	if value == nil {
		return []toolkit.ProcessChange{}
	}
	return value
}

func nonNilReviews(value []toolkit.ProcessReview) []toolkit.ProcessReview {
	if value == nil {
		return []toolkit.ProcessReview{}
	}
	return value
}

func nonNilScanFindings(value []scan.Finding) []scan.Finding {
	if value == nil {
		return []scan.Finding{}
	}
	return value
}

func nonNilScanIssues(value []scan.Issue) []scan.Issue {
	if value == nil {
		return []scan.Issue{}
	}
	return value
}

func formatLintOutput(findings []toolkit.LintFinding) string {
	flat := make([]lint.Finding, 0, len(findings))
	for _, item := range findings {
		value := item.Finding
		if item.ProcessID != "" {
			value.ProcessID = item.ProcessID
		}
		flat = append(flat, value)
	}
	return lint.FormatText(lint.Result{Findings: flat})
}

func formatDiffOutput(changes []toolkit.ProcessChange) string {
	flat := make([]bpmndiff.Change, 0, len(changes))
	for _, change := range changes {
		if change.Change != nil {
			flat = append(flat, *change.Change)
			continue
		}
		switch change.Kind {
		case toolkit.ProcessAdded:
			flat = append(flat, bpmndiff.Change{
				Kind: bpmndiff.ProcessAdded, ProcessID: change.AfterProcessID,
				Summary: "process added: " + change.AfterProcessID,
			})
		case toolkit.ProcessRemoved:
			flat = append(flat, bpmndiff.Change{
				Kind: bpmndiff.ProcessRemoved, ProcessID: change.BeforeProcessID,
				Summary: "process removed: " + change.BeforeProcessID,
			})
		case toolkit.DocumentChanged:
			flat = append(flat, bpmndiff.Change{Kind: bpmndiff.ProcessAdded, Summary: "document changed"})
		}
	}
	return bpmndiff.FormatText(flat)
}

func developerBPMNContents(inputs []toolkit.BPMNInput, labels []string) map[string]string {
	contents := make(map[string]string)
	for _, input := range inputs {
		label := bpmnInputLabel(input)
		if len(input.Content) > 0 {
			contents[label] = string(input.Content)
			continue
		}
		if input.Path != "" {
			if data, err := os.ReadFile(input.Path); err == nil {
				contents[label] = string(data)
			}
		}
	}
	for _, label := range labels {
		if _, ok := contents[label]; ok {
			continue
		}
		if data, err := os.ReadFile(label); err == nil {
			contents[label] = string(data)
		}
	}
	if len(contents) == 0 {
		return nil
	}
	return contents
}

func bpmnInputLabel(input toolkit.BPMNInput) string {
	if input.Name != "" {
		return input.Name
	}
	if input.Path != "" {
		return input.Path
	}
	return "<memory>"
}
