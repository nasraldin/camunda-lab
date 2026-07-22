package drift

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/nasraldin/camunda-lab/internal/cluster"
	"github.com/nasraldin/camunda-lab/internal/env"
	"github.com/nasraldin/camunda-lab/internal/inventory"
	"github.com/nasraldin/camunda-lab/internal/plan"
	"github.com/nasraldin/camunda-lab/internal/project"
	"gopkg.in/yaml.v3"
)

type Request struct {
	ProjectRoot string                  `json:"projectRoot"`
	GitRef      string                  `json:"gitRef"`
	Environment string                  `json:"environment,omitempty"`
	Limits      cluster.InventoryLimits `json:"-"`
}

type Service struct {
	Factory       cluster.Factory
	Git           GitRunner
	BuildWorking  func(inventory.LocalRequest) (inventory.Inventory, error)
	BuildDeployed func(context.Context, Request) (inventory.Inventory, env.Resolved, error)
}

func NewService(factory cluster.Factory) *Service {
	return &Service{Factory: factory, Git: ExecGitRunner{}}
}

func (s *Service) Run(ctx context.Context, request Request) (Report, error) {
	report := emptyThreeWayReport()
	if err := ctx.Err(); err != nil {
		return report, err
	}
	if strings.TrimSpace(request.ProjectRoot) == "" {
		return report, errors.New("project root is required")
	}
	if err := validateGitRef(request.GitRef); err != nil {
		return report, err
	}
	report.Baseline.Ref = request.GitRef
	if s == nil {
		return report, errors.New("drift service is required")
	}
	runner := s.Git
	if runner == nil {
		runner = ExecGitRunner{}
	}
	opened, err := project.Open(request.ProjectRoot)
	if err != nil {
		return report, fmt.Errorf("open working project: %w", err)
	}
	buildWorking := s.BuildWorking
	if buildWorking == nil {
		buildWorking = inventory.BuildLocal
	}
	working, err := buildWorking(inventory.LocalRequest{Root: opened.Root})
	if err != nil {
		return refuse(report, "working-inventory", err), fmt.Errorf("build working canonical inventory: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	report.Working = Snapshot{Source: working.Source, Total: len(working.Resources)}

	buildDeployed := s.BuildDeployed
	if buildDeployed == nil {
		if s.Factory == nil {
			err := errors.New("drift service requires a cluster factory")
			return unknown(report, "deployed-inventory", err), err
		}
		buildDeployed = func(ctx context.Context, value Request) (inventory.Inventory, env.Resolved, error) {
			return cluster.BuildClusterInventory(ctx, s.Factory, cluster.InventoryRequest{
				Environment: value.Environment, ProjectRoot: opened.Root, Limits: value.Limits,
			})
		}
	}
	deployed, resolved, err := buildDeployed(ctx, request)
	report.Environment, report.Env = resolved, resolved.Profile.Name
	if err != nil {
		return unknown(report, "deployed-inventory", err), fmt.Errorf("build deployed canonical inventory: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	report.Deployed = Snapshot{Source: deployed.Source, Total: len(deployed.Resources)}
	if request.Environment != "" && resolved.Profile.Name != request.Environment {
		return refuse(report, "resolved-environment", fmt.Errorf(
			"resolved environment %q does not match explicit request %q",
			resolved.Profile.Name, request.Environment,
		)), nil
	}

	repository, err := resolveRepository(ctx, runner, opened.Root)
	if err != nil {
		return unknown(report, "git-baseline", err), err
	}
	commit, err := resolveCommit(ctx, runner, repository, request.GitRef)
	if err != nil {
		return unknown(report, "git-baseline", err), err
	}
	report.Baseline = Baseline{Ref: request.GitRef, Commit: commit, Repository: repository}
	baseline, baselineConfig, err := buildBaselineInventory(ctx, runner, repository, opened.Root, commit)
	if err != nil {
		return refuse(report, "git-baseline", err), fmt.Errorf("build Git baseline inventory: %w", err)
	}
	workingAssetPaths := inventoryAssetPaths(repository, opened.Root, working.Resources)
	if err := inspectEffectiveWorkingFilter(ctx, runner, repository, workingAssetPaths); err != nil {
		return refuse(report, "git-filters", err), fmt.Errorf("effective working Git filter risk: %w", err)
	}
	changes, err := workingChanges(ctx, runner, repository, opened.Root, opened.Config, baselineConfig)
	if err != nil {
		return unknown(report, "git-working-tree", err), fmt.Errorf("inspect Git working tree: %w", err)
	}
	report.Changes, report.Dirty = changes, len(changes) != 0
	return buildThreeWayReport(report, baseline, working, deployed, resolved), nil
}

func emptyThreeWayReport() Report {
	return Report{
		Status: StatusUnknown, Changes: make([]Change, 0), Warnings: make([]inventory.Warning, 0),
		Entries: make([]Entry, 0), Policy: Policy{Outcome: "unknown", ExitCode: 2},
		Source:            Comparison{Name: "baseline-working"},
		Deployment:        Comparison{Name: "baseline-deployed"},
		PendingDeployment: Comparison{Name: "working-deployed"},
	}
}

func refuse(report Report, capability string, cause error) Report {
	report.Status, report.Complete, report.Comparable = StatusRefused, false, false
	report.Policy = Policy{Outcome: "refused", ExitCode: 2}
	report.Warnings = append(report.Warnings, inventory.Warning{
		Capability: capability, Message: safeWarningMessage(capability, cause),
	})
	sortWarnings(report.Warnings)
	return report
}

func unknown(report Report, capability string, cause error) Report {
	report.Status, report.Complete, report.Comparable = StatusUnknown, false, false
	report.Policy = Policy{Outcome: "unknown", ExitCode: 2}
	report.Warnings = append(report.Warnings, inventory.Warning{
		Capability: capability, Message: safeWarningMessage(capability, cause),
	})
	sortWarnings(report.Warnings)
	return report
}

type safeReportError struct {
	message string
	cause   error
}

func (e *safeReportError) Error() string { return e.message }
func (e *safeReportError) Unwrap() error { return e.cause }

func reportError(message string, cause error) error {
	return &safeReportError{message: message, cause: cause}
}

func safeWarningMessage(capability string, cause error) string {
	var safe *safeReportError
	if errors.As(cause, &safe) {
		return safe.message
	}
	switch capability {
	case "working-inventory":
		return "working inventory is not comparable"
	case "deployed-inventory":
		return "deployed inventory operation failed"
	case "git-baseline":
		return "Git baseline inventory is not comparable"
	case "git-working-tree":
		return "Git working-tree inspection failed"
	case "git-filters":
		return "Git attribute inspection could not prove canonical byte equivalence"
	case "resolved-environment":
		return "resolved environment does not match the explicit request"
	case "three-way-comparison":
		return "baseline, working, and deployed inventories are not fully comparable"
	default:
		return "drift comparison could not complete safely"
	}
}

func buildBaselineInventory(ctx context.Context, runner GitRunner, repository, projectRoot, commit string) (inventory.Inventory, project.Config, error) {
	projectRelative, err := filepath.Rel(repository, projectRoot)
	if err != nil || projectRelative == ".." || strings.HasPrefix(projectRelative, ".."+string(filepath.Separator)) {
		return inventory.Inventory{}, project.Config{}, errors.New("project root is outside Git repository")
	}
	projectPrefix := filepath.ToSlash(projectRelative)
	if projectPrefix == "." {
		projectPrefix = ""
	}
	pathspec := projectPrefix
	if pathspec == "" {
		pathspec = "."
	}
	treeOutput, err := runSafeGit(ctx, runner, repository,
		"ls-tree", "-r", "-z", "--full-tree", commit, "--", pathspec)
	if err != nil {
		return inventory.Inventory{}, project.Config{}, err
	}
	treeEntries, err := parseTreeEntries(treeOutput)
	if err != nil {
		return inventory.Inventory{}, project.Config{}, err
	}
	configPath := joinGitPath(projectPrefix, project.ConfigFileName)
	configEntry, found := findTreeEntry(treeEntries, configPath)
	if !found || configEntry.Type != "blob" {
		return inventory.Inventory{}, project.Config{}, fmt.Errorf("baseline commit does not contain %s", configPath)
	}
	configBytes, err := runSafeGit(ctx, runner, repository, "cat-file", "blob", configEntry.Object)
	if err != nil {
		return inventory.Inventory{}, project.Config{}, fmt.Errorf("read baseline project config: %w", err)
	}
	var cfg project.Config
	if err := yaml.Unmarshal(configBytes, &cfg); err != nil {
		return inventory.Inventory{}, project.Config{}, fmt.Errorf("parse baseline project config: %w", err)
	}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return inventory.Inventory{}, project.Config{}, fmt.Errorf("validate baseline project config: %w", err)
	}

	roots := make([]string, 0, len(assetSpecs(cfg)))
	for _, spec := range assetSpecs(cfg) {
		roots = append(roots, joinGitPath(projectPrefix, spec.path))
	}
	for _, entry := range treeEntries {
		if entry.Mode != "160000" && entry.Type != "commit" {
			continue
		}
		for _, root := range roots {
			if pathsIntersect(entry.Path, root) {
				detail := fmt.Errorf("gitlink %s intersects configured asset root %s", entry.Path, root)
				return inventory.Inventory{}, project.Config{}, reportError(
					"gitlink intersects a configured asset root", detail,
				)
			}
		}
	}
	baselineAssets := make([]string, 0)
	for _, entry := range treeEntries {
		if entry.Type != "blob" {
			continue
		}
		for _, spec := range assetSpecs(cfg) {
			root := joinGitPath(projectPrefix, spec.path)
			if pathWithinGitRoot(entry.Path, root) && acceptsKind(spec.kind, entry.Path) {
				baselineAssets = append(baselineAssets, entry.Path)
				break
			}
		}
	}
	attributeFiles, err := baselineAttributeFiles(
		ctx, runner, repository, commit, projectPrefix, treeEntries,
	)
	if err != nil {
		return inventory.Inventory{}, project.Config{}, err
	}
	if err := inspectFilterRisk(attributeFiles, baselineAssets); err != nil {
		return inventory.Inventory{}, project.Config{}, err
	}

	source := inventory.Source{Type: "local", ProjectRoot: projectRoot}
	result := inventory.Inventory{Source: source}
	seen := make(map[string]string)
	for _, spec := range assetSpecs(cfg) {
		root := joinGitPath(projectPrefix, spec.path)
		for _, entry := range treeEntries {
			if entry.Type != "blob" || !pathWithinGitRoot(entry.Path, root) ||
				!acceptsKind(spec.kind, entry.Path) {
				continue
			}
			raw, err := runSafeGit(ctx, runner, repository, "cat-file", "blob", entry.Object)
			if err != nil {
				return inventory.Inventory{}, project.Config{}, fmt.Errorf("read baseline asset %s: %w", entry.Path, err)
			}
			if isLFSPointer(raw) {
				detail := fmt.Errorf("baseline asset %s is a Git LFS pointer", entry.Path)
				return inventory.Inventory{}, project.Config{}, reportError(
					"Git LFS pointer prevents canonical comparison", detail,
				)
			}
			relative := strings.TrimPrefix(entry.Path, projectPrefix+"/")
			if projectPrefix == "" {
				relative = entry.Path
			}
			resources, err := resourcesFromBlob(spec.kind, relative, raw, source)
			if err != nil {
				return inventory.Inventory{}, project.Config{}, fmt.Errorf("inventory baseline asset %s: %w", relative, err)
			}
			for _, resource := range resources {
				key := resource.Kind.String() + "\x00" + resource.ID
				if prior, duplicate := seen[key]; duplicate {
					return inventory.Inventory{}, project.Config{}, fmt.Errorf(
						"duplicate %s resource ID %q in %s and %s", resource.Kind, resource.ID, prior, resource.Path,
					)
				}
				seen[key] = resource.Path
				result.Resources = append(result.Resources, resource)
			}
		}
	}
	sortResources(result.Resources)
	if err := result.ValidateComparable(); err != nil {
		return inventory.Inventory{}, project.Config{}, err
	}
	return result, cfg, nil
}

type assetSpec struct {
	kind inventory.Kind
	path string
}

func assetSpecs(cfg project.Config) []assetSpec {
	return []assetSpec{
		{kind: inventory.KindProcess, path: filepath.ToSlash(cfg.Paths.BPMN)},
		{kind: inventory.KindDecision, path: filepath.ToSlash(cfg.Paths.DMN)},
		{kind: inventory.KindForm, path: filepath.ToSlash(cfg.Paths.Forms)},
	}
}

func resourcesFromBlob(kind inventory.Kind, path string, raw []byte, source inventory.Source) ([]inventory.Resource, error) {
	ids, err := inventory.ResourceIDs(kind, raw)
	if err != nil {
		return nil, err
	}
	result := make([]inventory.Resource, 0, len(ids))
	if kind == inventory.KindProcess {
		for _, id := range ids {
			digest, err := inventory.DigestCanonicalProcess(raw, id)
			if err != nil {
				return nil, err
			}
			result = append(result, inventory.Resource{
				Kind: kind, ID: id, Path: path, Digest: digest, Source: source,
			})
		}
		return result, nil
	}
	digest, err := inventory.DigestCanonical(kind, raw)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		result = append(result, inventory.Resource{
			Kind: kind, ID: id, Path: path, Digest: digest, Source: source,
		})
	}
	return result, nil
}

func workingChanges(ctx context.Context, runner GitRunner, repository, projectRoot string, configs ...project.Config) ([]Change, error) {
	projectRelative, err := filepath.Rel(repository, projectRoot)
	if err != nil || projectRelative == ".." || strings.HasPrefix(projectRelative, ".."+string(filepath.Separator)) {
		return nil, errors.New("project root is outside Git repository")
	}
	pathspec := filepath.ToSlash(projectRelative)
	if pathspec == "." {
		pathspec = "."
	}
	output, err := runSafeGit(ctx, runner, repository, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--", pathspec)
	if err != nil {
		return nil, err
	}
	records, err := splitNUL(output)
	if err != nil {
		return nil, err
	}
	changes := make([]Change, 0)
	for index := 0; index < len(records); index++ {
		record := records[index]
		if len(record) < 4 || record[2] != ' ' {
			return nil, errors.New("Git returned an invalid working-tree status record")
		}
		path := record[3:]
		relative := strings.TrimPrefix(path, filepath.ToSlash(projectRelative)+"/")
		if projectRelative == "." {
			relative = path
		}
		change := Change{Path: relative, Index: string(record[0]), Worktree: string(record[1])}
		change.Untracked = record[:2] == "??"
		change.Deleted = record[0] == 'D' || record[1] == 'D'
		if record[0] == 'R' || record[0] == 'C' || record[1] == 'R' || record[1] == 'C' {
			index++
			if index >= len(records) {
				return nil, errors.New("Git rename status omitted its source path")
			}
			from := records[index]
			change.RenamedFrom = strings.TrimPrefix(from, filepath.ToSlash(projectRelative)+"/")
			if projectRelative == "." {
				change.RenamedFrom = from
			}
		}
		if configuredChange(relative, configs...) || configuredChange(change.RenamedFrom, configs...) {
			changes = append(changes, change)
		}
	}
	sort.SliceStable(changes, func(i, j int) bool {
		if changes[i].Path != changes[j].Path {
			return changes[i].Path < changes[j].Path
		}
		return changes[i].RenamedFrom < changes[j].RenamedFrom
	})
	return changes, nil
}

func configuredChange(path string, configs ...project.Config) bool {
	if path == project.ConfigFileName {
		return true
	}
	for _, cfg := range configs {
		for _, spec := range assetSpecs(cfg) {
			if pathWithinGitRoot(filepath.ToSlash(path), spec.path) && acceptsKind(spec.kind, path) {
				return true
			}
		}
	}
	return false
}

func buildThreeWayReport(report Report, baseline, working, deployed inventory.Inventory, resolved env.Resolved) Report {
	baselinePlan, _ := plan.BuildResolved(baseline, deployed, resolved)
	workingPlan, _ := plan.BuildResolved(working, deployed, resolved)
	report.Warnings = append(report.Warnings, baselinePlan.Warnings...)
	report.Warnings = append(report.Warnings, workingPlan.Warnings...)
	report.Warnings = dedupeWarnings(report.Warnings)
	sourceComparable := baseline.ValidateComparable() == nil && working.ValidateComparable() == nil &&
		baseline.Source.Type == "local" && working.Source.Type == "local" &&
		baseline.Source.ProjectRoot == working.Source.ProjectRoot
	if !sourceComparable || !baselinePlan.Comparable || !workingPlan.Comparable ||
		baselinePlan.Status == plan.StatusUnknown || workingPlan.Status == plan.StatusUnknown {
		report = unknown(report, "three-way-comparison", errors.New("baseline, working, and deployed inventories are not fully comparable"))
		report.Warnings = dedupeWarnings(report.Warnings)
		return report
	}
	report.Status, report.Complete, report.Comparable = StatusReady, true, true
	report.Source.Comparable, report.Deployment.Comparable, report.PendingDeployment.Comparable = true, true, true
	baselineByID := indexLocal(baseline.Resources)
	workingByID := indexLocal(working.Resources)
	deployedByID := indexDeployed(deployed.Resources)
	keys := unionKeys(baselineByID, workingByID, deployedByID)
	for _, key := range keys {
		base, hasBase := baselineByID[key]
		work, hasWork := workingByID[key]
		remote, hasRemote := deployedByID[key]
		entry := Entry{
			Key:            strings.Replace(key, "\x00", "/", 1),
			BaselineDigest: base.Digest, WorkingDigest: work.Digest, DeployedDigest: remote.Digest,
			BaselinePath: base.Path, WorkingPath: work.Path, DeployedPath: remote.Path,
			GitDigest: base.Digest, ClusterDigest: remote.Digest,
		}
		if remote.Version > 0 {
			entry.DeployedVersion = strconv.FormatInt(remote.Version, 10)
			entry.ClusterVer = entry.DeployedVersion
		}
		entry.SourceDrift = sourceStatus(base, hasBase, work, hasWork)
		entry.DeploymentDrift = deploymentStatus(base, hasBase, remote, hasRemote)
		entry.PendingDeployment = deploymentStatus(work, hasWork, remote, hasRemote)
		entry.Status = string(entry.PendingDeployment)
		entry.Detail = detailFor(entry, hasBase, hasWork, hasRemote)
		if hasBase || hasWork {
			addCount(&report.Source.Counts, entry.SourceDrift)
		}
		if hasBase || hasRemote {
			addCount(&report.Deployment.Counts, entry.DeploymentDrift)
		}
		if hasWork || hasRemote {
			addCount(&report.PendingDeployment.Counts, entry.PendingDeployment)
		}
		report.Entries = append(report.Entries, entry)
	}
	if HasDrift(report) {
		report.Policy = Policy{Outcome: "drift", ExitCode: 1}
	} else {
		report.Policy = Policy{Outcome: "in-sync", ExitCode: 0}
	}
	return report
}

func sourceStatus(base inventory.Resource, hasBase bool, work inventory.Resource, hasWork bool) ComparisonStatus {
	switch {
	case !hasBase && !hasWork:
		return ComparisonInSync
	case hasBase && hasWork && base.Digest == work.Digest:
		return ComparisonInSync
	case hasWork && !hasBase:
		return ComparisonLocalOnly
	default:
		return ComparisonDrift
	}
}

func deploymentStatus(local inventory.Resource, hasLocal bool, remote inventory.Resource, hasRemote bool) ComparisonStatus {
	switch {
	case hasLocal && hasRemote && local.Digest == remote.Digest:
		return ComparisonInSync
	case hasLocal && !hasRemote:
		return ComparisonLocalOnly
	case !hasLocal && hasRemote:
		return ComparisonClusterOnly
	case hasLocal && hasRemote:
		return ComparisonDrift
	default:
		return ComparisonInSync
	}
}

func detailFor(entry Entry, hasBase, hasWork, hasRemote bool) string {
	var details []string
	if hasBase && !hasWork {
		details = append(details, "baseline resource is deleted from the working project")
	}
	if !hasBase && hasWork {
		details = append(details, "working project contains a resource absent from the baseline")
	}
	if hasRemote && !hasBase {
		details = append(details, "deployed resource is absent from the baseline")
	}
	if !hasRemote && (hasBase || hasWork) {
		details = append(details, "resource is not deployed")
	}
	if len(details) == 0 {
		if entry.SourceDrift == ComparisonInSync && entry.DeploymentDrift == ComparisonInSync {
			return "canonical content matches across baseline, working project, and deployment"
		}
		return "canonical resource content differs between snapshots"
	}
	return strings.Join(details, "; ")
}

func indexLocal(resources []inventory.Resource) map[string]inventory.Resource {
	result := make(map[string]inventory.Resource, len(resources))
	for _, resource := range resources {
		result[resource.Kind.String()+"\x00"+resource.ID] = resource
	}
	return result
}

func indexDeployed(resources []inventory.Resource) map[string]inventory.Resource {
	result := make(map[string]inventory.Resource, len(resources))
	for _, resource := range resources {
		key := resource.Kind.String() + "\x00" + resource.ID
		current, found := result[key]
		if !found || resource.Version > current.Version ||
			resource.Version == current.Version && resource.Key < current.Key {
			result[key] = resource
		}
	}
	return result
}

func unionKeys(values ...map[string]inventory.Resource) []string {
	set := make(map[string]struct{})
	for _, value := range values {
		for key := range value {
			set[key] = struct{}{}
		}
	}
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func addCount(counts *Counts, status ComparisonStatus) {
	counts.Total++
	switch status {
	case ComparisonInSync:
		counts.InSync++
	case ComparisonDrift:
		counts.Drift++
	case ComparisonLocalOnly:
		counts.LocalOnly++
	case ComparisonClusterOnly:
		counts.ClusterOnly++
	default:
		counts.Unknown++
	}
}

func sortResources(values []inventory.Resource) {
	sort.SliceStable(values, func(i, j int) bool {
		if values[i].Kind != values[j].Kind {
			return values[i].Kind < values[j].Kind
		}
		if values[i].ID != values[j].ID {
			return values[i].ID < values[j].ID
		}
		return values[i].Path < values[j].Path
	})
}

func sortWarnings(values []inventory.Warning) {
	sort.SliceStable(values, func(i, j int) bool {
		if values[i].Capability != values[j].Capability {
			return values[i].Capability < values[j].Capability
		}
		return values[i].Message < values[j].Message
	})
}

func dedupeWarnings(values []inventory.Warning) []inventory.Warning {
	sortWarnings(values)
	if len(values) < 2 {
		return values
	}
	result := values[:1]
	for _, value := range values[1:] {
		previous := result[len(result)-1]
		if value.Capability != previous.Capability || value.Message != previous.Message {
			result = append(result, value)
		}
	}
	return result
}

func joinGitPath(parts ...string) string {
	var clean []string
	for _, part := range parts {
		if part != "" && part != "." {
			clean = append(clean, filepath.ToSlash(part))
		}
	}
	return strings.Join(clean, "/")
}

func pathWithinGitRoot(path, root string) bool {
	return path == root || strings.HasPrefix(path, strings.TrimSuffix(root, "/")+"/")
}

func acceptsKind(kind inventory.Kind, path string) bool {
	switch kind {
	case inventory.KindProcess:
		return strings.EqualFold(filepath.Ext(path), ".bpmn")
	case inventory.KindDecision:
		return strings.EqualFold(filepath.Ext(path), ".dmn")
	case inventory.KindForm:
		return strings.EqualFold(filepath.Ext(path), ".form")
	default:
		return false
	}
}

func findTreeEntry(values []treeEntry, wanted string) (treeEntry, bool) {
	for _, value := range values {
		if value.Path == wanted {
			return value, true
		}
	}
	return treeEntry{}, false
}

func pathsIntersect(left, right string) bool {
	return pathWithinGitRoot(left, right) || pathWithinGitRoot(right, left)
}

func inventoryAssetPaths(repository, projectRoot string, resources []inventory.Resource) []string {
	projectRelative, _ := filepath.Rel(repository, projectRoot)
	prefix := filepath.ToSlash(projectRelative)
	if prefix == "." {
		prefix = ""
	}
	set := make(map[string]struct{}, len(resources))
	for _, resource := range resources {
		set[joinGitPath(prefix, resource.Path)] = struct{}{}
	}
	paths := make([]string, 0, len(set))
	for path := range set {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}
