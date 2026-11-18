package app

import (
	"flag"
	"fmt"
	"io"

	"ai-dev/internal/conventionalcommit"
	"ai-dev/internal/report"
)

const gitCommitValidateUsage = "Usage: ai-dev git commit validate (--message TEXT | --file FILE) [--format text|json]"

// RunGitCommitValidate implements "ai-dev git commit validate". It is pure:
// it runs no Git, reads no repository, strategy, authorization or audit
// state, and writes nothing but its result. A valid message exits 0, an
// invalid one 1, and a usage or output error 2.
func RunGitCommitValidate(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("git commit validate", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	message := flags.String("message", "", "")
	file := flags.String("file", "", "")
	format := flags.String("format", "text", "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || (*format != "text" && *format != "json") {
		fmt.Fprintln(stderr, gitCommitValidateUsage)
		return 2
	}
	given := map[string]bool{}
	flags.Visit(func(f *flag.Flag) { given[f.Name] = true })
	if given["message"] == given["file"] {
		fmt.Fprintln(stderr, gitCommitValidateUsage)
		return 2
	}
	data := []byte(*message)
	if given["file"] {
		var err error
		if data, err = readBoundedFile(*file, conventionalcommit.MaxMessageBytes+1); err != nil {
			fmt.Fprintf(stderr, "ai-dev: git commit validate: the message file cannot be read\n")
			return 2
		}
	}
	validation := conventionalcommit.Validate(data)
	var err error
	if *format == "json" {
		err = report.WriteConventionalCommitJSON(stdout, validation)
	} else {
		err = report.WriteConventionalCommitText(stdout, validation)
	}
	if err != nil {
		fmt.Fprintf(stderr, "ai-dev: git commit validate: output failed: %v\n", err)
		return 2
	}
	if !validation.Valid {
		return 1
	}
	return 0
}
