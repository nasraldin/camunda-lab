package toolkit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

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
	for index, process := range document.Processes {
		processDoc := processDocument(document, process)
		if index > 0 {
			processDoc.Messages = nil
			processDoc.Errors = nil
		}
		result.Processes = append(result.Processes, ProcessExplanation{
			ProcessID: process.ID, Explanation: explain.Offline(processDoc),
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
	type aiWork struct {
		input          string
		processIndex   int
		lintDocument   bpmn.Document
		promptDocument bpmn.Document
		promptFindings []lint.Finding
	}
	var aiWorks []aiWork
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
		documentFindings := lint.Run(bpmn.Document{Messages: document.Messages}, lint.Options{
			File: input.label(), Ignore: ignore, FailOn: string(request.FailOn),
		}).Findings
		result.Findings = append(result.Findings, attributeFindings("", documentFindings)...)
		for _, process := range document.Processes {
			reviewDoc := processDocument(document, process)
			lintDoc := reviewDoc
			lintDoc.Messages = nil
			domainResult, err := review.RunContext(ctx, lintDoc, review.Options{
				File: input.label(), FailOn: string(request.FailOn), Ignore: ignore,
			})
			if err != nil {
				return ReviewResult{}, operationError(OperationReview, ErrorInput, input.label(), err)
			}
			result.Findings = append(result.Findings, attributeFindings(process.ID, domainResult.Findings)...)
			result.Processes = append(result.Processes, ProcessReview{ProcessID: process.ID, Review: domainResult})
			aiWorks = append(aiWorks, aiWork{
				input: input.label(), processIndex: len(result.Processes) - 1,
				lintDocument: lintDoc, promptDocument: reviewDoc,
				promptFindings: documentFindings,
			})
		}
	}
	if lint.ShouldFail(rawLintFindings(result.Findings), string(request.FailOn)) {
		result.Status = StatusFailed
	}
	if !aiRequested {
		return result, nil
	}
	if s.AI == nil {
		result.Complete = false
		message := "AI client is not configured; choose a provider, model, and credentials"
		if request.AI.Required {
			result.AIStatus = AIStatusFailed
			err := &review.AIError{
				Stage: "configuration", Code: "ai_unavailable", Message: message,
				Err: errors.New(message),
			}
			return result, operationError(OperationReview, ErrorAI, "", err)
		}
		result.AIStatus = AIStatusSkipped
		if result.Status != StatusFailed {
			result.Status = StatusPartial
		}
		result.Warnings = append(result.Warnings, Warning{Code: "ai_unavailable", Message: message})
		return result, nil
	}
	result.AIStatus = AIStatusSucceeded
	for _, work := range aiWorks {
		domainResult, aiErr := review.RunContext(ctx, work.lintDocument, review.Options{
			File: work.input, FailOn: string(request.FailOn), Ignore: ignore,
			AI: true, AIRequired: request.AI.Required, AIClient: s.AI,
			PromptDocument: &work.promptDocument, PromptFindings: work.promptFindings,
		})
		result.Processes[work.processIndex].Review = domainResult
		if aiErr != nil {
			result.AIStatus = AIStatusFailed
			result.Complete = false
			var typedAIError *review.AIError
			if errors.As(aiErr, &typedAIError) {
				return result, operationError(OperationReview, ErrorAI, work.input, aiErr)
			}
			return result, operationError(OperationReview, ErrorInput, work.input, aiErr)
		}
		switch domainResult.AIStatus {
		case review.AIStatusFailed:
			result.AIStatus = AIStatusFailed
			result.Complete = false
			if result.Status != StatusFailed {
				result.Status = StatusPartial
			}
		case review.AIStatusSkipped:
			if result.AIStatus != AIStatusFailed {
				result.AIStatus = AIStatusSkipped
			}
			result.Complete = false
			if result.Status != StatusFailed {
				result.Status = StatusPartial
			}
		}
		for _, warning := range domainResult.Warnings {
			result.Warnings = append(result.Warnings, Warning{
				Code: warning.Code, Message: warning.Message, Path: work.input,
			})
			if warning.Code == "ai_prompt_compacted" {
				result.Complete = false
				if result.Status != StatusFailed {
					result.Status = StatusPartial
				}
			}
		}
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
	inputs, _, err := resolveBPMNInputs([]BPMNInput{request.Input}, request.ProjectDir, OperationGenerate)
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
	result := GenerateResult{Status: StatusCompleted, Complete: true, Document: document}
	rendered, err := testgen.Render(document, testgen.Options{Lang: string(request.Lang)})
	if err != nil {
		return GenerateResult{}, operationError(OperationGenerate, ErrorArtifact, "", err)
	}
	result.Artifacts = make([]Artifact, len(rendered))
	for index, artifact := range rendered {
		result.Artifacts[index] = Artifact{
			Path: artifact.Path, MediaType: artifact.MediaType, Content: append([]byte(nil), artifact.Content...),
		}
	}
	if strings.TrimSpace(request.OutDir) != "" {
		if err := validateContext(ctx, OperationGenerate); err != nil {
			return GenerateResult{}, err
		}
		paths, writeErr := testgen.Write(request.OutDir, rendered, request.Force)
		if writeErr != nil {
			result.Status, result.Complete = StatusPartial, false
			result.Warnings = append(result.Warnings, Warning{Code: "artifact_write_failed", Message: writeErr.Error()})
			return result, operationError(OperationGenerate, ErrorArtifact, request.OutDir, writeErr)
		}
		for index := range result.Artifacts {
			result.Artifacts[index].Path = paths[index]
		}
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
		if err := ctx.Err(); err != nil {
			return ScanResult{}, operationError(OperationScan, ErrorDiscovery, request.ProjectDir, err)
		}
		roots = []string{opened.Root}
	}
	scanRunner := s.scan
	if scanRunner == nil {
		scanRunner = scan.WalkWithReportContext
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
		if err := ctx.Err(); err != nil {
			return ScanResult{}, operationError(OperationScan, ErrorScan, root, err)
		}
		scanResult, err := scanRunner(ctx, scan.Options{
			Root: root, FailOn: string(request.FailOn), Ignore: request.Ignore,
		})
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return ScanResult{}, operationError(OperationScan, ErrorScan, root, err)
			}
			result.FailedRoots = append(result.FailedRoots, root)
			result.Warnings = append(result.Warnings, Warning{Code: "scan_failed", Message: err.Error(), Path: root})
			continue
		}
		result.ScannedRoots = append(result.ScannedRoots, root)
		result.Findings = append(result.Findings, scanResult.Findings...)
		result.Issues = append(result.Issues, scanResult.Issues...)
		result.Stats.Discovered += scanResult.Stats.Discovered
		result.Stats.Scanned += scanResult.Stats.Scanned
		result.Stats.Ignored += scanResult.Stats.Ignored
		result.Stats.Errored += scanResult.Stats.Errored
		for _, issue := range scanResult.Issues {
			if issue.Kind == scan.IssueIgnored {
				continue
			}
			result.Warnings = append(result.Warnings, Warning{
				Code: "scan_incomplete", Message: issue.Message, Path: issue.Path,
			})
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
	return attributeFindings("", lint.Run(document, lint.Options{
		File: file, Ignore: ignore,
	}).Findings)
}

func runDocumentLint(document bpmn.Document, file string, ignore []string) []LintFinding {
	documentOnly := bpmn.Document{Messages: document.Messages}
	return attributeFindings("", lint.Run(documentOnly, lint.Options{
		File: file, Ignore: ignore,
	}).Findings)
}

func attributeFindings(processID string, findings []lint.Finding) []LintFinding {
	attributed := make([]LintFinding, 0, len(findings))
	for _, finding := range findings {
		attributedProcessID := processID
		if attributedProcessID == "" {
			attributedProcessID = finding.ProcessID
		}
		finding.ProcessID = ""
		attributed = append(attributed, LintFinding{
			ProcessID: attributedProcessID,
			Finding:   finding,
		})
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
	var changes []ProcessChange
	for _, domainChange := range bpmndiff.Compare(before, after) {
		switch domainChange.Kind {
		case bpmndiff.ProcessAdded:
			changes = append(changes, ProcessChange{
				Kind: ProcessAdded, AfterProcessID: domainChange.ProcessID,
			})
		case bpmndiff.ProcessRemoved:
			changes = append(changes, ProcessChange{
				Kind: ProcessRemoved, BeforeProcessID: domainChange.ProcessID,
			})
		default:
			kind := DocumentChanged
			beforeProcessID, afterProcessID := "", ""
			if domainChange.ProcessID != "" {
				kind = ProcessModified
				beforeProcessID, afterProcessID = domainChange.ProcessID, domainChange.ProcessID
			}
			change := domainChange
			changes = append(changes, ProcessChange{
				Kind: kind, BeforeProcessID: beforeProcessID, AfterProcessID: afterProcessID, Change: &change,
			})
		}
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
	case "", GenerateLanguageJava, GenerateLanguageJavaScript, GenerateLanguagePython:
		return nil
	default:
		return operationError(OperationGenerate, ErrorInvalidRequest, "", errors.New("language must be java, js, or python"))
	}
}

type preparedArtifact struct {
	artifact Artifact
	existed  bool
	original []byte
	mode     os.FileMode
}

type artifactFileOps struct {
	write  func(string, []byte, os.FileMode) error
	remove func(string) error
}

func defaultArtifactFileOps() artifactFileOps {
	return artifactFileOps{write: atomicWrite, remove: os.Remove}
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
		rendered, err := testgen.Render(processDocument(document, process), testgen.Options{Lang: string(language)})
		if err != nil {
			return nil, operationError(OperationGenerate, ErrorArtifact, outDir, err)
		}
		paths, err := testgen.Write(processRoot, rendered, false)
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
	return publishArtifactsWithOps(ctx, prepared, defaultArtifactFileOps())
}

func publishArtifactsWithOps(ctx context.Context, prepared []preparedArtifact, ops artifactFileOps) ([]Artifact, error) {
	published := make([]Artifact, 0, len(prepared))
	var createdDirs []string
	for index, item := range prepared {
		if err := validateContext(ctx, OperationGenerate); err != nil {
			_, rollbackErr := rollbackArtifacts(prepared[:index], published, err, ops)
			removeCreatedDirs(createdDirs)
			return nil, rollbackErr
		}
		dirs, err := makeArtifactDir(filepath.Dir(item.artifact.Path))
		createdDirs = append(createdDirs, dirs...)
		if err != nil {
			_, rollbackErr := rollbackArtifacts(prepared[:index], published, err, ops)
			removeCreatedDirs(createdDirs)
			return nil, rollbackErr
		}
		if err := ops.write(item.artifact.Path, item.artifact.Content, 0o644); err != nil {
			_, rollbackErr := rollbackArtifacts(prepared[:index], published, err, ops)
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

func rollbackArtifacts(
	prepared []preparedArtifact,
	published []Artifact,
	publishErr error,
	ops artifactFileOps,
) ([]Artifact, error) {
	joined := publishErr
	rollbackFailed := false
	for index := len(prepared) - 1; index >= 0; index-- {
		item := prepared[index]
		var err error
		if item.existed {
			err = ops.write(item.artifact.Path, item.original, item.mode)
		} else {
			err = ops.remove(item.artifact.Path)
			if os.IsNotExist(err) {
				err = nil
			}
		}
		if err != nil {
			rollbackFailed = true
			joined = errors.Join(joined, fmt.Errorf(
				"rollback artifact %q: %w",
				filepath.Base(item.artifact.Path),
				err,
			))
		}
	}
	if rollbackFailed {
		return published, joined
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
