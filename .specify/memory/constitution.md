<!--
Sync Impact Report
- Version change: 1.0.0 -> 1.1.0
- Modified principles:
  - I. Specification First -> I. Specification First
  - III. Simplicity and YAGNI -> III. Simplicity and YAGNI
  - VIII. AI-Assisted Engineering -> VIII. AI-Assisted Engineering
  - IX. Human Authority -> IX. Human Authority
- Modified sections: Development Workflow and Quality Gates; Governance
- Added sections: Operational Definitions
- Removed sections: none
- Follow-up TODOs: none
-->
# Agent Lab Constitution

## Core Principles

### I. Specification First

Every significant feature MUST proceed through Specification -> Plan -> Tasks ->
Implementation. A specification MUST define the intended outcomes, constraints, and verifiable
acceptance criteria before implementation begins. If a requirement is ambiguous in a way that
could materially change the solution, implementation MUST pause until the ambiguity is resolved
or an explicit, documented assumption receives human approval. Small, low-risk maintenance
changes MAY use abbreviated artifacts, but their intent and validation criteria MUST remain clear.
This sequence keeps implementation aligned with an agreed problem rather than an inferred one.

### II. Architecture Before Framework

Architecture decisions MUST follow from domain needs, quality attributes, operational context,
and measurable system constraints. A preferred framework, cloud, platform, or infrastructure
tool MUST NOT dictate system boundaries or design without supporting requirements. Plans MUST
record consequential architecture choices, the requirements they satisfy, and relevant
trade-offs. This repository is technology-agnostic so that experiments evaluate tools against
the problem instead of reshaping the problem around a tool.

### III. Simplicity and YAGNI

Solutions MUST use the simplest design that satisfies the current requirements and acceptance
criteria. Microservices, Kubernetes, event-driven architecture, domain-driven design, CQRS,
additional abstraction layers, and new infrastructure MUST NOT be introduced unless a stated
requirement or measurable technical constraint justifies them. Any such complexity MUST be
documented in the plan with its benefit, cost, trade-offs, and a simpler alternative considered.
An explicit learning or technology-evaluation objective in a specification is valid justification
when the plan documents that objective, its cost and trade-offs, and the simpler alternative.
Speculative extensibility without such an objective is not sufficient justification.

### IV. Testable by Design

Every feature specification MUST include objective, verifiable acceptance criteria.
Implementations MUST include automated tests appropriate to their risk and scope, covering the
primary behavior and important failure scenarios. Plans MUST identify the test level and
strategy when unit tests alone cannot verify system behavior. A change is not complete when its
acceptance criteria cannot be reproducibly demonstrated.

### V. Security by Design

Secrets, credentials, private keys, and sensitive tokens MUST never be committed. Designs and
implementations MUST apply least privilege, validate inputs at trust boundaries, maintain
dependency hygiene, and choose secure defaults. Security-sensitive changes MUST identify their
threats, data exposure, and required human approval. Known security risks MUST be resolved or
receive human approval for explicit acceptance before merge.

### VI. Observability by Design

Backend and distributed-system specifications and plans MUST consider the logs, metrics, and
traces needed to operate and diagnose the system. Instrumentation MUST be proportional to the
experiment's operational needs and MUST avoid exposing secrets or sensitive data. When
observability is required, OpenTelemetry SHOULD be preferred as the vendor-neutral
instrumentation standard; deviations MUST state the interoperability or operational reason.

### VII. Reproducibility and Automation

Build, test, lint, and local execution workflows MUST be reproducible through version-controlled
commands and configuration. Required tools and versions MUST be declared or discoverable from
repository documentation and configuration. Manual steps that affect correctness MUST be
documented and SHOULD be automated when they recur. Validation MUST be runnable without relying
on undocumented local state.

### VIII. AI-Assisted Engineering

