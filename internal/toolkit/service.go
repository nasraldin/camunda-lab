package toolkit

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nasraldin/camunda-lab/internal/ai"
	"github.com/nasraldin/camunda-lab/internal/bpmn"
	bpmndiff "github.com/nasraldin/camunda-lab/internal/diff"
	"github.com/nasraldin/camunda-lab/internal/explain"
	"github.com/nasraldin/camunda-lab/internal/lint"
	"github.com/nasraldin/camunda-lab/internal/project"
	"github.com/nasraldin/camunda-lab/internal/review"
	"github.com/nasraldin/camunda-lab/internal/scan"
	"github.com/nasraldin/camunda-lab/internal/testgen"
)

func (s Service) Lint(ctx context.Context, request LintRequest) (LintResult, error) {
	if err := validateContext(ctx, OperationLint); err != nil {
		return LintResult{}, err
	}
	if err := validateLintThreshold(OperationLint, request.FailOn); err != nil {
		return LintResult{}, err
	}
	inputs, opened, err := resolveBPMNInputs(request.Inputs, request.ProjectDir, OperationLint)
	if err != nil {
		return LintResult{}, err
	}
	ignore := append([]string(nil), request.Ignore...)
	if opened != nil {
		ignore = append(ignore, opened.Config.Lint.Ignore...)
	}
	result := LintResult{Status: StatusCompleted, Complete: true}
	for _, input := range inputs {
		if err := ctx.Err(); err != nil {
			return LintResult{}, operationError(OperationLint, ErrorInput, input.label(), err)
		}
		document, err := parseInput(input, OperationLint)
		if err != nil {
			return LintResult{}, err
		}
		result.Inputs = append(result.Inputs, input.label())
		result.Documents = append(result.Documents, document)
		result.Findings = append(result.Findings, runLint(document, input.label(), ignore)...)
	}
	if lint.ShouldFail(rawLintFindings(result.Findings), string(request.FailOn)) {
		result.Status = StatusFailed
	}
	return result, nil
}

func (s Service) Diff(ctx context.Context, request DiffRequest) (DiffResult, error) {
	if err := ctx.Err(); err != nil {
		return DiffResult{}, operationError(OperationDiff, ErrorInput, "", err)
	}
	before := request.Before
	if request.BeforeGit != nil {
		if s.Git == nil {
			return DiffResult{}, operationError(OperationDiff, ErrorGit, request.BeforeGit.Path, errors.New("Git reader is not configured"))
		}
		content, err := s.Git.Read(ctx, request.BeforeGit.Ref, request.BeforeGit.Path)
		if err != nil {
			return DiffResult{}, operationError(OperationDiff, ErrorGit, request.BeforeGit.Path, err)
		}
		if err := validateContext(ctx, OperationDiff); err != nil {
			return DiffResult{}, err
		}
		before = BPMNInput{Name: request.BeforeGit.Path, Content: content}
	}
	if !hasInput(before) || !hasInput(request.After) {
		return DiffResult{}, operationError(OperationDiff, ErrorInvalidRequest, "", errors.New("before and after BPMN inputs are required"))
	}
	resolved, _, err := resolveBPMNInputs([]BPMNInput{before, request.After}, request.ProjectDir, OperationDiff)
	if err != nil {
		return DiffResult{}, err
	}
	if err := validateContext(ctx, OperationDiff); err != nil {
		return DiffResult{}, err
	}
	beforeDocument, err := parseInput(resolved[0], OperationDiff)
	if err != nil {
		return DiffResult{}, err
	}
	if err := validateContext(ctx, OperationDiff); err != nil {
		return DiffResult{}, err
	}
	afterDocument, err := parseInput(resolved[1], OperationDiff)
	if err != nil {
		return DiffResult{}, err
	}
	changes := compareDocuments(beforeDocument, afterDocument)
	status := StatusCompleted
	if len(changes) > 0 {
		status = StatusFailed
	}
	return DiffResult{
		Status: status, Complete: true, Before: beforeDocument, After: afterDocument, Changes: changes,
	}, nil
}

