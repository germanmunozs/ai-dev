package report

import (
	"bytes"
	"testing"

	"ai-dev/internal/version"
)

func TestWriteVersion(t *testing.T) {
	full := version.Info{Version: "v0.1.0", Commit: "d4c71b5e0a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d", CommitTime: "2026-11-20T16:07:05Z", Modified: true, GoVersion: "go1.27.1", Platform: "darwin/arm64"}
	bare := version.Info{Version: "unknown", GoVersion: "go1.27.1", Platform: "linux/amd64"}
	cases := []struct {
		name, text, json string
		info             version.Info
	}{
		{"full", "ai-dev v0.1.0 (commit d4c71b5e0a6b, modified, go1.27.1, darwin/arm64)\n",
			`{"schema_version":"ai-dev.version/v1","version":"v0.1.0","commit":"d4c71b5e0a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d","commit_time":"2026-11-20T16:07:05Z","modified":true,"go_version":"go1.27.1","platform":"darwin/arm64"}` + "\n", full},
		{"no VCS information", "ai-dev unknown (go1.27.1, linux/amd64)\n",
			`{"schema_version":"ai-dev.version/v1","version":"unknown","go_version":"go1.27.1","platform":"linux/amd64"}` + "\n", bare},
		{"short commit, clean tree", "ai-dev (devel) (commit abc, go1.27.1, linux/amd64)\n",
			`{"schema_version":"ai-dev.version/v1","version":"(devel)","commit":"abc","go_version":"go1.27.1","platform":"linux/amd64"}` + "\n",
			version.Info{Version: "(devel)", Commit: "abc", GoVersion: "go1.27.1", Platform: "linux/amd64"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var text, wire bytes.Buffer
			if err := WriteVersionText(&text, tc.info); err != nil {
				t.Fatal(err)
			}
			if err := WriteVersionJSON(&wire, tc.info); err != nil {
				t.Fatal(err)
			}
			if text.String() != tc.text || wire.String() != tc.json {
				t.Fatalf("text = %q, want %q\njson = %s, want %s", text.String(), tc.text, wire.String(), tc.json)
			}
		})
	}
}
