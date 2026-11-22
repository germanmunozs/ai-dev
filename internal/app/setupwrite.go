package app

import (
	"io/fs"

	"github.com/germanmunozs/ai-dev/internal/projectconfig"
	"github.com/germanmunozs/ai-dev/internal/report"
)

// ConfigStore writes the Feature 011 project configuration atomically.
// cmd/ai-dev supplies internal/configstore; this package never imports it,
// so configstore stays the sole write path (F011 R-03).
type ConfigStore interface {
	Write(data []byte) (written bool, warnings []string)
}

const configStoreUnavailable = "ai-dev: internal error: project configuration store unavailable"

// decideWrite compares the requested configuration with the one on disk and
// writes only when needed: an equal configuration is unchanged whatever its
// formatting, and an existing different or invalid one is replaced only with
// replace (F011-FR-011). The setup assistant passes replace after the
// explicit confirmation.
func decideWrite(root fs.FS, store ConfigStore, requested projectconfig.Config, replace bool) (report.SetupOutcome, []string) {
	existing, state, _ := projectconfig.Load(root)
	if state == projectconfig.Valid && existing.Equal(requested) {
		return report.ConfigurationUnchanged, nil
	}
	if state != projectconfig.Absent && !replace {
		return report.ConfigurationExists, nil
	}
	if store == nil {
		return report.PersistenceFailed, []string{configStoreUnavailable}
	}
	written, warnings := store.Write(requested.Encode())
	if !written {
		return report.PersistenceFailed, warnings
	}
	return report.ConfigurationWritten, warnings
}
