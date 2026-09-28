#!/bin/sh
set -eu

exit_code=$1
shift

printf 'cwd=%s\n' "$(pwd)"
for argument in "$@"; do
	printf 'arg=%s\n' "$argument"
done
printf '%s\n' 'fixture-stderr' >&2

exit "$exit_code"