func (s Service) Explain(ctx context.Context, request ExplainRequest) (ExplainResult, error) {
	if err := validateContext(ctx, OperationExplain); err != nil {
		return ExplainResult{}, err
	}
	inputs, _, err := resolveBPMNInputs([]BPMNInput{request.Input}, request.ProjectDir, OperationExplain)
	if err != nil {
		return ExplainResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return ExplainResult{}, operationError(OperationExplain, ErrorInput, inputs[0].label(), err)
	}
	document, err := parseInput(inputs[0], OperationExplain)
	if err != nil {
		return ExplainResult{}, err
	}
	result := ExplainResult{Status: StatusCompleted, Complete: true, Document: document}
	for _, process := range document.Processes {
		result.Processes = append(result.Processes, ProcessExplanation{
			ProcessID: process.ID, Explanation: explain.Offline(processDocument(document, process)),
		})
	}
	return result, nil
}

func (s Service) Review(ctx context.Context, request ReviewRequest) (ReviewResult, error) {
	if err := validateContext(ctx, OperationReview); err != nil {
		return ReviewResult{}, err
	}
	if err := validateLintThreshold(OperationReview, request.FailOn); err != nil {
		return ReviewResult{}, err
	}
	inputs, opened, err := resolveBPMNInputs(request.Inputs, request.ProjectDir, OperationReview)
	if err != nil {
		return ReviewResult{}, err
	}
	ignore := append([]string(nil), request.Ignore...)
	if opened != nil {
		ignore = append(ignore, opened.Config.Lint.Ignore...)
	}
	result := ReviewResult{
		Status: StatusCompleted, Complete: true, AIStatus: AIStatusDisabled,
	}
	aiRequested := request.AI.Enabled || request.AI.Required
	if aiRequested {
		result.AIStatus = AIStatusSucceeded
		if s.AI == nil {
			if request.AI.Required {
				return ReviewResult{}, operationError(OperationReview, ErrorAI, "", errors.New("AI client is not configured"))
			}
			result.AIStatus = AIStatusSkipped
			result.Status, result.Complete = StatusPartial, false
			result.Warnings = append(result.Warnings, Warning{Code: "ai_unavailable", Message: "AI client is not configured"})
		}
	}
	for _, input := range inputs {
		if err := ctx.Err(); err != nil {
			return ReviewResult{}, operationError(OperationReview, ErrorInput, input.label(), err)
		}
		document, err := parseInput(input, OperationReview)
		if err != nil {
			return ReviewResult{}, err
		}
		result.Inputs = append(result.Inputs, input.label())
		result.Documents = append(result.Documents, document)
		result.Findings = append(result.Findings, runDocumentLint(document, input.label(), ignore)...)
		for _, process := range document.Processes {
			reviewDoc := processDocument(document, process)
			lintDoc := reviewDoc
			lintDoc.Messages = nil
			domainResult, err := review.Run(lintDoc, review.Options{
				File: input.label(), FailOn: string(request.FailOn), Ignore: ignore,
			})
			if err != nil {
				return ReviewResult{}, operationError(OperationReview, ErrorInput, input.label(), err)
			}
			if aiRequested && s.AI != nil {
				if err := validateContext(ctx, OperationReview); err != nil {
					return ReviewResult{}, err
				}
				response, aiErr := s.AI.Complete(ctx, ai.ChatRequest{
					Purpose: "review", Document: reviewDoc, Findings: domainResult.Findings,
				})
				if aiErr != nil {
					if request.AI.Required {
						return ReviewResult{}, operationError(OperationReview, ErrorAI, input.label(), aiErr)
					}
					result.AIStatus = AIStatusFailed
					result.Status, result.Complete = StatusPartial, false
					result.Warnings = append(result.Warnings, Warning{
						Code: "ai_failed", Message: aiErr.Error(), Path: input.label(),
					})
				} else {
					domainResult.AIText = response.Content
				}
			}
			result.Findings = append(result.Findings, attributeFindings(process.ID, domainResult.Findings)...)
			result.Processes = append(result.Processes, ProcessReview{ProcessID: process.ID, Review: domainResult})
		}
	}
	if lint.ShouldFail(rawLintFindings(result.Findings), string(request.FailOn)) {
		result.Status = StatusFailed
	}
	return result, nil
}

