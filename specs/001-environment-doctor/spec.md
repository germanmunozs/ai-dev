# Feature Specification: Environment Doctor

**Feature Branch**: `001-environment-doctor`

**Created**: 2026-09-27

**Status**: Draft

**Input**: User description: "Create the first feature specification for the AI Dev Orchestrator. Feature: Environment Doctor. Goal: Provide a command named `ai-dev doctor` that determines whether the current development environment and repository are ready to use the AI Dev Orchestrator. The doctor must inspect and report the readiness of: Git; Spec Kit; Claude Code; Codex CLI; the current Git repository; Spec Kit initialization in the current repository; the project constitution; the installed Claude and Codex Spec Kit integrations. Requirements: every check has an explicit success or failure result; failure of one check must not prevent independent checks from running; every failure explains what is missing or invalid; the command produces an overall readiness result; results must be useful to a human in a terminal; results must also be suitable for future automation; define objective and verifiable acceptance criteria; include relevant failure and edge cases; do not implement anything. Keep the specification technology-agnostic. Do not select a programming language, CLI framework, serialization library, or terminal UI library. Do not invent authentication-detection behavior for Claude or Codex; if authentication readiness is ambiguous, identify it for clarification. Follow the project constitution."

## Clarifications

### Session 2026-09-27

- Q: Should each required coding-agent integration be an independent readiness check, making Claude and Codex separate checks and increasing the current total from eight to nine? → A: Yes; Claude and Codex are separate checks, for nine checks total.
- Q: Must the first version of `ai-dev doctor` provide an explicitly selectable machine-readable report containing stable check identifiers and results? → A: Yes; it must provide a selectable machine-readable report with stable check identifiers and results.
- Q: Should the Spec Kit initialization check evaluate only the core `.specify` scaffold, leaving all integration-record and integration-file validation exclusively to the separate Claude and Codex checks? → A: Yes; initialization checks only the core scaffold, and integration checks own all integration validation.
- Q: What must “intact” mean for each coding-agent integration check? → A: The installation record must be valid and every required file must exist and satisfy declared structural requirements; permitted customization does not fail.
- Q: Must every failed check include actionable remediation guidance in addition to explaining what is missing or invalid? → A: Yes; every failure includes both a diagnosis and a concise corrective action.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Check readiness before starting work (Priority: P1)

A developer who has just cloned or opened a repository wants to know, in one command, whether their machine and this repository are ready to use the AI Dev Orchestrator, before they try to run any orchestrator workflow.

**Why this priority**: This is the core value of the feature. Without a single trustworthy readiness check, developers discover missing prerequisites one broken step at a time, which wastes time and produces confusing downstream errors.

**Independent Test**: Can be fully tested by running the doctor command in a repository and environment with every prerequisite satisfied, and confirming it reports every check as passing and the overall result as ready.

**Acceptance Scenarios**:

1. **Given** a machine with Git, Spec Kit, Claude Code, and Codex CLI all installed and discoverable, and a repository with Spec Kit initialized, a completed project constitution, and both Spec Kit integrations correctly installed, **When** the developer runs the doctor command, **Then** every individual check is reported as passing and the overall result is reported as ready.
2. **Given** the same fully-ready environment and repository, **When** the developer runs the doctor command twice in a row without changing anything, **Then** both runs report identical per-check and overall results.

---

### User Story 2 - Diagnose and fix a specific missing prerequisite (Priority: P2)

A developer whose environment or repository is missing one or more prerequisites (for example, the project constitution has not been completed yet, or one Spec Kit integration is missing) runs the doctor command to find out exactly what is wrong and what to do about it.

**Why this priority**: Identifying *that* something is wrong is necessary but not sufficient; the feature only delivers value if the developer can act on the result without having to read source code or ask someone else.

**Independent Test**: Can be fully tested by removing or invalidating one prerequisite at a time (for example, deleting the constitution file) and confirming the doctor command reports that specific check as failing, explains what is missing or invalid, provides a concise corrective action, and still runs every other check.

