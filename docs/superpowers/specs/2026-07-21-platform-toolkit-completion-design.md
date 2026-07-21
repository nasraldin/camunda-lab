# Design: Complete the Phase 1–3 Platform Toolkit

**Status:** Approved  
**Date:** 2026-07-21  
**Branch:** `feat/platform-toolkit-ui-parity`  
**Pull request:** #16  
**Product contract:** [Platform Toolkit Vision](./2026-07-17-platform-toolkit-vision.md)

## Goal

Finish every Phase 1–3 capability promised by the platform-toolkit vision, detailed implementation plans, user-facing documentation, CLI help, API routes, and Lab UI. PR #16 is complete only when those surfaces provide the promised behavior end to end; command or page scaffolding alone is not completion.

The implementation remains complementary to official Camunda tooling. It must not add the items explicitly marked **Later**, **Out of scope**, or hard non-goals in the vision and individual plans.

## Source of truth and conflict resolution

The completion inventory is the union of:

1. `docs/superpowers/specs/2026-07-17-platform-toolkit-vision.md`
2. Every Phase 1–3 plan under `docs/superpowers/plans/`
3. User-facing claims in README, docs, CLI help, API routes, and Lab UI
4. Security and correctness requirements necessary to make those features safe

When sources conflict:

- Explicit **Later**, **Out of scope**, and hard non-goal statements win.
- A detailed feature plan is more specific than a summary roadmap statement.
- A safer behavior wins over a convenient but unsafe behavior.
- Unsupported behavior must return an actionable error or explicit `unknown` result. It must never be reported as success, clean, in-sync, or `noop`.
- Documentation must be corrected to describe the behavior that is actually verified.

## Completion definition

A feature is complete only when:

- Its domain implementation performs the promised behavior without placeholders.
- CLI flags, exit codes, text output, and JSON output match its plan.
- Its API validates inputs, returns stable structured results, and preserves domain errors.
- The Lab UI exposes the promised controls and does not silently bypass confirmation or safety gates.
- Unit and integration tests cover success, failure, and security paths.
- A representative real workflow is verified in the isolated light or full profile where applicable.

## Architecture

Keep CLI commands and HTTP handlers thin. Business logic belongs in focused Go packages and is shared by CLI and UI. External systems use injectable interfaces so tests can run without Docker, Kubernetes, Camunda, Git, or a paid AI provider.

The dependency direction is:

```text
CLI / HTTP handlers / React UI
              |
              v
       domain services
              |
              v
filesystem | Git | Camunda REST/OIDC | kubectl | AI provider
```

Stable domain request and result types are the contract between CLI, API, and UI. Presentation-specific formatting remains at the edge.

## Workstream 1: Safety foundation

This workstream precedes feature completion because later acceptance tests exercise destructive operations.

### Backup extraction

- Validate every archive entry before writing anything.
- Reject absolute paths, traversal, mixed-separator traversal, links, devices, unsupported entry types, duplicate destinations, excessive entry counts, excessive per-file sizes, and excessive total decompressed size.
- Extract into a staging directory and replace the destination only after complete validation and extraction.
- Preserve restrictive archive and restored-secret permissions.
- Refuse restore while the lab is running unless the documented force option is supplied.

### Environment identities

- Use one validator for CLI, API, and storage operations.
- Reject empty names, dot segments, separators, encoded separator equivalents, reserved names, and names that cannot round-trip safely as filenames.
- `env use` must require a valid existing profile.
- Removing the active profile must atomically return to `lab` or fail with a clear message.

### Local control-plane protection

- Validate loopback Host values and reject DNS-rebinding-style requests.
- Require same-origin browser mutations and a per-process CSRF token for JSON, form, and multipart writes.
- Keep read-only API routes usable by the local UI.
- Resolve symlinks before authorizing filesystem reads or writes.
- Show explicit confirmations in the UI for restore, incident resolution, Kubernetes restart/scale, reset, and equivalent destructive operations.

This protection is for the Camunda Lab API. It is separate from the Compose overlay that disables Camunda application CSRF for local cross-tab sessions.

## Workstream 2: Shared project and BPMN foundations

### Project configuration

- Validate scaffold inputs consistently in CLI and UI.
- Resolve configured BPMN, DMN, and form paths relative to the project root.
- Support recursive discovery where promised.
- Preserve project-local configuration as the primary source for project operations.
- Remove hardcoded Camunda versions from the UI.