func (s Service) Generate(ctx context.Context, request GenerateRequest) (GenerateResult, error) {
	if err := validateContext(ctx, OperationGenerate); err != nil {
		return GenerateResult{}, err
	}
	if err := validateGenerateLanguage(request.Lang); err != nil {
		return GenerateResult{}, err
	}
	inputs, opened, err := resolveBPMNInputs([]BPMNInput{request.Input}, request.ProjectDir, OperationGenerate)
	if err != nil {
		return GenerateResult{}, err
	}
	if err := validateContext(ctx, OperationGenerate); err != nil {
		return GenerateResult{}, err
	}
	document, err := parseInput(inputs[0], OperationGenerate)
	if err != nil {
		return GenerateResult{}, err
	}
	outDir := request.OutDir
	if outDir == "" && opened != nil {
		outDir, err = opened.Resolve(opened.Config.Paths.Tests)
		if err != nil {
			return GenerateResult{}, operationError(OperationGenerate, ErrorArtifact, opened.Config.Paths.Tests, err)
		}
	}
	if strings.TrimSpace(outDir) == "" {
		return GenerateResult{}, operationError(OperationGenerate, ErrorInvalidRequest, "", errors.New("output directory is required"))
	}
	result := GenerateResult{Status: StatusCompleted, Complete: true, Document: document}
	prepared, err := prepareArtifacts(ctx, document, outDir, request.Lang, request.Force)
	if err != nil {
		return GenerateResult{}, err
	}
	published, err := publishArtifacts(ctx, prepared)
	result.Artifacts = published
	if err != nil {
		result.Status, result.Complete = StatusPartial, false
		result.Warnings = append(result.Warnings, Warning{Code: "artifact_write_failed", Message: err.Error()})
		return result, operationError(OperationGenerate, ErrorArtifact, outDir, err)
	}
	return result, nil
}

func (s Service) Scan(ctx context.Context, request ScanRequest) (ScanResult, error) {
	if err := validateContext(ctx, OperationScan); err != nil {
		return ScanResult{}, err
	}
	if err := validateScanThreshold(request.FailOn); err != nil {
		return ScanResult{}, err
	}
	roots := append([]string(nil), request.Roots...)
	if len(roots) == 0 {
		if strings.TrimSpace(request.ProjectDir) == "" {
			return ScanResult{}, operationError(OperationScan, ErrorInvalidRequest, "", errors.New("scan roots or project directory are required"))
		}
		opened, err := project.Open(request.ProjectDir)
		if err != nil {
			return ScanResult{}, operationError(OperationScan, ErrorDiscovery, request.ProjectDir, err)
		}
		roots = []string{opened.Root}
	}
	result := ScanResult{Status: StatusCompleted, Complete: true}
	for _, root := range roots {
		if err := ctx.Err(); err != nil {
			return ScanResult{}, operationError(OperationScan, ErrorScan, root, err)
		}
		if _, err := os.Stat(root); err != nil {
			result.FailedRoots = append(result.FailedRoots, root)
			result.Warnings = append(result.Warnings, Warning{Code: "scan_failed", Message: err.Error(), Path: root})
			continue
		}
		scanResult, err := scan.WalkWithReport(scan.Options{Root: root, FailOn: string(request.FailOn)})
		if err != nil {
			result.FailedRoots = append(result.FailedRoots, root)
			result.Warnings = append(result.Warnings, Warning{Code: "scan_failed", Message: err.Error(), Path: root})
			continue
		}
		result.ScannedRoots = append(result.ScannedRoots, root)
		result.Findings = append(result.Findings, scanResult.Findings...)
		for _, issue := range scanResult.Issues {
			result.Warnings = append(result.Warnings, Warning{Code: "scan_incomplete", Message: issue.Err.Error(), Path: issue.Path})
		}
	}
	if len(result.ScannedRoots) == 0 {
		return ScanResult{}, operationError(OperationScan, ErrorScan, "", errors.New("all scan roots failed"))
	}
	if len(result.FailedRoots) > 0 {
		result.Status, result.Complete = StatusPartial, false
	}
	if len(result.Warnings) > 0 {
		result.Status, result.Complete = StatusPartial, false
	}
	if scan.ShouldFail(result.Findings, string(request.FailOn)) {
		result.Status = StatusFailed
	}
	return result, nil
}