**Acceptance Scenarios**:

1. **Given** a repository where the project constitution file is missing, **When** the developer runs the doctor command, **Then** the constitution check is reported as failing with an explanation that the constitution is missing and a concise corrective action, and every other check still runs and reports its own pass or fail result.
2. **Given** a repository where the Codex Spec Kit integration files are missing or incomplete, **When** the developer runs the doctor command, **Then** the Codex integration check is reported as failing with an explanation naming what is missing or invalid and a concise corrective action, and the Claude integration check still runs independently and reports its own result.
3. **Given** an environment where the Codex CLI tool itself is not installed or not discoverable, **When** the developer runs the doctor command, **Then** the Codex CLI check is reported as failing with an explanation and a concise corrective action, and the Git, Spec Kit, and Claude Code checks still run and report their own results.

---

### User Story 3 - Gate automated workflows on readiness (Priority: P3)

An automated workflow (for example, a script or a CI job) runs the doctor command before invoking any AI Dev Orchestrator workflow, and needs to reliably determine, without a human reading free text, whether it is safe to proceed.

**Why this priority**: This extends the value of User Story 1 to non-interactive contexts. It is lower priority than the human-facing scenarios because a human can always fall back to reading the terminal output, but automation cannot.

**Independent Test**: Can be fully tested by selecting the command's machine-readable report in both a fully-ready and a not-ready environment, and confirming a script can determine the overall result and each individual check's result from stable identifiers and explicit results, without parsing free-form prose.

**Acceptance Scenarios**:

1. **Given** a fully-ready environment and repository, **When** an automated caller runs the doctor command, **Then** the caller can determine, without human interpretation, that the overall result is ready and that every individual check passed.
2. **Given** an environment or repository with at least one failing check, **When** an automated caller runs the doctor command, **Then** the caller can determine, without human interpretation, that the overall result is not ready and which specific check(s) failed.
3. **Given** a fully-ready environment and repository, **When** an automated caller runs the doctor command, **Then** the process reports successful completion; **and given** any environment or repository that is not ready, **when** the caller runs the command, **then** the process reports unsuccessful completion.
4. **Given** any supported environment and repository state, **When** an automated caller explicitly selects the machine-readable report, **Then** the report identifies every check by a stable identifier and exposes each check result and the overall readiness result without requiring free-text parsing.

---

### Edge Cases

