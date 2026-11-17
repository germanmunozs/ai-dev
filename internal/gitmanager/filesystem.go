package gitmanager

import (
	"fmt"
	"strings"
)

// FilesystemProfileVersion names the capability-profile rules below.
const FilesystemProfileVersion = "ai-dev.git-filesystem/v1"

// FilesystemObservation is what the platform adapter observes about the
// filesystem holding one location, before any capability decision.
type FilesystemObservation struct {
	GOOS           string
	FilesystemType string
	MountIdentity  string
	DeviceDecimal  string
	InodeDecimal   string
	MountFlags     []string
	Local          bool
	ReadOnly       bool
}

// FilesystemPrimitives records which durability and exclusion primitives
// were proven for a location. EvidenceDigest identifies the probe that
// proved them; it is empty when they come from certification instead.
type FilesystemPrimitives struct {
	ExclusiveCreate            bool
	AdvisoryLock               bool
	FileSync                   bool
	DirectorySync              bool
	AtomicSameFilesystemRename bool
	AtomicReplace              bool
	StableIdentity             bool
	EvidenceDigest             ContentDigest
}

func (p FilesystemPrimitives) complete() bool {
	return p.ExclusiveCreate && p.AdvisoryLock && p.FileSync && p.DirectorySync &&
		p.AtomicSameFilesystemRename && p.AtomicReplace && p.StableIdentity
}

// productionFilesystems are the only production profiles: local APFS on
// macOS and local ext4 on Linux (plan m-07).
var productionFilesystems = map[string]string{"darwin": "apfs", "linux": "ext4"}

func productionType(obs FilesystemObservation) bool {
	return productionFilesystems[obs.GOOS] == obs.FilesystemType && obs.Local && !obs.ReadOnly
}

// TestOnlyFilesystem reports a tmpfs location: usable by explicitly
// labeled non-durability fixtures, never production, never native evidence.
func TestOnlyFilesystem(obs FilesystemObservation) bool { return obs.FilesystemType == "tmpfs" }

// CertifiedPrimitives returns the primitives a production filesystem is
// certified to provide. Project-side locations use it because inspect and
// plan never write a probe into the project; apply still verifies every
// actual operation. Anything other than a production filesystem gets none.
func CertifiedPrimitives(obs FilesystemObservation) FilesystemPrimitives {
	if !productionType(obs) {
		return FilesystemPrimitives{}
	}
	return FilesystemPrimitives{ExclusiveCreate: true, AdvisoryLock: true, FileSync: true, DirectorySync: true,
		AtomicSameFilesystemRename: true, AtomicReplace: true, StableIdentity: true}
}

// ClassifyFilesystem builds the capability profile of one location. It is
// production-supported only for a production filesystem type whose every
// required primitive is proven or certified.
func ClassifyFilesystem(role FilesystemRole, obs FilesystemObservation, primitives FilesystemPrimitives) (FilesystemCapabilityProfile, error) {
	if primitives.EvidenceDigest != "" {
		if err := primitives.EvidenceDigest.Validate(); err != nil {
			return FilesystemCapabilityProfile{}, err
		}
	}
	return NewFilesystemCapabilityProfile(FilesystemCapabilityProfileInput{
		Role:                       role,
		GOOS:                       obs.GOOS,
		FilesystemType:             obs.FilesystemType,
		MountIdentity:              obs.MountIdentity,
		DeviceDecimal:              obs.DeviceDecimal,
		InodeDecimal:               obs.InodeDecimal,
		MountFlags:                 obs.MountFlags,
		Local:                      obs.Local,
		ReadOnly:                   obs.ReadOnly,
		ProfileVersion:             FilesystemProfileVersion,
		ExclusiveCreate:            primitives.ExclusiveCreate,
		AdvisoryLock:               primitives.AdvisoryLock,
		FileSync:                   primitives.FileSync,
		DirectorySync:              primitives.DirectorySync,
		AtomicSameFilesystemRename: primitives.AtomicSameFilesystemRename,
		AtomicReplace:              primitives.AtomicReplace,
		StableIdentity:             primitives.StableIdentity,
		ProductionSupported:        productionType(obs) && primitives.complete(),
		ProbeEvidenceDigest:        primitives.EvidenceDigest,
	})
}

// FilesystemError reports a location that cannot hold production plan or
// apply state.
type FilesystemError struct {
	Reason  Reason
	Role    FilesystemRole
	Profile string
}

func (e *FilesystemError) Error() string {
	return fmt.Sprintf("%s location on %s is not a supported filesystem (%s)", e.Role, e.Profile, e.Reason)
}

// RequireProductionFilesystem refuses any profile that is not production
// supported with filesystem_unsupported.
func RequireProductionFilesystem(profile FilesystemCapabilityProfile) error {
	if profile.ProductionSupported {
		return nil
	}
	flags := strings.Join(profile.MountFlags(), ",")
	return &FilesystemError{Reason: ReasonFilesystemUnsupported, Role: profile.Role,
		Profile: fmt.Sprintf("%s/%s local=%t read_only=%t flags=[%s]", profile.GOOS, profile.FilesystemType, profile.Local, profile.ReadOnly, flags)}
}