func resolveBPMNInputs(inputs []BPMNInput, projectDir string, operation Operation) ([]BPMNInput, *project.Project, error) {
	var opened *project.Project
	openProject := func() (*project.Project, error) {
		if opened != nil {
			return opened, nil
		}
		if strings.TrimSpace(projectDir) == "" {
			return nil, errors.New("project directory is required for discovery or relative inputs")
		}
		value, err := project.Open(projectDir)
		if err != nil {
			return nil, err
		}
		opened = &value
		return opened, nil
	}
	if strings.TrimSpace(projectDir) != "" {
		if _, err := openProject(); err != nil {
			return nil, nil, operationError(operation, ErrorDiscovery, projectDir, err)
		}
	}
	if len(inputs) == 0 {
		value, err := openProject()
		if err != nil {
			return nil, nil, operationError(operation, ErrorDiscovery, projectDir, err)
		}
		paths, err := value.Discover(project.AssetBPMN)
		if err != nil {
			return nil, nil, operationError(operation, ErrorDiscovery, projectDir, err)
		}
		if len(paths) == 0 {
			return nil, nil, operationError(operation, ErrorDiscovery, value.Root, errors.New("no BPMN inputs discovered"))
		}
		resolved := make([]BPMNInput, 0, len(paths))
		for _, path := range paths {
			resolved = append(resolved, BPMNInput{Name: path, Path: path})
		}
		return resolved, opened, nil
	}
	resolved := make([]BPMNInput, 0, len(inputs))
	for _, input := range inputs {
		if len(input.Content) > 0 {
			if strings.TrimSpace(input.Name) == "" {
				input.Name = "<memory>"
			}
			resolved = append(resolved, input)
			continue
		}
		if strings.TrimSpace(input.Path) == "" {
			return nil, opened, operationError(operation, ErrorInvalidRequest, input.Name, errors.New("BPMN content or path is required"))
		}
		if filepath.IsAbs(input.Path) {
			resolved = append(resolved, input)
			continue
		}
		value, err := openProject()
		if err != nil {
			return nil, opened, operationError(operation, ErrorDiscovery, projectDir, err)
		}
		path, err := value.ResolveInput(project.AssetBPMN, input.Path)
		if err != nil {
			return nil, opened, operationError(operation, ErrorInput, input.Path, err)
		}
		input.Path = path
		if input.Name == "" {
			input.Name = path
		}
		resolved = append(resolved, input)
	}
	return resolved, opened, nil
}

func parseInput(input BPMNInput, operation Operation) (bpmn.Document, error) {
	var (
		document bpmn.Document
		err      error
	)
	if len(input.Content) > 0 {
		document, err = bpmn.Parse(bytes.NewReader(input.Content))
	} else {
		document, err = bpmn.ParseFile(input.Path)
	}
	if err != nil {
		return bpmn.Document{}, operationError(operation, ErrorInput, input.label(), err)
	}
	return document, nil
}

func (input BPMNInput) label() string {
	if input.Name != "" {
		return input.Name
	}
	if input.Path != "" {
		return input.Path
	}
	return "<memory>"
}

func hasInput(input BPMNInput) bool {
	return len(input.Content) > 0 || strings.TrimSpace(input.Path) != ""
}

func processDocument(document bpmn.Document, process bpmn.Process) bpmn.Document {
	return bpmn.Document{
		Processes: []bpmn.Process{process}, Messages: document.Messages, Errors: document.Errors,
		UnknownExtensions: document.UnknownExtensions, ProcessID: process.ID, Name: process.Name,
		Elements: process.Elements, Flows: process.Flows,
	}
}

func runLint(document bpmn.Document, file string, ignore []string) []LintFinding {
	findings := runDocumentLint(document, file, ignore)
	for _, process := range document.Processes {
		processDoc := processDocument(document, process)
		processDoc.Messages = nil
		findings = append(findings, attributeFindings(process.ID, lint.Run(processDoc, lint.Options{
			File: file, Ignore: ignore,
		}))...)
	}
	return findings
}

func runDocumentLint(document bpmn.Document, file string, ignore []string) []LintFinding {
	documentOnly := bpmn.Document{Messages: document.Messages}
	return attributeFindings("", lint.Run(documentOnly, lint.Options{File: file, Ignore: ignore}))
}

func attributeFindings(processID string, findings []lint.Finding) []LintFinding {
	attributed := make([]LintFinding, 0, len(findings))
	for _, finding := range findings {
		attributed = append(attributed, LintFinding{ProcessID: processID, Finding: finding})
	}
	return attributed
}

func rawLintFindings(findings []LintFinding) []lint.Finding {
	raw := make([]lint.Finding, 0, len(findings))
	for _, finding := range findings {
		raw = append(raw, finding.Finding)
	}
	return raw
}

