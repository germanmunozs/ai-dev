#!/bin/sh
set -eu

release_file=$1
pid_file=$2
exit_code=${3:-0}

printf '%s\n' "$$" > "$pid_file"

trap 'exit 130' INT TERM

printf '%s\n' 'stdout-before-exit'
printf '%s\n' 'stderr-before-exit' >&2

while [ ! -f "$release_file" ]; do
	sleep 0.01
done

exit "$exit_code"
