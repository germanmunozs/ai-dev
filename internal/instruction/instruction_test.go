package instruction

import (
	"errors"
	"io"
	"strings"
	"testing"
)

type failReader struct {
	t *testing.T
}

func (r failReader) Read([]byte) (int, error) {
	r.t.Fatal("stdin was read")
	return 0, io.EOF
}

func TestResolve(t *testing.T) {
	tests := []struct {
		name        string
		arg         string
		hasArg      bool
		stdin       io.Reader
		interactive bool
		want        string
		wantErr     error
	}{
		{
			name:        "argument only",
			arg:         "explain `go test`\nwithout changing it",
			hasArg:      true,
			stdin:       strings.NewReader(""),
			interactive: false,
			want:        "explain `go test`\nwithout changing it",
		},
		{
			name:        "argument with interactive stdin does not read",
			arg:         "keep exact spacing  ",
			hasArg:      true,
			stdin:       failReader{t: t},
			interactive: true,
			want:        "keep exact spacing  ",
		},
		{
			name:        "non-interactive stdin through EOF",
			stdin:       strings.NewReader("line one\nline two\n"),
			interactive: false,
			want:        "line one\nline two\n",
		},
		{
			name:        "missing from empty stdin",
			stdin:       strings.NewReader(""),
			interactive: false,
			wantErr:     ErrInstructionRequired,
		},
		{
			name:        "missing from whitespace argument",
			arg:         " \t\n ",
			hasArg:      true,
			stdin:       strings.NewReader(""),
			interactive: false,
			wantErr:     ErrInstructionRequired,
		},
		{
			name:        "missing from whitespace stdin",
			stdin:       strings.NewReader(" \t\n "),
			interactive: false,
			wantErr:     ErrInstructionRequired,
		},
		{
			name:        "interactive stdin fails immediately",
			stdin:       failReader{t: t},
			interactive: true,
			wantErr:     ErrInstructionRequired,
		},
		{
			name:        "argument and stdin are ambiguous",
			arg:         "argument instruction",
			hasArg:      true,
			stdin:       strings.NewReader("stdin instruction\n"),
			interactive: false,
			wantErr:     ErrAmbiguousInstruction,
		},
		{
			name:        "whitespace stdin does not conflict with argument",
			arg:         "argument instruction",
			hasArg:      true,
			stdin:       strings.NewReader(" \n\t"),
			interactive: false,
			want:        "argument instruction",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(tt.arg, tt.hasArg, tt.stdin, tt.interactive)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Resolve() error = %v, want %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("Resolve() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolvePropagatesReadFailure(t *testing.T) {
	want := errors.New("read failed")
	_, err := Resolve("", false, errorReader{err: want}, false)
	if !errors.Is(err, want) {
		t.Fatalf("Resolve() error = %v, want wrapped %v", err, want)
	}
}

type errorReader struct {
	err error
}

func (r errorReader) Read([]byte) (int, error) {
	return 0, r.err
}