- What happens when the command is run outside any Git repository? The Git and Spec Kit tool checks (which depend only on the environment) still run and report their own results; every check that depends on being inside a repository (current Git repository, Spec Kit initialization, project constitution, installed integrations) fails with an explanation that no repository was found.
- What happens when the command is run inside a Git repository that has no commits yet? The current Git repository check passes, since it verifies that the working directory is a Git repository, not that it has history.
- What happens when the `.specify` scaffold exists but is incomplete (for example, missing its templates or scripts)? The Spec Kit initialization check fails and explains which required parts are missing.
- What happens when the project constitution file exists but still contains unfilled template placeholders, or is missing its version or ratification date? The constitution check fails and explains that the constitution has not been completed.
- What happens when a Spec Kit integration is recorded as installed for the repository, but one or more of its required files are missing or violate a declared structural requirement? The corresponding integration check fails and names the affected integration, file, and unmet requirement. A permitted customization that continues to satisfy the declared structural requirements does not cause failure.
- What happens when a required tool (Git, Spec Kit, Claude Code, or Codex CLI) is installed on the machine but not discoverable in the environment the command runs in (for example, not on the current `PATH`)? That check fails and explains that the tool could not be found, with the same treatment as if it were not installed.
- What happens when every check fails at once (for example, a completely empty directory with no repository and no tools installed)? All nine checks still run to completion, each reports its own failure with its own explanation, and the overall result is reported as not ready.
- What happens when the command is re-run after an earlier failure has been fixed? Only the checks affected by the fix change their result; unrelated checks continue to report the same result as before.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST provide a command, `ai-dev doctor`, that performs a full readiness assessment of the current development environment and the current repository with no required arguments.
- **FR-002**: The system MUST evaluate exactly the following nine checks on every invocation: (1) Git tool availability, (2) Spec Kit tool availability, (3) Claude Code tool availability, (4) Codex CLI tool availability, (5) current directory is a valid Git repository, (6) Spec Kit is initialized in the current repository, (7) the project constitution is present and complete, (8) the Claude Spec Kit integration installed in the current repository is complete and intact, and (9) the Codex Spec Kit integration installed in the current repository is complete and intact. Each additional coding-agent integration added to the readiness requirements in the future MUST be represented by its own independent check.
- **FR-003**: Each check MUST produce exactly one of two explicit results: Pass or Fail. No check may be left unreported, partially reported, or reported in an ambiguous state.
- **FR-004**: The failure of any one check MUST NOT prevent any other, independent check from running and reporting its own result in the same invocation.
- **FR-005**: Every check that results in Fail MUST include a human-readable explanation of specifically what is missing or invalid for that check and a concise corrective action the developer can take. The corrective action need not prescribe an exact command.
- **FR-006**: The system MUST produce a single overall readiness result for the invocation, which MUST be Ready if and only if all nine checks Pass, and MUST be Not Ready if any check Fails. The command process MUST report successful completion if and only if the overall result is Ready, and MUST report unsuccessful completion when the overall result is Not Ready. This specification does not assign implementation-specific numeric status codes.
- **FR-007**: The output MUST present, for a human reading it in a terminal, the result, explanation, and corrective action (when failed) of every individual check, plus the overall readiness result, without requiring the reader to consult external documentation or source code to understand what failed or what to do next.
- **FR-008**: The command MUST provide an explicitly selectable machine-readable report through which a program can reliably determine every individual check's result and the overall readiness result, independent of the exact wording of any human-readable explanation. Every check MUST have a stable identifier in this report. The specific serialization format and the mechanism used to select this report are not defined by this specification.
- **FR-009**: The Git tool check MUST Pass only when a Git client is installed and discoverable in the environment in which the command runs; otherwise it MUST Fail with an explanation that Git could not be found.
- **FR-010**: The Spec Kit tool check MUST Pass only when the Spec Kit tooling used to create and manage the `.specify` project scaffold is installed and discoverable in the environment in which the command runs; otherwise it MUST Fail with an explanation that it could not be found.
- **FR-011**: The Claude Code check MUST Pass only when the Claude Code tool is installed and discoverable in the environment in which the command runs; otherwise it MUST Fail with an explanation that it could not be found. Readiness for this check is scoped to installation/discoverability only; authentication status is out of scope for this feature (see Assumptions).
- **FR-012**: The Codex CLI check MUST use the same installation/discoverability-only scope defined for FR-011, applied to the Codex CLI tool; otherwise it MUST Fail with an explanation that it could not be found.
- **FR-013**: The current Git repository check MUST Pass only when the directory in which the command runs is recognized by Git as being inside a Git working tree, regardless of whether it has any commit history; otherwise it MUST Fail with an explanation that no Git repository was found.
- **FR-014**: The Spec Kit initialization check MUST Pass only when the current repository contains a complete core Spec Kit scaffold, including its governance memory, templates, and scripts; otherwise it MUST Fail with an explanation naming which required core part(s) are missing or invalid. This check MUST NOT validate coding-agent integration records or files, which are owned exclusively by the corresponding integration checks.
- **FR-015**: The project constitution check MUST Pass only when the repository's constitution file exists, is non-empty, contains no unresolved template placeholders, and declares both a version and a ratification date; otherwise it MUST Fail with an explanation of what is missing or unfilled.
- **FR-016**: The Claude integration check and the Codex integration check MUST each independently evaluate whether that integration has a valid installation record and whether every required file identified by that record exists and satisfies the integration's declared structural requirements. Each check MUST Pass only when its own integration satisfies those conditions and otherwise MUST Fail with an explanation naming the missing or invalid record, affected file(s), and unmet requirement(s). Permitted file customizations MUST NOT cause failure when the declared structural requirements remain satisfied; exact byte-for-byte equality with originally installed content is not required.
- **FR-017**: The system MUST run and report a result for every check listed in FR-002 even when the command is invoked outside a Git repository or outside a Spec Kit-initialized project; checks that depend on repository state that does not exist MUST Fail explicitly rather than the command aborting without a result.
- **FR-018**: Given no change to the environment or repository between invocations, the system MUST produce the same per-check and overall results on repeated invocations.

