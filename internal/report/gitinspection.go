package report

import (
	"fmt"
	"io"
	"strings"

	"ai-dev/internal/gitmanager"
)

// WriteGitInspectionJSON writes exactly one GitInspectOutput object. The
// inspection is validated and redacted before anything is written.
func WriteGitInspectionJSON(w io.Writer, inspection gitmanager.GitInspection) error {
	data, err := gitmanager.EncodeInspectOutput(inspection)
	if err != nil {
		return err
	}
	_, err = w.Write(append(data, '\n'))
	return err
}

// WriteGitInspectionText writes one stable, human-readable block rendered
// from the same validated model. It claims no authorization.
func WriteGitInspectionText(w io.Writer, inspection gitmanager.GitInspection) error {
	if err := inspection.Validate(); err != nil {
		return err
	}
	o := inspection.Observation
	var b strings.Builder
	line := func(label, format string, args ...any) {
		fmt.Fprintf(&b, "%-15s %s\n", label+":", fmt.Sprintf(format, args...))
	}
	identity := inspection.ProjectIdentity
	path := identity.PhysicalPathUTF8()
	if path == "" {
		path = "base64url:" + gitmanager.EncodeReversiblePath(identity.ReversiblePathBytes())
	}
	line("project", "%s", path)
	line("identity", "device %s, inode %s, %s", identity.DeviceDecimal(), identity.InodeDecimal(), identity.Digest())
	line("repository", "%s", o.RepositoryKind)
	head := string(o.Head.State)
	if !o.Head.SymbolicRef.IsZero() {
		head += " " + displayRef(o.Head.SymbolicRef)
	}
	if o.Head.CommitOID != "" {
		head += " " + string(o.Head.CommitOID)
	}
	line("head", "%s (history: %t)", head, o.Head.HistoryPresent)
	line("refs", "%d (%s)", len(o.Refs()), o.RefsObservationState)
	line("index", "%s", o.Index.Presence)
	line("worktree", "%d changes (%s)", len(o.WorktreeChanges()), o.WorktreeObservationState)
	line("sequencer", "%s", o.SequencerState)
	line("configuration", "%d entries (%s)", len(o.EffectiveConfiguration()), o.ConfigurationObservationState)
	line("git flow", "%s", o.GitFlowEvidence.Classification)
	line("conversions", "%s", o.ConversionSettings.Risk)
	line("project config", "%s", o.ProjectConfig.State)
	for _, profile := range o.FilesystemCapabilities() {
		line("filesystem", "%s %s (production supported: %t)", profile.Role, profile.FilesystemType, profile.ProductionSupported)
	}
	for _, remote := range inspection.RemotesRedacted() {
		for _, url := range remote.FetchURLs() {
			line("remote", "%s %s", remote.Name, url.DisplayRedacted)
		}
	}
	tool := inspection.ToolIdentity
	line("git", "%s (%s)", tool.Version, tool.InvocationMode)
	for _, warning := range inspection.Warnings() {
		line("warning", "%s", warning)
	}
	for _, incompatibility := range inspection.Incompatibilities() {
		if incompatibility.Subject != "" {
			line("incompatible", "%s: %s", incompatibility.Reason, incompatibility.Subject)
		} else {
			line("incompatible", "%s", incompatibility.Reason)
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func displayRef(ref gitmanager.ObservedRef) string {
	if ref.DisplayUTF8 != "" {
		return ref.DisplayUTF8
	}
	return "base64url:" + ref.ReversibleBytesBase64URL
}
