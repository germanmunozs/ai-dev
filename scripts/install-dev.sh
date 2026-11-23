#!/usr/bin/env bash
# scripts/install-dev.sh: build ai-dev from this checkout and install it
# for manual testing, before there is a release to download. It changes
# only the installed binary (and, with --sandbox, a new directory you
# name). It never edits shell startup files, configuration, credentials or
# the repository.
#
#   scripts/install-dev.sh [--source DIR] [--prefix DIR]   build and install
#   scripts/install-dev.sh --uninstall [--prefix DIR]       remove the binary
#   scripts/install-dev.sh --sandbox DIR                    create a test project
set -euo pipefail

# The checkout this script belongs to, unless --source says otherwise.
source_dir="$(cd "$(dirname "$0")/.." && pwd)"
prefix="$HOME/.local/bin"
action=install
sandbox=""

die() { printf 'install-dev: %s\n' "$*" >&2; exit 1; }

while [ $# -gt 0 ]; do
  case "$1" in
    --source) source_dir="${2:?--source needs a directory}"; shift 2 ;;
    --prefix) prefix="${2:?--prefix needs a directory}"; shift 2 ;;
    --uninstall) action=uninstall; shift ;;
    --sandbox) action=sandbox; sandbox="${2:?--sandbox needs a directory}"; shift 2 ;;
    -h|--help) sed -n '2,10p' "$0"; exit 0 ;;
    *) die "unknown option: $1" ;;
  esac
done

target="$prefix/ai-dev"

do_uninstall() {
  [ -e "$target" ] || { echo "Nothing to remove: $target does not exist."; return; }
  # Remove the file only if it really is ai-dev.
  "$target" version --format json 2>/dev/null | grep -q '"schema_version":"ai-dev.version/v1"' \
    || die "$target does not look like ai-dev; not removing it"
  rm -f "$target"
  echo "Removed $target."
}

do_sandbox() {
  [ ! -e "$sandbox" ] || die "$sandbox already exists; choose a new directory"
  mkdir -p "$sandbox/project/specs/001-demo" "$sandbox/fake-agents"
  printf '# Demo feature\n' > "$sandbox/project/specs/001-demo/spec.md"
  git -C "$sandbox/project" init -q
  # Fake agents print their arguments instead of doing anything.
  for name in claude codex; do
    printf '#!/bin/sh\necho "fake %s received:"\nfor a in "$@"; do printf "  [%%s]\\n" "$a"; done\n' "$name" \
      > "$sandbox/fake-agents/$name"
    chmod 0755 "$sandbox/fake-agents/$name"
  done
  cat <<EOF
Sandbox created:
  project      $sandbox/project   (git repository, one feature: specs/001-demo)
  fake agents  $sandbox/fake-agents (claude and codex print their arguments)

Use it with the fake agents first on PATH:
  cd "$sandbox/project"
  PATH="$sandbox/fake-agents:\$PATH" ai-dev tui
Delete it when you are done:
  rm -rf "$sandbox"
EOF
}

do_install() {
  command -v go >/dev/null || die "Go is not installed (go.mod asks for $(awk '/^go /{print $2}' "$source_dir/go.mod" 2>/dev/null))"
  command -v git >/dev/null || die "git is not installed"
  [ -f "$source_dir/go.mod" ] && grep -q '^module github.com/germanmunozs/ai-dev$' "$source_dir/go.mod" \
    || die "$source_dir is not an ai-dev checkout (use --source DIR)"

  want_go="$(awk '/^go /{print $2}' "$source_dir/go.mod")"
  have_go="$(go env GOVERSION)"
  [ "$have_go" = "go$want_go" ] || echo "warning: go.mod declares go $want_go, this Go is $have_go" >&2

  commit="$(git -C "$source_dir" rev-parse --short=12 HEAD)"
  branch="$(git -C "$source_dir" rev-parse --abbrev-ref HEAD)"
  suffix=""
  if [ -n "$(git -C "$source_dir" status --porcelain)" ]; then
    suffix=".dirty"
    echo "warning: $source_dir has uncommitted changes; they are included in the build" >&2
  fi
  version="v0.2.2-dev+${commit}${suffix}"

  build_dir="$(mktemp -d)"
  trap 'rm -rf "$build_dir"' EXIT
  echo "Building ai-dev $version from $source_dir ($branch)…"
  (cd "$source_dir" && go build -trimpath \
    -ldflags "-X github.com/germanmunozs/ai-dev/internal/version.Version=$version" \
    -o "$build_dir/ai-dev" ./cmd/ai-dev)
  "$build_dir/ai-dev" version >/dev/null || die "the built binary does not run"

  mkdir -p "$prefix"
  # Copy next to the target, then rename: the replacement is atomic.
  install -m 0755 "$build_dir/ai-dev" "$prefix/.ai-dev.new"
  mv -f "$prefix/.ai-dev.new" "$target"
  echo "Installed $target"
  "$target" version

  case ":$PATH:" in
    *":$prefix:"*) ;;
    *) printf '\n%s is not on your PATH. For this shell:\n  export PATH="%s:$PATH"\n' "$prefix" "$prefix" ;;
  esac
  cat <<EOF

Shell completion (optional, not installed automatically):
  zsh:  eval "\$(ai-dev completion zsh)"    # after compinit
  bash: eval "\$(ai-dev completion bash)"
Uninstall:
  $0 --uninstall --prefix "$prefix"
EOF
}

case "$action" in
  install) do_install ;;
  uninstall) do_uninstall ;;
  sandbox) do_sandbox ;;
esac