func compareDocuments(before, after bpmn.Document) []ProcessChange {
	beforeByID := make(map[string]bpmn.Process, len(before.Processes))
	afterByID := make(map[string]bpmn.Process, len(after.Processes))
	ids := map[string]bool{}
	for _, process := range before.Processes {
		beforeByID[process.ID], ids[process.ID] = process, true
	}
	for _, process := range after.Processes {
		afterByID[process.ID], ids[process.ID] = process, true
	}
	ordered := make([]string, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Strings(ordered)
	var changes []ProcessChange
	for _, id := range ordered {
		beforeProcess, beforeOK := beforeByID[id]
		afterProcess, afterOK := afterByID[id]
		if !beforeOK {
			changes = append(changes, ProcessChange{Kind: ProcessAdded, AfterProcessID: id})
			continue
		}
		if !afterOK {
			changes = append(changes, ProcessChange{Kind: ProcessRemoved, BeforeProcessID: id})
			continue
		}
		beforeDoc := processDocument(before, beforeProcess)
		beforeDoc.Messages = nil
		afterDoc := processDocument(after, afterProcess)
		afterDoc.Messages = nil
		for _, domainChange := range bpmndiff.Compare(beforeDoc, afterDoc) {
			change := domainChange
			changes = append(changes, ProcessChange{
				Kind: ProcessModified, BeforeProcessID: id, AfterProcessID: id, Change: &change,
			})
		}
	}
	for _, domainChange := range bpmndiff.Compare(
		bpmn.Document{Messages: before.Messages},
		bpmn.Document{Messages: after.Messages},
	) {
		change := domainChange
		changes = append(changes, ProcessChange{Kind: DocumentChanged, Change: &change})
	}
	return changes
}

func validateContext(ctx context.Context, operation Operation) error {
	if err := ctx.Err(); err != nil {
		return operationError(operation, ErrorInput, "", err)
	}
	return nil
}

func validateLintThreshold(operation Operation, threshold LintThreshold) error {
	switch threshold {
	case "", LintThresholdError, LintThresholdWarning:
		return nil
	default:
		return operationError(operation, ErrorInvalidRequest, "", errors.New("fail threshold must be error or warning"))
	}
}

func validateScanThreshold(threshold ScanThreshold) error {
	switch threshold {
	case "", ScanThresholdLow, ScanThresholdMedium, ScanThresholdHigh:
		return nil
	default:
		return operationError(OperationScan, ErrorInvalidRequest, "", errors.New("fail threshold must be low, medium, or high"))
	}
}

func validateGenerateLanguage(language GenerateLanguage) error {
	switch language {
	case "", GenerateLanguageJava, GenerateLanguageJavaScript:
		return nil
	default:
		return operationError(OperationGenerate, ErrorInvalidRequest, "", errors.New("language must be java or js"))
	}
}

type preparedArtifact struct {
	artifact Artifact
	existed  bool
	original []byte
	mode     os.FileMode
}

func prepareArtifacts(
	ctx context.Context,
	document bpmn.Document,
	outDir string,
	language GenerateLanguage,
	force bool,
) ([]preparedArtifact, error) {
	if err := validateContext(ctx, OperationGenerate); err != nil {
		return nil, err
	}
	staging, err := os.MkdirTemp("", "camunda-lab-generate-*")
	if err != nil {
		return nil, operationError(OperationGenerate, ErrorArtifact, outDir, err)
	}
	defer os.RemoveAll(staging)

	seen := make(map[string]string)
	var prepared []preparedArtifact
	for _, process := range document.Processes {
		if err := ctx.Err(); err != nil {
			return nil, operationError(OperationGenerate, ErrorArtifact, outDir, err)
		}
		processRoot, err := os.MkdirTemp(staging, "process-*")
		if err != nil {
			return nil, operationError(OperationGenerate, ErrorArtifact, outDir, err)
		}
		paths, err := testgen.Generate(processDocument(document, process), testgen.Options{
			OutDir: processRoot, Lang: string(language),
		})
		if err != nil {
			return nil, operationError(OperationGenerate, ErrorArtifact, outDir, err)
		}
		for _, stagedPath := range paths {
			relative, err := filepath.Rel(processRoot, stagedPath)
			if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				if err == nil {
					err = errors.New("generated artifact escapes staging directory")
				}
				return nil, operationError(OperationGenerate, ErrorArtifact, stagedPath, err)
			}
			finalPath := filepath.Join(outDir, relative)
			key := strings.ToLower(filepath.Clean(finalPath))
			if previous, duplicate := seen[key]; duplicate {
				return nil, operationError(
					OperationGenerate,
					ErrorArtifact,
					finalPath,
					errors.New("generated artifact path collides for processes "+previous+" and "+process.ID),
				)
			}
			seen[key] = process.ID
			if err := validateContext(ctx, OperationGenerate); err != nil {
				return nil, err
			}
			content, err := os.ReadFile(stagedPath)
			if err != nil {
				return nil, operationError(OperationGenerate, ErrorArtifact, stagedPath, err)
			}
			item := preparedArtifact{artifact: Artifact{
				Path: finalPath, MediaType: generatedMediaType(finalPath), Content: content,
			}}
			info, statErr := os.Stat(finalPath)
			switch {
			case statErr == nil && info.IsDir():
				return nil, operationError(OperationGenerate, ErrorArtifact, finalPath, errors.New("artifact path is a directory"))
			case statErr == nil && !force:
				return nil, operationError(OperationGenerate, ErrorArtifact, finalPath, errors.New("artifact exists (use force to replace)"))
			case statErr == nil:
				item.existed = true
				item.mode = info.Mode().Perm()
				item.original, err = os.ReadFile(finalPath)
				if err != nil {
					return nil, operationError(OperationGenerate, ErrorArtifact, finalPath, err)
				}
			case !os.IsNotExist(statErr):
				return nil, operationError(OperationGenerate, ErrorArtifact, finalPath, statErr)
			}
			prepared = append(prepared, item)
		}
	}
	return prepared, nil
}

