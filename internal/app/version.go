package app

import (
	"fmt"
	"io"

	"github.com/germanmunozs/ai-dev/internal/report"
	"github.com/germanmunozs/ai-dev/internal/version"
)

const versionUsage = "Usage: ai-dev version [--format text|json]"

// RunVersion reports which build of ai-dev is running (F009-FR-001). It
// reads only the build information embedded in the binary.
func RunVersion(args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("version")
	format := flags.String("format", "text", "output format: text or json")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || (*format != "text" && *format != "json") {
		fmt.Fprintln(stderr, versionUsage)
		return 2
	}
	info := version.Read()
	var err error
	if *format == "json" {
		err = report.WriteVersionJSON(stdout, info)
	} else {
		err = report.WriteVersionText(stdout, info)
	}
	if err != nil {
		fmt.Fprintf(stderr, "write version report: %v\n", err)
		return 2
	}
	return 0
}
