package gitmanager

import (
	"context"
	"io/fs"
	"time"
)

// GitRunner is the consumer-owned boundary for one already validated Git
// invocation. Building the closed operation matrix and executing Git belong to
// later phases; this port grants neither capability by itself.
type GitRunner interface {
	Run(context.Context, GitInvocation) (GitRunResult, error)
}

// GitInvocation carries exact process inputs. Args never includes the
// executable, and no field contains a shell command string.
type GitInvocation struct {
	Executable  string
	Args        []string
	Directory   string
	Environment []string
	Stdin       []byte
	StdoutLimit int64
	StderrLimit int64
}

// GitRunResult contains bounded bytes captured from one Git invocation.
type GitRunResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// GitStore is the consumer-owned persistence boundary. Operation meanings and
// the durable state machine are deliberately deferred to their authorized
// implementation phases.
type GitStore interface {
	Execute(context.Context, GitStoreRequest) (GitStoreResult, error)
}

// GitStoreRequest is a typed transport envelope for a manager-owned store
// operation. Payloads are copied by test doubles so callers cannot mutate
// recorded evidence after a call.
type GitStoreRequest struct {
	Operation string
	Key       string
	Version   string
	Payload   []byte
}

// GitStoreResult is the transport result of a GitStore request.
type GitStoreResult struct {
	Payload []byte
	Version string
	Applied bool
}

// Filesystem is the consumer-owned boundary for descriptor-safe filesystem
// work. The closed operation set and platform implementations are deferred to
// later Feature 008 phases.
type Filesystem interface {
	Execute(context.Context, FilesystemRequest) (FilesystemResult, error)
}

// FilesystemRequest describes one manager-owned filesystem request without
// exposing an operating-system handle across the port.
type FilesystemRequest struct {
	Operation string
	Path      string
	Data      []byte
	Mode      fs.FileMode
	Limit     int64
}

// FilesystemResult contains the bounded data and identity observations needed
// by the manager.
type FilesystemResult struct {
	Data          []byte
	Mode          fs.FileMode
	DeviceDecimal string
	InodeDecimal  string
	Applied       bool
}

// Clock supplies time without consulting ambient time from domain code.
type Clock interface {
	Now() time.Time
}

// RandomSource supplies cryptographic random bytes through an injectable
// io.Reader-compatible boundary.
type RandomSource interface {
	Read([]byte) (int, error)
}

// Reporter is the consumer-owned output boundary. Rendering and delivery are
// separate from Git execution and storage.
type Reporter interface {
	Report(context.Context, ReportRequest) error
}

// ReportRequest carries one already validated rendering request.
type ReportRequest struct {
	Kind    string
	Format  string
	Payload []byte
}
