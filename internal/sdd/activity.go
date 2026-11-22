// Package sdd implements Feature 005's read-only next-step domain. It has no
// dependency on agent discovery, role selection, execution, or workflow
// transitions.
package sdd

// Activity is a canonical Spec-Driven Development activity.
type Activity string

const (
	ActivitySpecify   Activity = "specify"
	ActivityPlan      Activity = "plan"
	ActivityTasks     Activity = "tasks"
	ActivityImplement Activity = "implement"
	ActivityReview    Activity = "review"
)

// Canonical Spec Kit artifact names inspected by the Required Artifacts rule.
const (
	SpecArtifact  = "spec.md"
	PlanArtifact  = "plan.md"
	TasksArtifact = "tasks.md"
)
