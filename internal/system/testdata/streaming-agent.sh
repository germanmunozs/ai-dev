#!/bin/sh
set -eu

release_file=$1
pid_file=$2
exit_code=${3:-0}

trap 'exit 130' INT TERM

printf '%s\n' 'stdout-before-exit'
printf '%s\n' 'stderr-before-exit' >&2

# Publish the PID last, and atomically: tests signal the child as soon as the
# PID file appears, so both markers must already be written and the trap
# installed, and a reader must never see a created but still empty file.
printf '%s\n' "$$" > "$pid_file.tmp"
mv "$pid_file.tmp" "$pid_file"

while [ ! -f "$release_file" ]; do
	sleep 0.01
done

exit "$exit_code"