### BPMN intermediate representation

- Parse all process definitions in a file, not only the first.
- Represent the task, subprocess, call activity, gateway, event, message, error, timer, boundary, sequence-flow, condition, retry, and attachment data required by downstream plans.
- Reject non-BPMN XML and BPMN documents with no usable process.
- Keep unknown extension elements without silently converting unsupported semantics into valid empty data.
- Derive graph traversal from sequence flows rather than element IDs or XML order.

Representative Camunda 8 fixtures must include multiple processes, nested flow nodes, boundary events, messages, timers, retries, and malformed or unsupported inputs.

## Workstream 3: Developer tooling

### `camunda lint`

- Implement every planned deterministic rule with positive and negative fixtures.
- Recurse configured BPMN paths.
- Load `.camunda.yaml` `lint.ignore`.
- Preserve planned exit codes and text/JSON reporters.

### `camunda diff`

- Compare normalized BPMN semantics rather than raw XML formatting.
- Report process, element type, message, event, condition, attachment, and flow changes required by the plan.
- Support documented file, project, and Git comparison modes with project-relative path resolution.
- Provide text and JSON output.
- DMN/form semantic diff remains excluded where its plan marks it as later scope.

### `camunda explain`

- Derive happy and alternate paths through graph reachability.
- Produce stable business and technical sections.
- Implement the documented optional AI enrichment without making offline output depend on AI.

### `camunda review`

- Keep deterministic lint review as the offline baseline.
- Wire a real provider-neutral AI client through existing AI configuration.
- Implement documented model, required/optional AI, timeout, credential, and JSON behaviors.
- Distinguish successful AI review, intentional skip, and provider failure.
- Expose equivalent controls in the Lab UI.

### `camunda test generate`

- Generate the planned Java and JavaScript skeletons with deterministic output and overwrite protection.
- Make UI-generated artifacts downloadable or explicitly writable to an authorized project path.
- Remove temporary artifacts after use.
- Running generated tests remains out of scope where the plan says so.

### `camunda scan`

- Respect `.gitignore`, `.camunda-scanignore`, and inline suppressions.
- Report unreadable, skipped, or truncated inputs and never claim a clean scan after a partial traversal.
- Preserve masking, severity thresholds, deterministic output, and JSON mode.

### `camunda doctor --deep`

- Include endpoint health, Compose service state, Docker volume/disk checks, overlay consistency, and explicit lab-down diagnosis.
- Provide stable grouped text and JSON output.
- Surface the deep check, rather than the basic doctor only, in the Home UI.

## Workstream 4: Environments and cluster operations

### `camunda env`

- Support project-local `environments/*.yaml` and global profiles with documented precedence.
- Store the active environment in the documented configuration model.
- Validate profiles before activation.
- Support interactive and non-interactive creation.
- Store only environment-variable names for secrets.
- Configure token URL, client ID, client secret reference, audience, and endpoint data required for remote OIDC.

The active environment must drive all cluster-aware commands and Lab UI operations consistently.

### `camunda plan`

- Use configured recursive resource paths and resource IDs rather than filenames alone.
- Canonicalize comparable resource semantics before digesting.
- Treat failed remote retrieval as an error or unknown state, never `noop`.
- Include documented create, update, delete, and noop actions, active environment selection, JSON output, and running-instance context where available.
- If a remote resource type cannot be inventoried through supported APIs, report it as unsupported rather than fabricate a comparison.

### `camunda drift`

- Compare the documented Git/project state with the active cluster.
- Implement tracked-file filtering, dirty-state reporting, cached mode, non-Git behavior, environment selection, and JSON output.
- Reuse plan canonicalization so equivalent XML is in sync.

### `camunda incidents`

- Implement list, show, limit, environment selection, text/JSON output, resolution confirmation, and documented Operate links.
- Refresh state after resolution and preserve API errors.

### `camunda trace`

- Produce deterministic chronological activity and incident state.
- Keep CLI and API state derivation identical.
- Implement text/JSON output and documented follow behavior.
- Expose follow or refresh behavior in the UI without uncontrolled polling.

## Workstream 5: Backup and Kubernetes completion

### Backup and restore

