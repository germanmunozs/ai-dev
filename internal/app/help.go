package app

import (
	"fmt"
	"io"
	"slices"
	"strings"
)

const helpUsage = "Usage: ai-dev help [command...]"

// RunHelp writes the general help, or the help of the catalog path args
// names, to standard output (F009-FR-003, F010-FR-005).
func RunHelp(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return writeHelp(generalHelp(), stdout, stderr)
	}
	spec, consumed, ok := lookup(args)
	if !ok || consumed != len(args) {
		fmt.Fprintln(stderr, helpUsage)
		return 2
	}
	return writeHelp(pathHelp(spec), stdout, stderr)
}

// helpRequest reports the catalog path whose help args ask for: --help or
// -h right after a complete path, and nowhere else (F010-FR-003).
func helpRequest(args []string) (commandSpec, bool) {
	spec, consumed, ok := lookup(args)
	if !ok || consumed >= len(args) || (args[consumed] != "--help" && args[consumed] != "-h") {
		return commandSpec{}, false
	}
	return spec, true
}

func writeHelp(text string, stdout, stderr io.Writer) int {
	if _, err := io.WriteString(stdout, text); err != nil {
		fmt.Fprintf(stderr, "write help: %v\n", err)
		return 2
	}
	return 0
}

func generalHelp() string {
	width := 0
	for _, command := range catalog {
		width = max(width, len(strings.Join(command.path, " ")))
	}
	var out strings.Builder
	out.WriteString("ai-dev: spec-driven development with coding agents, under explicit human authority.\n\n")
	out.WriteString("Usage: ai-dev <command> [options]\n\nCommands:\n")
	for _, command := range catalog {
		if command.usage != "" {
			fmt.Fprintf(&out, "  %-*s  %s\n", width, strings.Join(command.path, " "), command.description)
		}
	}
	out.WriteString("\nRun 'ai-dev help <command>' or 'ai-dev <command> --help' for a command's options.\n")
	out.WriteString("Most commands accept --format text|json.\n")
	out.WriteString("Exit codes: 0 positive result, 1 negative domain result, 2 usage or internal failure.\n")
	out.WriteString("Nothing is approved, transitioned or committed without an explicit human decision.\n")
	return out.String()
}

// pathHelp renders a command's help, a group's help, or both for init
// (contracts/help.md).
func pathHelp(spec commandSpec) string {
	var out strings.Builder
	fmt.Fprintf(&out, "ai-dev %s: %s\n", strings.Join(spec.path, " "), spec.description)
	if spec.usage != "" {
		writeCommandHelp(&out, spec)
	}
	if subcommands := children(spec.path); len(subcommands) > 0 {
		writeGroupHelp(&out, subcommands)
	}
	return out.String()
}

func writeCommandHelp(out *strings.Builder, spec commandSpec) {
	fmt.Fprintf(out, "\n%s\n\n", spec.usage)
	type row struct{ label, text string }
	var arguments, options []row
	for _, arg := range spec.args {
		arguments = append(arguments, row{"<" + arg.name + ">", arg.description + valuesNote(arg.values)})
	}
	for _, option := range spec.options {
		label := "--" + option.name
		if option.value != "" {
			label += " " + option.value
		}
		text := option.description + valuesNote(option.values)
		if option.valuesHelp != "" {
			text = option.description + " [values: " + option.valuesHelp + "]"
		}
		if option.repeatable {
			text += " [repeatable]"
		}
		options = append(options, row{label, text})
	}
	options = append(options, row{"-h, --help", "Show this help"})
	width := 0
	for _, r := range slices.Concat(arguments, options) {
		width = max(width, len(r.label))
	}
	section := func(title string, rows []row) {
		if len(rows) == 0 {
			return
		}
		out.WriteString(title + ":\n")
		for _, r := range rows {
			fmt.Fprintf(out, "  %-*s  %s\n", width, r.label, r.text)
		}
	}
	section("Arguments", arguments)
	section("Options", options)
	fmt.Fprintf(out, "Changes: %s\nExit codes: %s\n", spec.changes, spec.exitCodes)
}

func writeGroupHelp(out *strings.Builder, subcommands []commandSpec) {
	width := 0
	for _, child := range subcommands {
		width = max(width, len(strings.Join(child.path, " ")))
	}
	out.WriteString("\nCommands:\n")
	for _, child := range subcommands {
		fmt.Fprintf(out, "  %-*s  %s\n", width, strings.Join(child.path, " "), child.description)
	}
	out.WriteString("\nRun 'ai-dev help <command>' for a command's options.\n")
}

func valuesNote(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return " [values: " + strings.Join(values, "|") + "]"
}