### Key Entities

- **Check**: A single, independently-executable readiness inspection (one of the nine listed in FR-002). Has a stable identifier, an identifying name, the area it inspects, a result of Pass or Fail, and an explanation plus concise corrective action whenever the result is Fail. Each coding-agent integration has its own Check.
- **Readiness Report**: The complete outcome of one `ai-dev doctor` invocation. Consists of the result of every Check plus one overall readiness result (Ready or Not Ready). It has a human-readable terminal presentation and an explicitly selectable machine-readable presentation in which each Check has a stable identifier.
- **Spec Kit Integration Record**: The repository's own record of which coding-agent integration is installed, which files that integration requires, and the structural requirements those files must satisfy. Each integration check uses its corresponding record; the record does not require exact original file content when customization is permitted.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A developer can determine complete environment-and-repository readiness for the AI Dev Orchestrator with a single command invocation, with no need to consult separate documentation to interpret the result.
- **SC-002**: 100% of the nine defined checks report an explicit Pass or Fail on every invocation, with zero invocations producing a missing, blank, or ambiguous result for any check.
- **SC-003**: When one or more checks fail, 100% of the remaining, independent checks still complete and report their own result in that same invocation.
- **SC-004**: For every Fail result produced across representative failure scenarios (one per check, and all checks failing at once), a person unfamiliar with the implementation can state what is missing or invalid and identify a corrective action using only the reported result.
- **SC-005**: Across representative fixture scenarios (all checks satisfied, each single check failing in isolation, and all checks failing), the overall readiness result matches the expected Ready/Not Ready outcome in 100% of scenarios.
- **SC-006**: An automated caller can determine the overall readiness result and every individual check's result programmatically, without a human reading or interpreting free-form text.
- **SC-007**: Across representative Ready and Not Ready scenarios, an automated caller observes successful process completion in 100% of Ready scenarios and unsuccessful process completion in 100% of Not Ready scenarios.

## Assumptions

- "Installed and discoverable" means the tool can be located and invoked from the standard executable-lookup mechanism of the environment in which `ai-dev doctor` runs; a tool present on disk but not reachable that way is treated the same as not installed.
- Verifying the specific version of an installed tool, or its compatibility with the Spec Kit version recorded in this repository, is out of scope for this feature; checks verify presence/discoverability and, for repository artifacts, structural completeness, not version compatibility.
- The current Git repository check only verifies that the working directory is inside a Git working tree; it does not require any commits, branches, or a configured remote to exist.
- Both the Claude and Codex Spec Kit integrations are expected to be installed and intact for a repository to be considered ready, consistent with this project's constitution, which anticipates cross-agent review between an implementing agent and a differing reviewing agent (Core Principle VIII, AI-Assisted Engineering).
- The project constitution check validates structural completeness (file exists, is non-empty, has no unresolved template placeholders, and declares a version and ratification date), not the substantive quality or correctness of its content.
- This specification defines the checks, their pass/fail semantics, and the required qualities of the output; it intentionally leaves the choice of programming language, CLI framework, output/serialization format, and terminal UI presentation to the implementation plan.
- The Claude Code and Codex CLI checks verify installation/discoverability only. Whether either tool is authenticated with its AI service is out of scope for this feature; a Pass result means the tool was found, not that it is logged in or able to complete a request.