- Include project configuration and configured BPMN/DMN/form assets.
- Include AI secret values only after explicit opt-in; otherwise store secret references and metadata.
- Support documented output/project paths, force behavior, confirmations, and restrictive permissions.
- Provide browser download for backup archives and safe upload/restore with progress and result reporting.
- Verify a CLI and API/UI round trip.

### `camunda k8s`

- Apply context, namespace, release, and selector overrides consistently to status, logs, restart, and scale.
- Preserve confirmation gates for mutations.
- Expose status, logs, tail/follow, context, namespace, release, restart, and scale in the Project UI.
- Return clear tool-not-installed, context, authorization, resource-not-found, and Compose-environment errors.
- Helm upgrade, port-forward, and other explicitly deferred extensions remain out of scope.

## Workstream 6: CLI, API, UI, and documentation parity

- Every promised command flag has an equivalent API request field where the UI needs it.
- Every toolkit API route has argument validation, stable status codes, and a documented response shape.
- BPMN, Cluster, Project, and Home pages expose the completed workflows instead of placeholder output.
- Long or generated results can be copied or downloaded.
- Loading, empty, success, partial, and failure states are visible.
- Destructive actions require deliberate user interaction.
- Form controls remain accessible and visually clear in light and dark themes.
- CLI reference pages document every toolkit command and flag.
- Roadmap and architecture claims change to “shipped” only after acceptance succeeds.
- Historical implementation checklists are updated with evidence, not merely checked wholesale.

## Testing strategy

Implementation follows red-green-refactor for each behavior.

### Unit tests

- Domain success and failure paths.
- Malicious filesystem and archive inputs.
- BPMN parsing and normalized semantics.
- Auth/profile validation and canonical resource comparison.
- Stable formatters and exit-code mapping.

### Integration tests

- Cobra command execution for every advertised command and important flag.
- `httptest` coverage for every toolkit route.
- Origin, Host, CSRF, multipart, path, and symlink security cases.
- Fake OIDC and Camunda REST servers for authentication, inventory, incidents, and trace.
- Fake kubectl runner for all Kubernetes options and failures.
- Deterministic AI adapter tests with no paid calls.

### Browser tests

Use Chrome DevTools against the local development build to verify:

- Home deep diagnostics and endpoint cards.
- BPMN lint, diff, explain, review, test generation, and scan.
- Environment management, plan, drift, incidents, and trace.
- Backup download and restore upload.
- Kubernetes status/logs/options and mutation confirmations.
- Network status codes, payloads, console errors, focus behavior, accessibility, light/dark borders, and hard refresh.
- Cross-origin mutation rejection.

### Live acceptance

Run in isolated `CAMUNDA_LAB_HOME` and project directories:

1. Light profile: install/up, wait, smoke, UI, project and BPMN workflows.
2. Full profile: OIDC token acquisition, endpoint probes, incidents, trace, plan, and drift.
3. Kubernetes helpers: validate against an available safe context, or use deterministic integration tests when no disposable context exists.
4. Shut down and remove only resources created by the acceptance run.

No test may reuse, reset, or delete the user’s normal Camunda Lab home or unrelated Docker/Kubernetes resources.

## Delivery sequence and gates

Each workstream is a reviewable checkpoint on PR #16:

1. Safety foundation
2. Project/BPMN foundations
3. Developer tooling
4. Environments/cluster operations
5. Backup/Kubernetes
6. UI/docs parity
7. Automated and live acceptance

After every checkpoint:

- Focused tests pass.
- `go test ./...` passes.
- UI typecheck, lint, tests, and production build pass.
- `git diff --check` passes.
- No newly advertised behavior is left without acceptance coverage.

PR #16 is mergeable only after the final full-profile browser acceptance passes and all remaining deviations are either fixed or proven to be explicitly deferred by the source-of-truth rules above.

## Explicit exclusions

The following remain outside PR #16:

- Named side-by-side labs
- Windows support
- Process replay
- C7-to-C8 migration assistant
- Executive HTML reports
- Worker inspector and variable editor
- Visual BPMN canvas and auto-fix rewriting
- SARIF output where marked as later scope
- Generated-test execution where marked as later scope
- DMN/form semantic diff where marked as later scope
- Helm upgrade, port-forward, and other advanced Kubernetes management
- Volume export and remote deployed-resource backup
- Replacing official deployment, Helm, Operate, Tasklist, Identity, or Optimize tooling