func publishArtifacts(ctx context.Context, prepared []preparedArtifact) ([]Artifact, error) {
	published := make([]Artifact, 0, len(prepared))
	var createdDirs []string
	for index, item := range prepared {
		if err := validateContext(ctx, OperationGenerate); err != nil {
			_, rollbackErr := rollbackArtifacts(prepared[:index], published, err)
			removeCreatedDirs(createdDirs)
			return nil, rollbackErr
		}
		dirs, err := makeArtifactDir(filepath.Dir(item.artifact.Path))
		createdDirs = append(createdDirs, dirs...)
		if err != nil {
			_, rollbackErr := rollbackArtifacts(prepared[:index], published, err)
			removeCreatedDirs(createdDirs)
			return nil, rollbackErr
		}
		if err := atomicWrite(item.artifact.Path, item.artifact.Content, 0o644); err != nil {
			_, rollbackErr := rollbackArtifacts(prepared[:index], published, err)
			removeCreatedDirs(createdDirs)
			return nil, rollbackErr
		}
		published = append(published, item.artifact)
	}
	return published, nil
}

func makeArtifactDir(path string) ([]string, error) {
	var missing []string
	for current := path; ; current = filepath.Dir(current) {
		if _, err := os.Stat(current); err == nil {
			break
		} else if !os.IsNotExist(err) {
			return missing, err
		}
		missing = append(missing, current)
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return missing, err
	}
	return missing, nil
}

func removeCreatedDirs(paths []string) {
	sort.SliceStable(paths, func(i, j int) bool {
		return len(paths[i]) > len(paths[j])
	})
	for _, path := range paths {
		_ = os.Remove(path)
	}
}

func rollbackArtifacts(prepared []preparedArtifact, published []Artifact, publishErr error) ([]Artifact, error) {
	for index := len(prepared) - 1; index >= 0; index-- {
		item := prepared[index]
		var err error
		if item.existed {
			err = atomicWrite(item.artifact.Path, item.original, item.mode)
		} else {
			err = os.Remove(item.artifact.Path)
			if os.IsNotExist(err) {
				err = nil
			}
		}
		if err != nil {
			return published, errors.Join(publishErr, errors.New("artifact rollback failed"), err)
		}
	}
	return nil, publishErr
}

func atomicWrite(path string, content []byte, mode os.FileMode) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".camunda-lab-artifact-*")
	if err != nil {
		return err
	}
	tempPath := file.Name()
	defer os.Remove(tempPath)
	if err := file.Chmod(mode); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(content); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func generatedMediaType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".js":
		return "text/javascript"
	case ".java":
		return "text/x-java-source"
	default:
		return "application/octet-stream"
	}
}