AI coding agents, regardless of vendor or product, MUST operate from the same version-controlled
specifications, plans, tasks, and engineering artifacts used by humans. AI-generated code MUST
meet the same review, testing, security, documentation, and reproducibility standards as
human-written code. AI involvement does not reduce accountability. When cross-agent review is
required, the plan, review artifact, or pull request MUST record the implementing agent,
reviewing agent, findings, and resolution. The final reviewing agent SHOULD differ from the
implementing agent; if that separation is not possible, the same record MUST state the reason.

### IX. Human Authority

Humans retain final authority over architecture decisions, exceptions to this constitution,
security-sensitive changes, production changes, and final merge decisions. Agents MAY analyze,
recommend, and implement within delegated scope, but MUST surface material assumptions, risks,
and irreversible actions for human decision. An AI coding agent MUST NOT create, fabricate, or
assert human approval on a human's behalf. No automated approval or generated artifact may
override an explicit human decision within that authority.

## Operational Definitions

- **Significant work**: Work is significant when it introduces or changes externally visible
  behavior, architecture or system boundaries, data models or migrations, security or privacy
  posture, production or infrastructure behavior, operational or cost exposure, or multiple
  coordinated components. Low-risk maintenance that changes none of these MAY be treated as
  non-significant.
- **Material**: An assumption, risk, decision, or change is material when it could alter acceptance
  criteria, architecture, security or privacy posture, production behavior, compatibility,
  operational or cost exposure, or the decision a human authority would make.
- **Human approval**: Every human approval, authorization, confirmation, or acceptance required by
  this constitution MUST identify the human approver and decision and MUST be recorded in a
  version-controlled artifact or an auditable review system, such as a pull request.
  Agent-generated statements alone do not constitute human approval.

## Engineering Constraints

- The repository MUST remain open to backend, cloud, mobile, GitOps, observability, and
  AI-assisted engineering experiments without imposing a universal technology stack.
- Dependencies, platforms, and architectural patterns MUST be selected per experiment and tied
  to documented requirements or constraints.
- Experiments MUST isolate or document external side effects, privileged access, cost exposure,
  and production impact before execution.

## Development Workflow and Quality Gates

Significant work MUST produce and keep aligned a specification, implementation plan, ordered
task list, and implementation. Reviews MUST verify constitution compliance, acceptance-criteria
coverage, appropriate automated tests, security impact, operational visibility when applicable,
and reproducible commands. Deviations discovered during implementation MUST be reflected in the
governing artifacts before the work is considered complete.

Every implementation plan MUST declare the applicable executable or otherwise objectively
verifiable quality gates, including test, lint, build, security, and other validation checks
relevant to the work. For each gate, the plan MUST state how it is run or evidenced and its pass
condition, or state why that gate category is not applicable. Work is complete only when all
applicable declared gates pass or each failure has an explicit exception with human approval.

Before final merge, a human MUST confirm that required checks pass and that material architecture,
security, production, and exception decisions are explicit. Production-affecting actions require
separate human authorization and MUST NOT be inferred from approval to write or review code. A
rollback or recovery approach MUST be documented before that human authorization is requested.

## Governance

This constitution is the repository's highest-level engineering policy. Specifications,
implementation plans, task lists, reviews, and implementation work MUST comply with it. When
another repository artifact conflicts with this constitution, this constitution governs unless
a documented exception has received human approval under the process defined here.

Amendments MUST be proposed as version-controlled changes that document the motivation, affected
principles or sections, compatibility impact, and any required migration. A human MUST approve
every amendment. Constitution versions follow semantic versioning: MAJOR for incompatible
governance changes or principle removals or redefinitions, MINOR for new principles or materially
expanded obligations, and PATCH for clarifications that do not change obligations. Ratification
records the original adoption date; Last Amended changes whenever governance content changes.

Every specification and plan review MUST include a constitution compliance check. Every final
review MUST verify applicable quality gates and record justified exceptions. Approved exceptions
MUST identify the affected principle, scope, rationale, risks, approving human, and expiration or
review condition; MUST be explicit and narrowly scoped; and MUST be retained with the affected
artifact. Ongoing experiments SHOULD review compliance when their scope or risk materially
changes.

**Version**: 1.1.0 | **Ratified**: 2026-09-26 | **Last Amended**: 2026-09-26
