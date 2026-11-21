package app

import (
	"fmt"
	"io"
	"strconv"
	"strings"
)

const completionUsage = "Usage: ai-dev completion bash|zsh"

// RunCompletion writes the static completion script for one shell
// (F010-FR-006).
func RunCompletion(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 || (args[0] != "bash" && args[0] != "zsh") {
		fmt.Fprintln(stderr, completionUsage)
		return 2
	}
	script := completionScript(args[0])
	if _, err := io.WriteString(stdout, script); err != nil {
		fmt.Fprintf(stderr, "write completion: %v\n", err)
		return 2
	}
	return 0
}

// completionScript renders a script from the catalog only (F010-FR-008).
// Both shells share one body: zsh runs it with ksharrays, so arrays are
// indexed from 0 as in bash, and only offering words and file names
// differs. The body uses only shell builtins and needs bash 3.2 or zsh 5.
func completionScript(shell string) string {
	var b strings.Builder
	if shell == "zsh" {
		b.WriteString("#compdef ai-dev\n")
	}
	fmt.Fprintf(&b, "# %s completion for ai-dev, written by \"ai-dev completion %s\".\n", shell, shell)
	b.WriteString("# It is static: completing runs no program and reads no file contents.\n\n")
	writeCompletionTables(&b)
	b.WriteString(completionBody)
	if shell == "zsh" {
		b.WriteString(zshEntry)
	} else {
		b.WriteString(bashEntry)
	}
	return b.String()
}

// writeCompletionTables renders the catalog as shell case statements that
// set _ai_dev_r. Every embedded word is shell-safe, which a catalog test
// enforces, so no quoting is needed inside the lists.
func writeCompletionTables(b *strings.Builder) {
	table := func(name, comment string, rows [][2]string) {
		fmt.Fprintf(b, "# %s\n%s() {\n\tcase \"$1\" in\n", comment, name)
		for _, row := range rows {
			fmt.Fprintf(b, "\t\"%s\") _ai_dev_r=\"%s\" ;;\n", row[0], row[1])
		}
		b.WriteString("\t*) _ai_dev_r=\"\" ;;\n\tesac\n}\n\n")
	}
	var paths, children, options, values, args [][2]string
	children = append(children, [2]string{"", childWords(nil)})
	for _, spec := range catalog {
		path := strings.Join(spec.path, " ")
		paths = append(paths, [2]string{path, "y"})
		if words := childWords(spec.path); words != "" {
			children = append(children, [2]string{path, words})
		}
		names := []string{}
		for _, option := range spec.options {
			names = append(names, "--"+option.name)
			switch {
			case len(option.values) > 0:
				values = append(values, [2]string{path + " --" + option.name, "v " + strings.Join(option.values, " ")})
			case option.file:
				values = append(values, [2]string{path + " --" + option.name, "f"})
			case option.value != "":
				values = append(values, [2]string{path + " --" + option.name, "t"})
			}
		}
		options = append(options, [2]string{path, strings.Join(append(names, "--help"), " ")})
		for i, arg := range spec.args {
			if len(arg.values) > 0 {
				args = append(args, [2]string{path + " " + strconv.Itoa(i), strings.Join(arg.values, " ")})
			}
		}
	}
	table("_ai_dev_is_path", "Catalog paths.", paths)
	table("_ai_dev_children", "The next words of each path.", children)
	table("_ai_dev_options", "The options of each path.", options)
	table("_ai_dev_value", "Options that take a value: v (fixed values), f (a file) or t (free text).", values)
	table("_ai_dev_arg", "Fixed values of positional arguments, by path and index.", args)
}

func childWords(path []string) string {
	var words []string
	if path == nil {
		for _, spec := range catalog {
			if len(spec.path) == 1 {
				words = append(words, spec.path[0])
			}
		}
	} else {
		for _, child := range children(path) {
			words = append(words, child.path[len(child.path)-1])
		}
	}
	return strings.Join(words, " ")
}

// completionBody reads the words in w and the cursor index in cw, which
// the shell entry function sets, and offers candidates through
// _ai_dev_offer and _ai_dev_files.
const completionBody = `# Complete the word at index cw of w.
_ai_dev_run() {
	local cmdpath sub list i n prev
	_ai_dev_cur="${w[cw]}"
	prev=""
	if [ "$cw" -gt 1 ]; then prev="${w[cw-1]}"; fi
	# The longest catalog path among the words before the cursor.
	cmdpath=""
	i=1
	while [ "$i" -lt "$cw" ] && _ai_dev_is_path "${cmdpath:+$cmdpath }${w[i]}" && [ -n "$_ai_dev_r" ]; do
		cmdpath="${cmdpath:+$cmdpath }${w[i]}"
		i=$((i + 1))
	done
	# help takes a catalog path as its argument.
	if [ "$cmdpath" = "help" ]; then
		sub=""
		while [ "$i" -lt "$cw" ]; do
			_ai_dev_is_path "${sub:+$sub }${w[i]}"
			if [ -z "$_ai_dev_r" ]; then return 0; fi
			sub="${sub:+$sub }${w[i]}"
			i=$((i + 1))
		done
		_ai_dev_children "$sub"
		_ai_dev_offer "$_ai_dev_r"
		return 0
	fi
	# Count the positional arguments typed after the path.
	n=0
	while [ "$i" -lt "$cw" ]; do
		case "${w[i]}" in
		--) return 0 ;;
		--*=*) ;;
		-*)
			_ai_dev_value "$cmdpath ${w[i]}"
			if [ -n "$_ai_dev_r" ]; then i=$((i + 1)); fi
			;;
		*) n=$((n + 1)) ;;
		esac
		i=$((i + 1))
	done
	# The value of an option.
	case "$prev" in
	-*)
		_ai_dev_value "$cmdpath $prev"
		case "$_ai_dev_r" in
		f) _ai_dev_files; return 0 ;;
		t) return 0 ;;
		v\ *) _ai_dev_offer "${_ai_dev_r#v }"; return 0 ;;
		esac
		;;
	esac
	case "$_ai_dev_cur" in
	-*)
		_ai_dev_options "$cmdpath"
		_ai_dev_offer "$_ai_dev_r"
		return 0
		;;
	esac
	list=""
	if [ "$n" -eq 0 ]; then
		_ai_dev_children "$cmdpath"
		list="$_ai_dev_r"
	fi
	if [ -n "$cmdpath" ]; then
		_ai_dev_arg "$cmdpath $n"
		list="$list $_ai_dev_r"
	fi
	_ai_dev_offer "$list"
}

`

const bashEntry = `_ai_dev_offer() {
	COMPREPLY=($(compgen -W "$1" -- "$_ai_dev_cur"))
}

_ai_dev_files() {
	local IFS=$'\n'
	COMPREPLY=($(compgen -f -- "$_ai_dev_cur"))
}

_ai_dev() {
	local -a w
	local cw _ai_dev_cur _ai_dev_r
	w=("${COMP_WORDS[@]}")
	cw=$COMP_CWORD
	COMPREPLY=()
	_ai_dev_run
}

complete -o filenames -F _ai_dev ai-dev
`

const zshEntry = `_ai_dev_offer() {
	compadd -- ${=1}
}

_ai_dev_files() {
	_files
}

_ai_dev() {
	setopt localoptions ksharrays
	local -a w
	local cw _ai_dev_cur _ai_dev_r
	w=("${words[@]}")
	cw=$((CURRENT - 1))
	_ai_dev_run
}

if [ "${funcstack[1]}" = "_ai_dev" ]; then
	_ai_dev "$@"
else
	compdef _ai_dev ai-dev
fi
`
